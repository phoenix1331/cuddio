package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLayers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	globalDir := filepath.Join(root, "config")
	repo := filepath.Join(root, "repo")
	writeFile(t, GlobalFile(globalDir), "mode: fake\nmapping:\n  ttl: 30m\nallowlist:\n  values: [test@example.com]\n")
	writeFile(t, filepath.Join(repo, ProjectFile), "mode: placeholder\nallowlist:\n  values: [ops@example.com]\n")
	writeFile(t, filepath.Join(root, "explicit.yaml"), "placeholder:\n  format: '[{label}]'\n")

	tests := []struct {
		name          string
		src           Sources
		wantMode      string
		wantTTL       time.Duration
		wantFormat    string
		wantAllowlist string
	}{
		{"defaults only", Sources{}, "placeholder", time.Hour, "<{label}_{n}>", ""},
		{"global only", Sources{GlobalDir: globalDir}, "fake", 30 * time.Minute, "<{label}_{n}>", "test@example.com"},
		{"project over global", Sources{GlobalDir: globalDir, WorkDir: repo}, "placeholder", 30 * time.Minute, "<{label}_{n}>", "test@example.com,ops@example.com"},
		{"explicit replaces project", Sources{GlobalDir: globalDir, WorkDir: repo, Explicit: filepath.Join(root, "explicit.yaml")}, "fake", 30 * time.Minute, "[{label}]", "test@example.com"},
		{"missing global is skipped", Sources{GlobalDir: filepath.Join(root, "nowhere"), WorkDir: repo}, "placeholder", time.Hour, "<{label}_{n}>", "ops@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(tt.src)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Mode != tt.wantMode || cfg.Mapping.TTL != tt.wantTTL || cfg.Placeholder.Format != tt.wantFormat {
				t.Errorf("Load() = mode %q ttl %v format %q, want %q %v %q", cfg.Mode, cfg.Mapping.TTL, cfg.Placeholder.Format, tt.wantMode, tt.wantTTL, tt.wantFormat)
			}
			if got := strings.Join(cfg.Allowlist.Values, ","); got != tt.wantAllowlist {
				t.Errorf("Allowlist.Values = %q, want %q", got, tt.wantAllowlist)
			}
		})
	}
}

func TestFindProject(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	nested := filepath.Join(repo, "app", "storage", "logs")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, ProjectFile), "mode: fake\n")

	inner := filepath.Join(root, "inner")
	if err := os.MkdirAll(filepath.Join(inner, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ProjectFile), "mode: fake\n")

	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"in the working directory", repo, filepath.Join(repo, ProjectFile)},
		{"found by walking up", nested, filepath.Join(repo, ProjectFile)},
		{"stops at the repository root", inner, ""},
		{"no working directory", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := findProject(tt.dir); got != tt.want {
				t.Errorf("findProject(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

func TestFindProjectStopsAtFilesystemRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if got := findProject(dir); got != "" && !strings.HasPrefix(dir, filepath.Dir(got)) {
		t.Errorf("findProject(%q) = %q, want no file or one above the temporary directory", dir, got)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Parallel()

	const secret = "sk_live_DO_NOT_ECHO_51Hc7"

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"unknown key", "unknown: " + secret + "\n", "line 1: unknown setting unknown"},
		{"unknown nested key", "rules:\n  disabled: [" + secret + "]\n", "line 2: unknown setting disabled"},
		{"wrong type", "mapping:\n  ttl: " + secret + "\n", "line 2: cannot unmarshal !!str a value into time.Duration"},
		{"list where a mapping belongs", "allowlist: [" + secret + "]\n", "line 1: cannot unmarshal !!seq into config.Allowlist"},
		{"invalid YAML", "mode: fake\n  bad: " + secret + "\n", "line 2: mapping values are not allowed in this context"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), ProjectFile)
			writeFile(t, path, tt.content)

			_, err := Load(Sources{Explicit: path})
			if err == nil {
				t.Fatal("Load() error = nil, want an error")
			}
			if got := err.Error(); !strings.Contains(got, tt.want) || !strings.HasPrefix(got, "config "+path+": ") {
				t.Errorf("Load() error = %q, want it to start with the path and contain %q", got, tt.want)
			}
			if strings.Contains(err.Error(), "sk_live") {
				t.Errorf("Load() error repeats a value from the file: %q", err)
			}
		})
	}
}

func TestLoadMissingExplicitFile(t *testing.T) {
	t.Parallel()

	_, err := Load(Sources{Explicit: filepath.Join(t.TempDir(), "missing.yaml")})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load() error = %v, want fs.ErrNotExist", err)
	}
}

func TestLoadUnreadableFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// A directory where a file is expected fails to read on every platform
	if err := os.MkdirAll(GlobalFile(root), 0o750); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ProjectFile), 0o750); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(Sources{GlobalDir: root}); err == nil {
		t.Error("Load() with an unreadable global file: error = nil")
	}
	if _, err := Load(Sources{WorkDir: repo}); err == nil {
		t.Error("Load() with an unreadable project file: error = nil")
	}
}

func TestLoadEmptyFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ProjectFile)
	writeFile(t, path, "# nothing set yet\n")

	cfg, err := Load(Sources{Explicit: path})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Mode != Default().Mode {
		t.Errorf("Mode = %q, want the default %q", cfg.Mode, Default().Mode)
	}
}
