// Package mapstore keeps the placeholder mapping from a run that used
// --map, so restore can put real values back. It writes to one place only,
// a cuddio folder in the user's cache directory, with owner-only
// permissions, and deletes what it wrote once the TTL has passed.
package mapstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/phoenix1331/cuddio/internal/replace"
)

// DefaultTTL is how long a mapping lasts when Options sets no TTL.
const DefaultTTL = time.Hour

const (
	fileName   = "mapping.json"
	tempPrefix = ".mapping-"
	// staleTemp is how old a temporary file must be before purge treats it
	// as left behind by a run that crashed mid-write.
	staleTemp = time.Minute
	version   = 1
)

// ErrDisabled is returned by Save when config turns mapping storage off.
var ErrDisabled = errors.New("mapping storage is disabled in config")

// ErrCorrupt is returned by Load when the mapping file cannot be read as a
// mapping. The file is deleted.
var ErrCorrupt = errors.New("the stored mapping was unreadable and has been deleted; clean the input again with --map")

// Options configures a Store. Zero values use the defaults.
type Options struct {
	// Dir is the folder the mapping lives in, normally DefaultDir.
	Dir string
	// TTL is how long a mapping lasts after it was last saved.
	TTL time.Duration
	// Disabled makes Save refuse to store anything.
	Disabled bool
	// Now is the clock, for tests.
	Now func() time.Time
}

// Store reads and writes the mapping for the current session.
type Store struct {
	dir      string
	ttl      time.Duration
	disabled bool
	now      func() time.Time
}

// New returns a store for opts. Nothing touches the disk until it is used.
func New(opts Options) *Store {
	s := &Store{dir: opts.Dir, ttl: opts.TTL, disabled: opts.Disabled, now: opts.Now}
	if s.ttl <= 0 {
		s.ttl = DefaultTTL
	}
	if s.now == nil {
		s.now = time.Now
	}

	return s
}

// DefaultDir returns the cuddio folder in the user's cache directory:
// ~/.cache/cuddio, ~/Library/Caches/cuddio or %LocalAppData%\cuddio.
func DefaultDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("mapping store: %w", err)
	}

	return filepath.Join(cache, "cuddio"), nil
}

// record is the file format.
type record struct {
	Version int       `json:"version"`
	SavedAt time.Time `json:"saved_at"`
	Pairs   []pair    `json:"pairs"`
}

type pair struct {
	Token string `json:"token"`
	Value string `json:"value"`
}

// Load returns the stored pairs, or none when there is no mapping or it
// has expired. Expired, corrupt and left-behind files are deleted first.
func (s *Store) Load() ([]replace.Pair, error) {
	rec, err := s.purge()
	if err != nil || rec == nil {
		return nil, err
	}

	pairs := make([]replace.Pair, len(rec.Pairs))
	for i, p := range rec.Pairs {
		pairs[i] = replace.Pair{Token: p.Token, Value: p.Value}
	}

	return pairs, nil
}

// Save replaces the stored mapping with pairs, stamped with the current
// time. The file is written to a temporary file, which os.CreateTemp
// creates with mode 0600, and renamed into place, so a reader never sees
// half a mapping. On Windows, where Go
// can only set the read-only bit, the file relies on the per-user ACL of
// the cache directory instead.
func (s *Store) Save(pairs []replace.Pair) error {
	if s.disabled {
		return ErrDisabled
	}
	if _, err := s.purge(); err != nil && !errors.Is(err, ErrCorrupt) {
		return err
	}
	if err := s.ensureDir(); err != nil {
		return err
	}

	rec := record{Version: version, SavedAt: s.now().UTC(), Pairs: make([]pair, len(pairs))}
	for i, p := range pairs {
		rec.Pairs[i] = pair{Token: p.Token, Value: p.Value}
	}
	// Strings and a UTC time always marshal
	data, _ := json.Marshal(rec)

	return s.writeAtomic(data)
}

// Forget deletes every file the store has written, live or not.
func (s *Store) Forget() error {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("mapping store: %w", err)
	}
	for _, e := range entries {
		if ours(e.Name()) {
			if err := remove(filepath.Join(s.dir, e.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

// purge deletes temporary files left by crashed writes, then reads the
// mapping file and deletes it if it has expired or cannot be read. It
// returns the live record, or nil.
func (s *Store) purge() (*record, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mapping store: %w", err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), tempPrefix) {
			continue
		}
		info, err := e.Info()
		if err == nil && s.now().Sub(info.ModTime()) < staleTemp {
			continue // another run may be writing it now
		}
		if err := remove(filepath.Join(s.dir, e.Name())); err != nil {
			return nil, err
		}
	}

	path := filepath.Join(s.dir, fileName)
	data, err := os.ReadFile(path) //nolint:gosec // G304: the path is cuddio's own cache file
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mapping store: %w", err)
	}

	var rec record
	// The decode error is dropped: it can quote bytes of the file, which
	// hold real values
	if json.Unmarshal(data, &rec) != nil || rec.Version != version || rec.SavedAt.IsZero() {
		if err := remove(path); err != nil {
			return nil, err
		}
		return nil, ErrCorrupt
	}
	if !s.now().Before(rec.SavedAt.Add(s.ttl)) {
		return nil, remove(path)
	}

	return &rec, nil
}

// ensureDir creates the store's folder with mode 0700, and narrows an
// existing one to 0700, since it is cuddio's own.
func (s *Store) ensureDir() error {
	if s.dir == "" {
		return errors.New("mapping store: no directory")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("mapping store: %w", err)
	}
	if err := os.Chmod(s.dir, 0o700); err != nil { //nolint:gosec // G302: a directory needs its execute bit; 0700 is owner-only
		return fmt.Errorf("mapping store: %w", err)
	}

	return nil
}

func (s *Store) writeAtomic(data []byte) error {
	tmp, err := os.CreateTemp(s.dir, tempPrefix+"*")
	if err != nil {
		return fmt.Errorf("mapping store: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op once renamed

	_, writeErr := tmp.Write(data)
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return fmt.Errorf("mapping store: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(s.dir, fileName)); err != nil {
		return fmt.Errorf("mapping store: %w", err)
	}

	return nil
}

// ours reports whether name is a file the store writes.
func ours(name string) bool {
	return name == fileName || strings.HasPrefix(name, tempPrefix)
}

func remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("mapping store: %w", err)
	}

	return nil
}
