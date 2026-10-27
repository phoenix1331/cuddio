package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phoenix1331/cuddio/internal/config"
)

func writeConfig(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// loadedConfig runs the root command with env and returns the config its
// commands would see.
func loadedConfig(t *testing.T, env Env) config.Config {
	t.Helper()

	env.Stdin = strings.NewReader("")
	env.Stdout = &bytes.Buffer{}
	env.Stderr = &bytes.Buffer{}
	a := newApp(env)
	a.root.SetArgs(append([]string{}, env.Args...))
	if err := a.root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	return a.config
}

func TestConfigSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	repo := filepath.Join(root, "repo")
	explicit := filepath.Join(root, "ci.yaml")
	writeConfig(t, config.GlobalFile(configDir), "mapping:\n  ttl: 10m\nplaceholder:\n  format: '[{label}#{n}]'\n")
	writeConfig(t, filepath.Join(repo, config.ProjectFile), "placeholder:\n  format: '<<{label}{n}>>'\n")
	writeConfig(t, explicit, "placeholder:\n  format: '[{label}:{n}]'\n")

	tests := []struct {
		name       string
		env        Env
		wantMode   string
		wantTTL    time.Duration
		wantFormat string
	}{
		{"no files", Env{}, "placeholder", time.Hour, "<{label}_{n}>"},
		{"global and project", Env{WorkDir: repo, ConfigDir: configDir}, "placeholder", 10 * time.Minute, "<<{label}{n}>>"},
		{"--config replaces the project file", Env{WorkDir: repo, ConfigDir: configDir, Args: []string{"--config", explicit}}, "placeholder", 10 * time.Minute, "[{label}:{n}]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := loadedConfig(t, tt.env)
			if cfg.Mode != tt.wantMode || cfg.Mapping.TTL != tt.wantTTL || cfg.Placeholder.Format != tt.wantFormat {
				t.Errorf("config = mode %q ttl %v format %q, want %q %v %q", cfg.Mode, cfg.Mapping.TTL, cfg.Placeholder.Format, tt.wantMode, tt.wantTTL, tt.wantFormat)
			}
		})
	}
}

func TestConfigErrorsExitWithoutLeakingValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	broken := filepath.Join(dir, config.ProjectFile)
	writeConfig(t, broken, "mapping:\n  ttl: sk_live_DO_NOT_ECHO_51Hc7\n")
	missing := filepath.Join(dir, "missing.yaml")

	tests := []struct {
		name string
		env  Env
		want string
	}{
		{"broken project file", Env{WorkDir: dir}, "cuddio: config " + broken + ": line 2: cannot unmarshal !!str a value into time.Duration\n"},
		{"missing --config file", Env{Args: []string{"--config", missing}}, "cuddio: config " + missing + ": "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, tt.env)

			if got.code != exitError {
				t.Errorf("exit code = %d, want %d", got.code, exitError)
			}
			if !strings.HasPrefix(got.stderr, tt.want) {
				t.Errorf("stderr = %q, want it to start with %q", got.stderr, tt.want)
			}
			if strings.Contains(got.stdout+got.stderr, "sk_live") {
				t.Errorf("output repeats a value from the config file: %q", got.stdout+got.stderr)
			}
		})
	}
}

func TestVersionIgnoresBrokenConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, config.ProjectFile), "not: [valid\n")

	got := execute(t, Env{Version: "v0.1.0", WorkDir: dir, Args: []string{"--version"}})

	if got.code != exitOK || got.stdout != "cuddio v0.1.0\n" {
		t.Errorf("--version with a broken config = %d %q, want %d %q", got.code, got.stdout, exitOK, "cuddio v0.1.0\n")
	}
}
