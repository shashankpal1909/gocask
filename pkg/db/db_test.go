package db

import (
	"bytes"
	"testing"

	"github.com/shashankpal1909/gocask/pkg/options"
)

func TestDB_PutGetDelete(t *testing.T) {
	opts := options.DefaultOptions()
	opts.DirPath = t.TempDir()

	db, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Verify Put and Get
	records := map[string]string{
		"user:101": "Alice",
		"user:102": "Bob",
		"user:103": "Charlie",
	}

	for k, v := range records {
		if err := db.Put([]byte(k), []byte(v)); err != nil {
			t.Fatalf("Put(%s) failed: %v", k, err)
		}
	}

	for k, expectedVal := range records {
		val, err := db.Get([]byte(k))
		if err != nil {
			t.Fatalf("Get(%s) failed: %v", k, err)
		}
		if !bytes.Equal(val, []byte(expectedVal)) {
			t.Fatalf("Get(%s) expected %s, got %s", k, expectedVal, string(val))
		}
	}

	// Verify non-existent key returns ErrKeyNotFound
	_, err = db.Get([]byte("user:999"))
	if err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound for non-existent key, got %v", err)
	}

	// Verify Delete
	if err := db.Delete([]byte("user:102")); err != nil {
		t.Fatalf("Delete(user:102) failed: %v", err)
	}

	// Verify deleted key cannot be fetched
	_, err = db.Get([]byte("user:102"))
	if err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound after Delete, got %v", err)
	}

	// Verify deleting an already deleted key returns ErrKeyNotFound
	if err := db.Delete([]byte("user:102")); err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound when deleting already deleted key, got %v", err)
	}
}

func TestDB_StartupRecoveryAndTombstones(t *testing.T) {
	opts := options.DefaultOptions()
	opts.DirPath = t.TempDir()

	// 1. Initial write session
	db, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	if err := db.Put([]byte("city:sf"), []byte("San Francisco")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := db.Put([]byte("city:ny"), []byte("New York")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := db.Delete([]byte("city:sf")); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if err := db.Put([]byte("city:la"), []byte("Los Angeles")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 2. Reopen database in exact same directory to test WAL replay and tombstone recovery
	reopenedDB, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer reopenedDB.Close()

	// Verify surviving keys
	val, err := reopenedDB.Get([]byte("city:ny"))
	if err != nil || string(val) != "New York" {
		t.Fatalf("expected New York, got %s (err: %v)", string(val), err)
	}
	val, err = reopenedDB.Get([]byte("city:la"))
	if err != nil || string(val) != "Los Angeles" {
		t.Fatalf("expected Los Angeles, got %s (err: %v)", string(val), err)
	}

	// Verify tombstone deleted key stays deleted after restart
	_, err = reopenedDB.Get([]byte("city:sf"))
	if err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound for tombstoned key after restart, got %v", err)
	}
}

func TestDB_EmptyKey(t *testing.T) {
	opts := options.DefaultOptions()
	opts.DirPath = t.TempDir()

	db, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Put(nil, []byte("val")); err != ErrEmptyKey {
		t.Fatalf("expected ErrEmptyKey for nil key, got %v", err)
	}
	if err := db.Put([]byte(""), []byte("val")); err != ErrEmptyKey {
		t.Fatalf("expected ErrEmptyKey for empty string key, got %v", err)
	}
}
