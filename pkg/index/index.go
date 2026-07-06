package index

// LogRecordPos represents the physical location of a log record on disk.
type LogRecordPos struct {
	Fid    uint32
	Offset int64
	Size   uint32
}

// Indexer defines the interface for in-memory index data structures.
type Indexer interface {
	Put(key []byte, pos *LogRecordPos) *LogRecordPos
	Get(key []byte) (*LogRecordPos, bool)
	Delete(key []byte) *LogRecordPos
	Size() int
	Iterator(reverse bool) Iterator
	Close()
}

// Iterator defines the interface for iterating over index keys in sorted order.
type Iterator interface {
	Rewind()
	Seek(key []byte) bool
	Next() bool
	Valid() bool
	Key() []byte
	Value() *LogRecordPos
	Close()
}


