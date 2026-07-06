package index

import (
	"bytes"
	"sort"
	"sync"

	"github.com/google/btree"
)

// Item represents a key-value pointer pair stored in the B-Tree.
type Item struct {
	key []byte
	pos *LogRecordPos
}

// Less compares two B-Tree items lexicographically by key.
func (i *Item) Less(bi btree.Item) bool {
	return bytes.Compare(i.key, bi.(*Item).key) < 0
}

// BTreeIndexer is a thread-safe B-Tree implementation of the Indexer interface.
type BTreeIndexer struct {
	tree *btree.BTree
	lock *sync.RWMutex
}

// NewBTreeIndexer initializes a new BTreeIndexer with the given degree.
func NewBTreeIndexer(degree int) *BTreeIndexer {
	return &BTreeIndexer{
		tree: btree.New(degree),
		lock: &sync.RWMutex{},
	}
}

// Put stores a key's position in the B-Tree, returning the old position if replaced.
func (bt *BTreeIndexer) Put(key []byte, pos *LogRecordPos) *LogRecordPos {
	bt.lock.Lock()
	defer bt.lock.Unlock()

	existingItem := bt.tree.ReplaceOrInsert(&Item{key: key, pos: pos})
	if existingItem == nil {
		return nil
	}
	return existingItem.(*Item).pos
}

// Get retrieves the log record position for a given key.
func (bt *BTreeIndexer) Get(key []byte) (*LogRecordPos, bool) {
	bt.lock.RLock()
	defer bt.lock.RUnlock()

	item := bt.tree.Get(&Item{key: key})
	if item == nil {
		return nil, false
	}
	return item.(*Item).pos, true
}

// Delete removes a key from the B-Tree, returning its previous position.
func (bt *BTreeIndexer) Delete(key []byte) *LogRecordPos {
	bt.lock.Lock()
	defer bt.lock.Unlock()

	item := bt.tree.Delete(&Item{key: key})
	if item == nil {
		return nil
	}
	return item.(*Item).pos
}

// Size returns the number of items currently stored in the B-Tree.
func (bt *BTreeIndexer) Size() int {
	bt.lock.RLock()
	defer bt.lock.RUnlock()

	return bt.tree.Len()
}

// Iterator returns a snapshot iterator for traversing the B-Tree.
func (bt *BTreeIndexer) Iterator(reverse bool) Iterator {
	bt.lock.RLock()
	defer bt.lock.RUnlock()

	items := make([]*Item, 0, bt.tree.Len())
	bt.tree.Ascend(func(i btree.Item) bool {
		items = append(items, i.(*Item))
		return true
	})

	if reverse {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}

	return &btreeIterator{
		items:   items,
		reverse: reverse,
		idx:     0,
	}
}

// Close closes the B-Tree indexer.
func (bt *BTreeIndexer) Close() {
}

// btreeIterator is a snapshot iterator over the B-Tree items.
type btreeIterator struct {
	items   []*Item
	reverse bool
	idx     int
}

// Rewind moves the iterator to the start of the snapshot.
func (bi *btreeIterator) Rewind() {
	bi.idx = 0
}

// Seek moves the iterator to the first key greater than or equal to target (or <= if reverse).
func (bi *btreeIterator) Seek(key []byte) bool {
	if bi.reverse {
		bi.idx = sort.Search(len(bi.items), func(i int) bool {
			return bytes.Compare(bi.items[i].key, key) <= 0
		})
	} else {
		bi.idx = sort.Search(len(bi.items), func(i int) bool {
			return bytes.Compare(bi.items[i].key, key) >= 0
		})
	}
	return bi.Valid()
}

// Next advances the iterator to the next item.
func (bi *btreeIterator) Next() bool {
	bi.idx++
	return bi.Valid()
}

// Valid checks if the iterator is currently pointing to a valid item.
func (bi *btreeIterator) Valid() bool {
	return bi.idx >= 0 && bi.idx < len(bi.items)
}

// Key returns the key at the current iterator position.
func (bi *btreeIterator) Key() []byte {
	if !bi.Valid() {
		return nil
	}
	return bi.items[bi.idx].key
}

// Value returns the position at the current iterator position.
func (bi *btreeIterator) Value() *LogRecordPos {
	if !bi.Valid() {
		return nil
	}
	return bi.items[bi.idx].pos
}

// Close releases the iterator's item snapshot.
func (bi *btreeIterator) Close() {
	bi.items = nil
}

