package mapstore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/phoenix1331/cuddio/internal/replace"
)

// clock is a settable time for expiry tests.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

var start = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

var pairs = []replace.Pair{{Token: "<EMAIL_1>", Value: "alice@shop.test"}, {Token: "<SECRET_1>", Value: "hunter2\n\"quoted\""}}

func newStore(t *testing.T, opts Options) (*Store, *clock) {
	t.Helper()

	c := &clock{t: start}
	if opts.Dir == "" {
		opts.Dir = filepath.Join(t.TempDir(), "cuddio")
	}
	opts.Now = c.now

	return New(opts), c
}

func TestSaveLoad(t *testing.T) {
	t.Parallel()

	s, _ := newStore(t, Options{})
	if got, err := s.Load(); err != nil || got != nil {
		t.Fatalf("Load() before any save = %v, %v, want nothing", got, err)
	}
	if err := s.Save(pairs); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || !reflect.DeepEqual(got, pairs) {
		t.Errorf("Load() = %v, %v, want %v", got, err, pairs)
	}

	if err := s.Save(pairs[:1]); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load(); !reflect.DeepEqual(got, pairs[:1]) {
		t.Errorf("Load() after a second save = %v, want it replaced", got)
	}
	if err := s.Save(nil); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(); err != nil || len(got) != 0 {
		t.Errorf("Load() of an empty mapping = %v, %v", got, err)
	}
}

func TestPermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix modes; the store relies on the per-user ACL of the cache directory")
	}

	dir := filepath.Join(t.TempDir(), "cuddio")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s, _ := newStore(t, Options{Dir: dir})
	if err := s.Save(pairs); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, fileName): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", filepath.Base(path), got, want)
		}
	}
}

func TestExpiry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ttl   time.Duration
		after time.Duration
		live  bool
	}{
		{"default ttl, just inside", 0, DefaultTTL - time.Second, true},
		{"default ttl, at expiry", 0, DefaultTTL, false},
		{"custom ttl, inside", 10 * time.Minute, 9 * time.Minute, true},
		{"custom ttl, past", 10 * time.Minute, 11 * time.Minute, false},
		{"saved in the future", 0, -time.Hour, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, c := newStore(t, Options{TTL: tt.ttl})
			if err := s.Save(pairs); err != nil {
				t.Fatal(err)
			}
			c.t = start.Add(tt.after)

			got, err := s.Load()
			if err != nil || (got != nil) != tt.live {
				t.Fatalf("Load() = %v, %v, want live %v", got, err, tt.live)
			}
			_, statErr := os.Stat(filepath.Join(s.dir, fileName))
			if exists := statErr == nil; exists != tt.live {
				t.Errorf("mapping file exists = %v, want %v", exists, tt.live)
			}
		})
	}
}

func TestSavePurgesExpired(t *testing.T) {
	t.Parallel()

	s, c := newStore(t, Options{})
	if err := s.Save(pairs); err != nil {
		t.Fatal(err)
	}
	c.t = start.Add(2 * DefaultTTL)
	if err := s.Save(pairs[:1]); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load(); !reflect.DeepEqual(got, pairs[:1]) {
		t.Errorf("Load() = %v, want only the new mapping", got)
	}
}

