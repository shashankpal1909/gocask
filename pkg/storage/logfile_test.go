package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/shashankpal1909/gocask/pkg/wal"
)

func TestLogFile_OpenWriteRead(t *testing.T) {
	dir, err := os.MkdirTemp("", "gocask-logfile-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	lf, err := OpenLogFile(dir, 1)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}
	defer lf.Close()

	if lf.WriteOffset != 0 {
		t.Fatalf("expected initial offset 0, got %d", lf.WriteOffset)
	}

	records := []*wal.LogRecord{
		{Key: []byte("key-1"), Value: []byte("val-1"), Type: wal.LogRecordNormal},
		{Key: []byte("key-2"), Value: []byte("val-2"), Type: wal.LogRecordNormal},
		{Key: []byte("key-3"), Value: nil, Type: wal.LogRecordDelete},
	}

	var positions []struct {
		offset int64
		size   uint32
	}

	for _, rec := range records {
		pos, err := lf.Write(rec)
		if err != nil {
			t.Fatalf("write failed: %v", err)
		}
		positions = append(positions, struct {
			offset int64
			size   uint32
		}{pos.Offset, pos.Size})
	}

	if lf.WriteOffset == 0 {
		t.Fatalf("expected offset > 0 after writes, got %d", lf.WriteOffset)
	}

	// Read back records and verify
	for i, pos := range positions {
		rec, err := lf.Read(pos.offset, pos.size)
		if err != nil {
			t.Fatalf("read failed at offset %d: %v", pos.offset, err)
		}
		if !bytes.Equal(rec.Key, records[i].Key) || !bytes.Equal(rec.Value, records[i].Value) || rec.Type != records[i].Type {
			t.Fatalf("record mismatch at index %d: expected %+v, got %+v", i, records[i], rec)
		}
	}
}

func TestLogFile_SyncAndReopen(t *testing.T) {
	dir, err := os.MkdirTemp("", "gocask-logfile-reopen-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	lf, err := OpenLogFile(dir, 1)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}

	rec := &wal.LogRecord{Key: []byte("persistent-key"), Value: []byte("persistent-val"), Type: wal.LogRecordNormal}
	pos, err := lf.Write(rec)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	expectedSize := lf.WriteOffset
	if err := lf.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// Reopen file
	reopenedLf, err := OpenLogFile(dir, 1)
	if err != nil {
		t.Fatalf("failed to reopen log file: %v", err)
	}
	defer reopenedLf.Close()

	if reopenedLf.WriteOffset != expectedSize {
		t.Fatalf("expected reopened offset %d, got %d", expectedSize, reopenedLf.WriteOffset)
	}

	// Read record from reopened file
	readRec, err := reopenedLf.Read(pos.Offset, pos.Size)
	if err != nil {
		t.Fatalf("failed to read from reopened file: %v", err)
	}
	if string(readRec.Key) != "persistent-key" || string(readRec.Value) != "persistent-val" {
		t.Fatalf("reopened content mismatch: got key=%s val=%s", readRec.Key, readRec.Value)
	}
}

func TestLogFile_CRCCorruptionDetection(t *testing.T) {
	dir, err := os.MkdirTemp("", "gocask-logfile-crc-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	lf, err := OpenLogFile(dir, 10)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}

	rec := &wal.LogRecord{Key: []byte("safe-key"), Value: []byte("super-secret-value"), Type: wal.LogRecordNormal}
	pos, err := lf.Write(rec)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	lf.Close()

	// Corrupt disk file directly
	filePath := filepath.Join(dir, "000000010.data")
	file, err := os.OpenFile(filePath, os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("failed to open file for corruption: %v", err)
	}
	// Corrupt a byte in the value payload
	file.WriteAt([]byte("X"), pos.Offset+wal.MaxLogRecordHeaderSize+int64(len(rec.Key))+1)
	file.Close()

	// Reopen log file and try to read
	corruptedLf, _ := OpenLogFile(dir, 10)
	defer corruptedLf.Close()

	_, err = corruptedLf.Read(pos.Offset, pos.Size)
	if err == nil {
		t.Fatalf("expected CRC mismatch error when reading corrupted record, got nil!")
	}
}
