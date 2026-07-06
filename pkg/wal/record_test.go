package wal

import (
	"testing"
)

func TestEncodeDecodeNormalRecord(t *testing.T) {
	rec := &LogRecord{
		Key:   []byte("apple"),
		Value: []byte("delicious"),
		Type:  LogRecordNormal,
	}

	enc, n := EncodeLogRecord(rec)
	expectedLen := int64(MaxLogRecordHeaderSize + len(rec.Key) + len(rec.Value))
	if n != expectedLen || int64(len(enc)) != expectedLen {
		t.Fatalf("expected length %d, got %d (slice len %d)", expectedLen, n, len(enc))
	}

	// Decode header
	header, headerLen := DecodeLogRecordHeader(enc)
	if header == nil || headerLen != MaxLogRecordHeaderSize {
		t.Fatalf("expected header len %d, got %d", MaxLogRecordHeaderSize, headerLen)
	}

	if header.RecordType != LogRecordNormal {
		t.Fatalf("expected type Normal (%d), got %d", LogRecordNormal, header.RecordType)
	}
	if header.KeySize != uint32(len(rec.Key)) || header.ValueSize != uint32(len(rec.Value)) {
		t.Fatalf("expected sizes %d/%d, got %d/%d", len(rec.Key), len(rec.Value), header.KeySize, header.ValueSize)
	}

	// Verify CRC
	crc := GetLogRecordCRC(rec, enc[:MaxLogRecordHeaderSize])
	if crc != header.CRC {
		t.Fatalf("CRC mismatch: calculated %d, stored in header %d", crc, header.CRC)
	}
}

func TestEncodeDecodeDeletedRecord(t *testing.T) {
	rec := &LogRecord{
		Key:   []byte("apple"),
		Value: nil,
		Type:  LogRecordDelete,
	}

	enc, _ := EncodeLogRecord(rec)
	header, _ := DecodeLogRecordHeader(enc)

	if header.RecordType != LogRecordDelete {
		t.Fatalf("expected type Delete (%d), got %d", LogRecordDelete, header.RecordType)
	}
	if header.ValueSize != 0 {
		t.Fatalf("expected value size 0, got %d", header.ValueSize)
	}

	crc := GetLogRecordCRC(rec, enc[:MaxLogRecordHeaderSize])
	if crc != header.CRC {
		t.Fatalf("CRC mismatch for deleted record: calculated %d, stored %d", crc, header.CRC)
	}
}

func TestRecordCRCCorruption(t *testing.T) {
	rec := &LogRecord{
		Key:   []byte("key-to-corrupt"),
		Value: []byte("val-to-corrupt"),
		Type:  LogRecordNormal,
	}

	enc, _ := EncodeLogRecord(rec)
	header, _ := DecodeLogRecordHeader(enc)

	// Simulate disk corruption / partial write by flipping a bit in the value
	corruptedRec := &LogRecord{
		Key:   rec.Key,
		Value: []byte("val-to-corrupt-MODIFIED"),
		Type:  LogRecordNormal,
	}

	crc := GetLogRecordCRC(corruptedRec, enc[:MaxLogRecordHeaderSize])
	if crc == header.CRC {
		t.Fatalf("expected CRC mismatch when payload is corrupted, but CRCs matched!")
	}
}
