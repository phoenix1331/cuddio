package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/phoenix1331/cuddio/internal/names"
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
	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}

	return cfg, nil
}

var (
	// quotedValue matches the start of a value yaml quotes in its type errors.
	quotedValue = regexp.MustCompile("`[^`]*`")
	// unknownField matches yaml's strict-mode message for a key with no struct field.
	unknownField = regexp.MustCompile(`field (\S+) not found in type (\S+)`)
	// tag matches the tag yaml names in a type error, such as !!str.
	tag = regexp.MustCompile(`cannot unmarshal (!\S*)( a value)?`)
	// quotedName matches an anchor, key or other name yaml quotes in its errors.
	quotedName = regexp.MustCompile(` ?('[^']*'|"[^"]*")`)
)

// coreTags are the tags yaml gives plain values; any other tag in a type
// error was written in the file.
var coreTags = []string{"!!binary", "!!bool", "!!float", "!!int", "!!map", "!!merge", "!!null", "!!seq", "!!str", "!!timestamp"}

// describe turns a yaml error into a message that never repeats text from
// the file: type errors quote the start of the offending value, which may be
// an allowlisted secret, and a key, anchor or tag may be a value written in
// the wrong place. An unknown key is named only as a near miss of a setting.
func describe(err error) string {
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return quotedName.ReplaceAllString(strings.TrimPrefix(err.Error(), "yaml: "), "")
	}

	settings := settingNames(reflect.TypeFor[Config](), map[string][]string{})
	lines := make([]string, len(typeErr.Errors))
	for i, line := range typeErr.Errors {
		line = quotedValue.ReplaceAllString(line, "a value")
		line = quotedName.ReplaceAllString(line, "")
		line = tag.ReplaceAllStringFunc(line, func(m string) string {
			if slices.Contains(coreTags, tag.FindStringSubmatch(m)[1]) {
				return m
			}
			return "cannot unmarshal a value with an unknown tag"
		})
		lines[i] = unknownField.ReplaceAllStringFunc(line, func(m string) string {
			match := unknownField.FindStringSubmatch(m)
			if near, ok := names.Nearest(match[1], settings[match[2]]); ok {
				return fmt.Sprintf("unknown setting; did you mean %s?", near)
			}
			return "unknown setting"
		})
	}

	return strings.Join(lines, "; ")
}

// settingNames adds the yaml keys of t, and of the structs it contains, to
// keys under each struct's type name, as yaml names it in errors.
func settingNames(t reflect.Type, keys map[string][]string) map[string][]string {
	for field := range t.Fields() {
		key, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if key == "" || key == "-" {
			continue
		}
		keys[t.String()] = append(keys[t.String()], key)

		inner := field.Type
		for inner.Kind() == reflect.Slice || inner.Kind() == reflect.Map {
			inner = inner.Elem()
		}
		if inner.Kind() == reflect.Struct {
			settingNames(inner, keys)
		}
	}

	return keys
}
