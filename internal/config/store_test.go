package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreLoadMissingFileReturnsEmptyRules(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "rules.json"))

	rules, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("Load() = %#v, want empty", rules)
	}
}

func TestStoreSaveRoundTripsWithPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	store := NewStore(path)
	rules := []Rule{{ID: "one", Name: "Screen Sharing", LocalPort: 30001, Target: "screen-share.example.net:5900"}}

	if err := store.Save(rules); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, rules) {
		t.Fatalf("Load() = %#v, want %#v", got, rules)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o600 {
		t.Fatalf("rules file mode = %o, want 600", gotMode)
	}
}

func TestStoreSaveReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	store := NewStore(path)
	first := []Rule{{ID: "one", Name: "one", LocalPort: 30001, Target: "host:22"}}
	second := []Rule{{ID: "two", Name: "two", LocalPort: 30002, Target: "host:80"}}

	if err := store.Save(first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if err := store.Save(second); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, second) {
		t.Fatalf("Load() = %#v, want %#v", got, second)
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".rules-*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}
