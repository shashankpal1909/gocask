package index

import (
	"reflect"
	"testing"
)

func TestBTree_PutGetDelete(t *testing.T) {
	bt := NewBTreeIndexer(32)
	defer bt.Close()

	if bt.Size() != 0 {
		t.Fatalf("expected size 0, got %d", bt.Size())
	}

	// Test Get non-existent
	pos, ok := bt.Get([]byte("not-exist"))
	if ok || pos != nil {
		t.Fatalf("expected nil/false for non-existent key, got %v/%v", pos, ok)
	}

	// Test Put
	res1 := bt.Put([]byte("key-1"), &LogRecordPos{Fid: 1, Offset: 100, Size: 50})
	if res1 != nil {
		t.Fatalf("expected nil when inserting new key, got %v", res1)
	}
	if bt.Size() != 1 {
		t.Fatalf("expected size 1, got %d", bt.Size())
	}

	// Test Get existing
	pos1, ok := bt.Get([]byte("key-1"))
	if !ok || pos1.Fid != 1 || pos1.Offset != 100 || pos1.Size != 50 {
		t.Fatalf("unexpected pos returned from Get: %v, %v", pos1, ok)
	}

	// Test Put update existing key
	res2 := bt.Put([]byte("key-1"), &LogRecordPos{Fid: 1, Offset: 200, Size: 50})
	if res2 == nil || res2.Offset != 100 {
		t.Fatalf("expected old pos offset 100 when updating, got %v", res2)
	}
	if bt.Size() != 1 {
		t.Fatalf("expected size 1 after update, got %d", bt.Size())
	}

	// Test Delete
	delRes := bt.Delete([]byte("key-1"))
	if delRes == nil || delRes.Offset != 200 {
		t.Fatalf("expected deleted pos offset 200, got %v", delRes)
	}
	if bt.Size() != 0 {
		t.Fatalf("expected size 0 after delete, got %d", bt.Size())
	}

	// Test Delete non-existent
	if bt.Delete([]byte("key-1")) != nil {
		t.Fatalf("expected nil when deleting non-existent key")
	}
}

func TestBTree_Iterator_Forward(t *testing.T) {
	bt := NewBTreeIndexer(32)
	defer bt.Close()

	keys := [][]byte{
		[]byte("code"),
		[]byte("apple"),
		[]byte("banana"),
		[]byte("zebra"),
	}
	for i, k := range keys {
		bt.Put(k, &LogRecordPos{Fid: uint32(i), Offset: int64(i * 10), Size: 100})
	}

	it := bt.Iterator(false)
	defer it.Close()

	expectedOrder := [][]byte{
		[]byte("apple"),
		[]byte("banana"),
		[]byte("code"),
		[]byte("zebra"),
	}

	var result [][]byte
	for it.Rewind(); it.Valid(); it.Next() {
		result = append(result, it.Key())
	}

	if !reflect.DeepEqual(result, expectedOrder) {
		t.Fatalf("forward iteration mismatch: expected %s, got %s", expectedOrder, result)
	}
}

func TestBTree_Iterator_Reverse(t *testing.T) {
	bt := NewBTreeIndexer(32)
	defer bt.Close()

	keys := [][]byte{
		[]byte("code"),
		[]byte("apple"),
		[]byte("banana"),
		[]byte("zebra"),
	}
	for i, k := range keys {
		bt.Put(k, &LogRecordPos{Fid: uint32(i), Offset: int64(i * 10), Size: 100})
	}

	it := bt.Iterator(true)
	defer it.Close()

	expectedOrder := [][]byte{
		[]byte("zebra"),
		[]byte("code"),
		[]byte("banana"),
		[]byte("apple"),
	}

	var result [][]byte
	for it.Rewind(); it.Valid(); it.Next() {
		result = append(result, it.Key())
	}

	if !reflect.DeepEqual(result, expectedOrder) {
		t.Fatalf("reverse iteration mismatch: expected %s, got %s", expectedOrder, result)
	}
}

func TestBTree_Iterator_Seek(t *testing.T) {
	bt := NewBTreeIndexer(32)
	defer bt.Close()

	keys := [][]byte{
		[]byte("a"),
		[]byte("c"),
		[]byte("e"),
		[]byte("g"),
	}
	for i, k := range keys {
		bt.Put(k, &LogRecordPos{Fid: uint32(i), Offset: int64(i * 10), Size: 100})
	}

	// Forward seek
	it := bt.Iterator(false)
	defer it.Close()

	// Seek exact match
	if !it.Seek([]byte("c")) || string(it.Key()) != "c" {
		t.Fatalf("expected seek 'c', got %s (valid: %v)", it.Key(), it.Valid())
	}

	// Seek between keys (should jump to first key >= target)
	if !it.Seek([]byte("d")) || string(it.Key()) != "e" {
		t.Fatalf("expected seek 'd' to land on 'e', got %s", it.Key())
	}

	// Seek past end
	if it.Seek([]byte("z")) || it.Valid() {
		t.Fatalf("expected seek 'z' to be invalid, got valid with key %s", it.Key())
	}

	// Reverse seek
	revIt := bt.Iterator(true)
	defer revIt.Close()

	// In reverse order ["g", "e", "c", "a"], seeking for "d" should land on first key <= "d", which is "c"
	if !revIt.Seek([]byte("d")) || string(revIt.Key()) != "c" {
		t.Fatalf("expected reverse seek 'd' to land on 'c', got %s", revIt.Key())
	}
}
