package db

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/shashankpal1909/gocask/pkg/index"
	"github.com/shashankpal1909/gocask/pkg/options"
	"github.com/shashankpal1909/gocask/pkg/storage"
	"github.com/shashankpal1909/gocask/pkg/wal"
)

var (
	ErrKeyNotFound = errors.New("key not found in database")
	ErrEmptyKey    = errors.New("key cannot be empty")
)

// DB represents a Bitcask storage engine instance.
// It manages on-disk log files and an in-memory B-Tree index.
type DB struct {
	mu         *sync.RWMutex
	opts       options.Options
	activeFile *storage.LogFile            // Current active file being written to (e.g., 000000001.data)
	oldFiles   map[uint32]*storage.LogFile // Older immutable files used for reads and recovery
	index      index.Indexer               // In-memory B-Tree index mapping keys to disk positions
}

// Open initializes or reopens a Bitcask database directory with the given options.
// It scans existing data files and reconstructs the in-memory index from disk.
func Open(opts options.Options) (*DB, error) {
	if err := os.MkdirAll(opts.DirPath, os.ModePerm); err != nil {
		return nil, err
	}

	db := &DB{
		mu:       &sync.RWMutex{},
		opts:     opts,
		oldFiles: make(map[uint32]*storage.LogFile),
		index:    index.NewBTreeIndexer(32),
	}

	availableFiles, err := os.ReadDir(opts.DirPath)
	if err != nil {
		return nil, err
	}

	var fids []uint32
	for _, file := range availableFiles {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if filepath.Ext(name) != ".data" {
			continue
		}

		fidStr := strings.TrimSuffix(name, ".data")
		fid, err := strconv.ParseUint(fidStr, 10, 32)
		if err != nil {
			continue
		}
		fids = append(fids, uint32(fid))
	}

	// If no existing data files are found, open initial log file 1
	if len(fids) == 0 {
		var err error
		db.activeFile, err = storage.OpenLogFile(opts.DirPath, 1)
		if err != nil {
			return nil, err
		}
		return db, nil
	}

	// Sort file IDs ascending to separate older immutable files from the active file
	sort.Slice(fids, func(i, j int) bool {
		return fids[i] < fids[j]
	})

	lastFid := fids[len(fids)-1]
	for i := 0; i < len(fids)-1; i++ {
		lf, err := storage.OpenLogFile(opts.DirPath, fids[i])
		if err != nil {
			return nil, err
		}
		db.oldFiles[fids[i]] = lf
	}

	db.activeFile, err = storage.OpenLogFile(opts.DirPath, lastFid)
	if err != nil {
		return nil, err
	}

	// Replay log files sequentially to rebuild the in-memory B-Tree index
	if err := db.loadIndexFromDisk(); err != nil {
		return nil, err
	}
	return db, nil
}

// loadIndexFromDisk iterates over all data files sequentially from oldest to newest
// and replays the records into the in-memory B-Tree index.
func (db *DB) loadIndexFromDisk() error {
	var fids []uint32
	for fid := range db.oldFiles {
		fids = append(fids, fid)
	}
	fids = append(fids, db.activeFile.Fid)

	sort.Slice(fids, func(i, j int) bool {
		return fids[i] < fids[j]
	})

	for _, fid := range fids {
		var lf *storage.LogFile
		if fid == db.activeFile.Fid {
			lf = db.activeFile
		} else {
			lf = db.oldFiles[fid]
		}

		if lf == nil {
			return errors.New("data file not found for record position")
		}

		offset := int64(0)
		for {
			if offset >= lf.WriteOffset {
				break
			}

			// Read up to MaxLogRecordHeaderSize bytes to decode the header
			headerBuf := make([]byte, wal.MaxLogRecordHeaderSize)
			if _, err := lf.File.ReadAt(headerBuf, offset); err != nil {
				return err
			}

			header, headerSize := wal.DecodeLogRecordHeader(headerBuf)
			if header == nil || header.CRC == 0 {
				break
			}

			recordSize := uint32(headerSize) + header.KeySize + header.ValueSize

			record, err := lf.Read(offset, recordSize)
			if err != nil {
				return err
			}

			// Update in-memory B-Tree index based on record type
			if record.Type == wal.LogRecordNormal {
				db.index.Put(record.Key, &index.LogRecordPos{
					Fid:    fid,
					Offset: offset,
					Size:   recordSize,
				})
			} else {
				db.index.Delete(record.Key)
			}

			offset += int64(recordSize)
		}
	}

	return nil
}

// Put writes a key-value pair to the active log file and updates the in-memory index.
func (db *DB) Put(key []byte, value []byte) error {
	if len(key) == 0 {
		return ErrEmptyKey
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	record := &wal.LogRecord{
		Key:   key,
		Value: value,
		Type:  wal.LogRecordNormal,
	}

	pos, err := db.activeFile.Write(record)
	if err != nil {
		return err
	}

	if db.opts.SyncOnPut {
		db.activeFile.Sync()
	}

	db.index.Put(key, pos)
	return nil
}

// Get retrieves the value associated with the given key from the database.
func (db *DB) Get(key []byte) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	pos, ok := db.index.Get(key)
	if !ok || pos == nil {
		return nil, ErrKeyNotFound
	}

	var lf *storage.LogFile
	if pos.Fid == db.activeFile.Fid {
		lf = db.activeFile
	} else {
		lf = db.oldFiles[pos.Fid]
	}

	if lf == nil {
		return nil, errors.New("data file not found for record position")
	}

	rec, err := lf.Read(pos.Offset, pos.Size)
	if err != nil {
		return nil, err
	}

	if rec.Type == wal.LogRecordDelete {
		return nil, ErrKeyNotFound
	}

	return rec.Value, nil
}

// Delete appends a tombstone record for the key and removes it from the in-memory index.
func (db *DB) Delete(key []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, ok := db.index.Get(key)
	if !ok {
		return ErrKeyNotFound
	}

	rec := &wal.LogRecord{
		Key:   key,
		Value: nil,
		Type:  wal.LogRecordDelete,
	}

	if _, err := db.activeFile.Write(rec); err != nil {
		return err
	}

	db.index.Delete(key)
	return nil
}

// Close flushes active writes and safely closes all open file handles.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if err := db.activeFile.Sync(); err != nil {
		return err
	}
	if err := db.activeFile.Close(); err != nil {
		return err
	}

	for _, lf := range db.oldFiles {
		lf.Close()
	}

	return nil
}
