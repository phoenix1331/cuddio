package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ProjectFile is the name of the per-project config file.
const ProjectFile = ".cuddio.yaml"

// Sources says where to look for config files.
type Sources struct {
	// WorkDir is the absolute directory where the search for ProjectFile
	// starts. It walks up to the nearest directory holding .git, or the
	// filesystem root.
	WorkDir string
	// GlobalDir is the user config directory (os.UserConfigDir); empty skips
	// the global file.
	GlobalDir string
	// Explicit is a file named with --config. It replaces the project search.
	Explicit string
}

// GlobalFile returns the global config path inside a user config directory.
func GlobalFile(userConfigDir string) string {
	return filepath.Join(userConfigDir, "cuddio", "config.yaml")
}

// Load merges the defaults, the global file and the project (or explicit)
// file, in that order. Missing global and project files are skipped; a
// missing explicit file is an error.
func Load(src Sources) (Config, error) {
	cfg := Default()

	if src.GlobalDir != "" {
		global, err := readOptional(GlobalFile(src.GlobalDir))
		if err != nil {
			return Config{}, err
		}
		cfg = merge(cfg, global)
	}

	if src.Explicit != "" {
		explicit, err := read(src.Explicit)
		if err != nil {
			return Config{}, err
		}
		return merge(cfg, explicit), nil
	}

	path := findProject(src.WorkDir)
	if path == "" {
		return cfg, nil
	}
	project, err := read(path)
	if err != nil {
		return Config{}, err
	}

	return merge(cfg, project), nil
}

// findProject returns the nearest ProjectFile at or above dir, stopping at
// the directory that holds .git. It returns "" when there is none.
func findProject(dir string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)

	for {
		candidate := filepath.Join(dir, ProjectFile)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return ""
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func readOptional(path string) (Config, error) {
	cfg, err := read(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}

	return cfg, err
}

func read(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // reading the user's own config file is the point
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}

	return decode(path, data)
}

func decode(path string, data []byte) (Config, error) {
	var cfg Config

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	err := decoder.Decode(&cfg)
	if errors.Is(err, io.EOF) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %s", path, describe(err))
	}

	return cfg, nil
}

var (
	// quotedValue matches the start of a value yaml quotes in its type errors.
	quotedValue = regexp.MustCompile("`[^`]*`")
	// unknownField matches yaml's strict-mode message for a key with no struct field.
	unknownField = regexp.MustCompile(`field (\S+) not found in type \S+`)
)

// describe turns a yaml error into a message that never repeats a value from
// the file: type errors quote the start of the offending value, which may be
// an allowlisted secret.
func describe(err error) string {
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return strings.TrimPrefix(err.Error(), "yaml: ")
	}

	lines := make([]string, len(typeErr.Errors))
	for i, line := range typeErr.Errors {
		line = quotedValue.ReplaceAllString(line, "a value")
		lines[i] = unknownField.ReplaceAllString(line, "unknown setting $1")
	}

	return strings.Join(lines, "; ")
}
