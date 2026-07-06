package storage

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/shashankpal1909/gocask/pkg/index"
	"github.com/shashankpal1909/gocask/pkg/wal"
)

// LogFile represents an on-disk append-only data file (e.g., 000000001.data).
// It wraps an OS file handle with a buffered writer for high-throughput sequential writes.
type LogFile struct {
	Fid         uint32        // Unique file identifier
	WriteOffset int64         // Current write offset in the file
	File        *os.File      // Raw OS file handle for random reads (ReadAt)
	Writer      *bufio.Writer // Buffered writer for batching sequential writes
	lock        *sync.RWMutex // Read-write mutex for concurrency control
}

// OpenLogFile opens an existing log file or creates a new one in dirPath with the given file ID.
func OpenLogFile(dirPath string, fid uint32) (*LogFile, error) {
	path := fmt.Sprintf("%s/%09d.data", dirPath, fid)

	// Open file with append and read-write permissions, creating it if it doesn't exist
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	return &LogFile{
		Fid:         fid,
		WriteOffset: fileInfo.Size(),
		File:        file,
		Writer:      bufio.NewWriterSize(file, 65536), // 64 KB in-memory buffer
		lock:        &sync.RWMutex{},
	}, nil
}

// Write encodes and appends a LogRecord to the buffered writer.
// It returns the LogRecordPos pointing to the exact disk location of the record.
func (lf *LogFile) Write(record *wal.LogRecord) (*index.LogRecordPos, error) {
	lf.lock.Lock()
	defer lf.lock.Unlock()

	buf, size := wal.EncodeLogRecord(record)
	if _, err := lf.Writer.Write(buf); err != nil {
		return nil, err
	}

	pos := &index.LogRecordPos{
		Fid:    lf.Fid,
		Offset: lf.WriteOffset,
		Size:   uint32(size),
	}

	lf.WriteOffset += size
	return pos, nil
}

// Read reads and decodes a LogRecord from disk at the specified offset and size.
func (lf *LogFile) Read(offset int64, size uint32) (*wal.LogRecord, error) {
	lf.lock.RLock()
	defer lf.lock.RUnlock()

	// Ensure any pending writes in RAM reach the OS file before reading
	lf.Writer.Flush()

	buf := make([]byte, size)
	if _, err := lf.File.ReadAt(buf, offset); err != nil {
		return nil, err
	}

	header, headerSize := wal.DecodeLogRecordHeader(buf)

	record := &wal.LogRecord{
		Key:   buf[headerSize : int(headerSize)+int(header.KeySize)],
		Value: buf[int(headerSize)+int(header.KeySize) : int(headerSize)+int(header.KeySize)+int(header.ValueSize)],
		Type:  header.RecordType,
	}

	// Verify CRC checksum against payload
	if wal.GetLogRecordCRC(record, buf[:int(headerSize)]) != header.CRC {
		return nil, errors.New("CRC mismatch")
	}

	return record, nil
}

// Sync flushes buffered data to the OS kernel and synchronizes disk caches (fsync).
func (lf *LogFile) Sync() error {
	lf.lock.Lock()
	defer lf.lock.Unlock()

	if err := lf.Writer.Flush(); err != nil {
		return err
	}

	return lf.File.Sync()
}

// Close synchronizes all buffered data to disk and closes the OS file handle.
func (lf *LogFile) Close() error {
	if err := lf.Sync(); err != nil {
		return err
	}

	return lf.File.Close()
}
