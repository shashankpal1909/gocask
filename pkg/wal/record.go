package wal

import (
	"encoding/binary"
	"hash/crc32"
)

// LogRecordType represents the type of log record stored on disk.
type LogRecordType byte

const (
	LogRecordNormal LogRecordType = 1
	LogRecordDelete LogRecordType = 2
)

// MaxLogRecordHeaderSize is the fixed 13-byte header size: CRC32(4) + Type(1) + KeySize(4) + ValueSize(4).
const MaxLogRecordHeaderSize = 13

// LogRecord represents the in-memory payload of a key-value operation.
type LogRecord struct {
	Key   []byte
	Value []byte
	Type  LogRecordType
}

// LogRecordHeader represents the decoded metadata header read from disk.
type LogRecordHeader struct {
	CRC        uint32
	RecordType LogRecordType
	KeySize    uint32
	ValueSize  uint32
}

// EncodeLogRecord serializes a LogRecord into a byte slice with a CRC32 checksum.
func EncodeLogRecord(record *LogRecord) ([]byte, int64) {
	buf := make([]byte, len(record.Key)+len(record.Value)+MaxLogRecordHeaderSize)

	buf[4] = byte(record.Type)
	binary.LittleEndian.PutUint32(buf[5:9], uint32(len(record.Key)))
	binary.LittleEndian.PutUint32(buf[9:13], uint32(len(record.Value)))

	index := 13
	copy(buf[index:], record.Key)
	index += len(record.Key)
	copy(buf[index:], record.Value)
	index += len(record.Value)

	crc := crc32.ChecksumIEEE(buf[4:])
	binary.LittleEndian.PutUint32(buf[0:4], crc)

	return buf, int64(len(buf))
}

// DecodeLogRecordHeader decodes the fixed 13-byte header from a byte buffer.
func DecodeLogRecordHeader(buf []byte) (*LogRecordHeader, int64) {
	if len(buf) < MaxLogRecordHeaderSize {
		return nil, 0
	}

	header := &LogRecordHeader{
		CRC:        binary.LittleEndian.Uint32(buf[0:4]),
		RecordType: LogRecordType(buf[4]),
		KeySize:    binary.LittleEndian.Uint32(buf[5:9]),
		ValueSize:  binary.LittleEndian.Uint32(buf[9:13]),
	}

	return header, int64(MaxLogRecordHeaderSize)
}

// GetLogRecordCRC computes the CRC32 checksum over header[4:] plus key and value payloads.
func GetLogRecordCRC(record *LogRecord, header []byte) uint32 {
	crc := crc32.ChecksumIEEE(header[4:])
	crc = crc32.Update(crc, crc32.IEEETable, record.Key)
	crc = crc32.Update(crc, crc32.IEEETable, record.Value)
	return crc
}
