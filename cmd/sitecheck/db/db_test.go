package db

import (
	"path/filepath"
	"reflect"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestResourceMetaRoundTrip(t *testing.T) {
	db := openTestDB(t)

	t.Run("absent returns false", func(t *testing.T) {
		if m, ok := db.ResourceMeta("slug", "op"); ok {
			t.Errorf("ResourceMeta = %v, %v; want nil, false", m, ok)
		}
	})

	t.Run("upsert then read back", func(t *testing.T) {
		want := map[string]string{"internal": "basic", "default": "full"}
		if err := db.UpsertResourceMeta("slug", "op", want); err != nil {
			t.Fatalf("UpsertResourceMeta: %v", err)
		}
		got, ok := db.ResourceMeta("slug", "op")
		if !ok {
			t.Fatal("ResourceMeta: not found after upsert")
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ResourceMeta = %v, want %v", got, want)
		}
	})

	t.Run("upsert overwrites prior entry", func(t *testing.T) {
		if err := db.UpsertResourceMeta("slug", "op", map[string]string{"internal": "basic"}); err != nil {
			t.Fatalf("UpsertResourceMeta: %v", err)
		}
		if err := db.UpsertResourceMeta("slug", "op", map[string]string{"internal": "full"}); err != nil {
			t.Fatalf("UpsertResourceMeta overwrite: %v", err)
		}
		got, _ := db.ResourceMeta("slug", "op")
		if got["internal"] != "full" {
			t.Errorf("after overwrite internal = %q, want full", got["internal"])
		}
	})

	t.Run("keyed by slug and outpost", func(t *testing.T) {
		if err := db.UpsertResourceMeta("a", "op1", map[string]string{"x": "basic"}); err != nil {
			t.Fatal(err)
		}
		if err := db.UpsertResourceMeta("a", "op2", map[string]string{"y": "basic"}); err != nil {
			t.Fatal(err)
		}
		m1, _ := db.ResourceMeta("a", "op1")
		m2, _ := db.ResourceMeta("a", "op2")
		if _, ok := m1["y"]; ok {
			t.Errorf("op1 entry leaked y: %v", m1)
		}
		if _, ok := m2["x"]; ok {
			t.Errorf("op2 entry leaked x: %v", m2)
		}
		if _, ok := db.ResourceMeta("b", "op1"); ok {
			t.Error("unrelated slug found")
		}
	})

	t.Run("nil sites stored as empty object", func(t *testing.T) {
		if err := db.UpsertResourceMeta("slug", "op", nil); err != nil {
			t.Fatalf("UpsertResourceMeta nil: %v", err)
		}
		got, ok := db.ResourceMeta("slug", "op")
		if !ok {
			t.Fatal("ResourceMeta: not found after nil upsert")
		}
		if len(got) != 0 {
			t.Errorf("ResourceMeta = %v, want empty map", got)
		}
	})
}