func TestCorruptFiles(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"not json":      "alice@shop.test is not json",
		"truncated":     `{"version":1,"saved_at":"2026-10-27T09:00:00Z","pairs":[{"token":"<EMAIL_1>","value":"alice@shop.te`,
		"wrong version": `{"version":2,"saved_at":"2026-10-27T09:00:00Z","pairs":[]}`,
		"no timestamp":  `{"version":1,"pairs":[{"token":"<EMAIL_1>","value":"alice@shop.test"}]}`,
		"wrong types":   `{"version":"1","saved_at":"alice@shop.test"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s, _ := newStore(t, Options{})
			if err := os.MkdirAll(s.dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(s.dir, fileName)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := s.Load()
			if !errors.Is(err, ErrCorrupt) || got != nil {
				t.Fatalf("Load() = %v, %v, want ErrCorrupt", got, err)
			}
			if strings.Contains(err.Error(), "alice") || strings.Contains(err.Error(), "shop") {
				t.Errorf("error %q repeats the file's contents", err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Error("corrupt mapping file was not deleted")
			}

			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := s.Save(pairs); err != nil {
				t.Errorf("Save() over a corrupt file = %v", err)
			}
		})
	}
}

func TestPurgesLeftoverTempFiles(t *testing.T) {
	t.Parallel()

	s, _ := newStore(t, Options{})
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(s.dir, tempPrefix+"stale")
	fresh := filepath.Join(s.dir, tempPrefix+"fresh")
	other := filepath.Join(s.dir, "notes.txt")
	for _, p := range []string{stale, fresh, other} {
		if err := os.WriteFile(p, []byte("alice@shop.test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for path, mtime := range map[string]time.Time{stale: start.Add(-2 * staleTemp), fresh: start.Add(-time.Second), other: start.Add(-48 * time.Hour)} {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{stale: false, fresh: true, other: true} {
		if _, err := os.Stat(path); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", filepath.Base(path), err == nil, want)
		}
	}
}

func TestDisabled(t *testing.T) {
	t.Parallel()

	s, _ := newStore(t, Options{Disabled: true})
	if err := s.Save(pairs); !errors.Is(err, ErrDisabled) {
		t.Errorf("Save() = %v, want ErrDisabled", err)
	}
	if _, err := os.Stat(s.dir); !errors.Is(err, os.ErrNotExist) {
		t.Error("a disabled store created its directory")
	}

	// A mapping saved before storage was turned off can still be read
	enabled := New(Options{Dir: s.dir, Now: s.now})
	if err := enabled.Save(pairs); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(); err != nil || !reflect.DeepEqual(got, pairs) {
		t.Errorf("Load() on a disabled store = %v, %v", got, err)
	}
}

func TestForget(t *testing.T) {
	t.Parallel()

	s, _ := newStore(t, Options{})
	if err := s.Forget(); err != nil {
		t.Errorf("Forget() with no directory = %v", err)
	}
	if err := s.Save(pairs); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(s.dir, tempPrefix+"x")
	other := filepath.Join(s.dir, "notes.txt")
	for _, p := range []string{temp, other} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Forget(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{filepath.Join(s.dir, fileName): false, temp: false, other: true} {
		if _, err := os.Stat(path); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", filepath.Base(path), err == nil, want)
		}
	}
	if got, err := s.Load(); err != nil || got != nil {
		t.Errorf("Load() after Forget() = %v, %v", got, err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// A directory where the store expects a file cannot be removed or read
	blocked := filepath.Join(root, "blocked")
	for _, p := range []string{filepath.Join(blocked, fileName, "x"), filepath.Join(blocked, tempPrefix+"dir", "x")} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	old := start.Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(blocked, tempPrefix+"dir"), old, old); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(root, "unreadable")
	if err := os.MkdirAll(filepath.Join(unreadable, fileName, "x"), 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		dir  string
		call func(*Store) error
	}{
		{"load from a file", file, func(s *Store) error { _, err := s.Load(); return err }},
		{"save under a file", filepath.Join(file, "cuddio"), func(s *Store) error { return s.Save(pairs) }},
		{"save with no directory", "", func(s *Store) error { return s.Save(pairs) }},
		{"forget in a file", file, func(s *Store) error { return s.Forget() }},
		{"temp file cannot be removed", blocked, func(s *Store) error { _, err := s.Load(); return err }},
		{"forget cannot remove", blocked, func(s *Store) error { return s.Forget() }},
		{"mapping cannot be read", unreadable, func(s *Store) error { _, err := s.Load(); return err }},
		{"mapping cannot be replaced", unreadable, func(s *Store) error { return s.Save(pairs) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(New(Options{Dir: tt.dir, Now: (&clock{t: start}).now}))
			if err == nil || !strings.HasPrefix(err.Error(), "mapping store: ") {
				t.Errorf("error = %v, want a mapping store error", err)
			}
		})
	}
}

func TestDefaultDir(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG_CACHE_HOME only steers os.UserCacheDir on Linux")
	}

	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	if got, err := DefaultDir(); err != nil || got != filepath.Join(cache, "cuddio") {
		t.Errorf("DefaultDir() = %q, %v", got, err)
	}

	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := DefaultDir(); err == nil || !strings.HasPrefix(err.Error(), "mapping store: ") {
		t.Errorf("DefaultDir() with no home = %v, want an error", err)
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	s := New(Options{Dir: "x"})
	if s.ttl != DefaultTTL || s.now == nil {
		t.Errorf("New() defaults = ttl %v, clock set %v", s.ttl, s.now != nil)
	}
}

func TestReadOnlyDirectory(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions that the current user cannot override")
	}

	s, _ := newStore(t, Options{})
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, fileName), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.dir, 0o700) })

	if _, err := s.Load(); err == nil || errors.Is(err, ErrCorrupt) {
		t.Errorf("Load() = %v, want an error deleting the corrupt file", err)
	}

	child := New(Options{Dir: filepath.Join(s.dir, "cuddio")})
	if err := child.Save(pairs); err == nil || !strings.HasPrefix(err.Error(), "mapping store: ") {
		t.Errorf("Save() under a read-only folder = %v, want a mapping store error", err)
	}
}
