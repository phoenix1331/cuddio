package cli

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/phoenix1331/cuddio/internal/config"
	"github.com/phoenix1331/cuddio/internal/detect"
)

// testRules stand in for the built-in rules, which the CLI receives from main.
var testRules = []detect.Rule{
	{ID: "email", Category: detect.PII, Label: "EMAIL", Pattern: regexp.MustCompile(`[a-z]+@[a-z]+\.[a-z]+`)},
	{ID: "token", Category: detect.Secret, Label: "TOKEN", Pattern: regexp.MustCompile(`tok_[a-z0-9]+`)},
}

// fileInfo is a minimal fs.FileInfo for fake stdin.
type fileInfo struct{ mode fs.FileMode }

func (fileInfo) Name() string             { return "stdin" }
func (fileInfo) Size() int64              { return 0 }
func (i fileInfo) Mode() fs.FileMode      { return i.mode }
func (fileInfo) ModTime() time.Time       { return time.Time{} }
func (i fileInfo) IsDir() bool            { return i.mode.IsDir() }
func (fileInfo) Sys() any                 { return nil }
func (terminal) Read([]byte) (int, error) { return 0, io.EOF }

// terminal is stdin attached to an interactive terminal.
type terminal struct{}

func (terminal) Stat() (fs.FileInfo, error) {
	return fileInfo{mode: fs.ModeDevice | fs.ModeCharDevice}, nil
}

// unstattable is stdin whose Stat fails, so cuddio treats it as a pipe.
type unstattable struct{ io.Reader }

func (unstattable) Stat() (fs.FileInfo, error) { return nil, errors.New("no stat") }

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func TestCleanStdinToStdout(t *testing.T) {
	t.Parallel()

	in := "from alice@shop.test tok_abc123\r\nto bob@shop.test cc alice@shop.test\r\nno newline"
	want := "from <EMAIL_1> <TOKEN_1>\r\nto <EMAIL_2> cc <EMAIL_1>\r\nno newline"

	for _, stdin := range []io.Reader{strings.NewReader(in), unstattable{strings.NewReader(in)}} {
		got := execute(t, Env{Rules: testRules, Stdin: stdin})

		if got.code != exitOK || got.stdout != want || got.stderr != "" {
			t.Errorf("clean stdin = %d %q %q, want %d %q with no stderr", got.code, got.stdout, got.stderr, exitOK, want)
		}
	}
}

func TestCleanFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	input := filepath.Join(dir, "app.log")
	writeFile(t, input, "user alice@shop.test\n")

	got := execute(t, Env{Rules: testRules, Args: []string{input}})

	if got.code != exitOK || got.stdout != "user <EMAIL_1>\n" {
		t.Errorf("clean file = %d %q (stderr %q), want %d %q", got.code, got.stdout, got.stderr, exitOK, "user <EMAIL_1>\n")
	}
	if readFile(t, input) != "user alice@shop.test\n" {
		t.Error("input file was modified")
	}
}

func TestCleanAppliesConfig(t *testing.T) {
	t.Parallel()

	in := "alice@shop.test ok@shop.test tok_abc\n"

	tests := []struct {
		name   string
		config string
		want   string
	}{
		{"placeholder format", "placeholder:\n  format: '[{label}#{n}]'\n", "[EMAIL#1] [EMAIL#2] [TOKEN#1]\n"},
		{"rule disabled by ID", "rules:\n  disable: [token]\n", "<EMAIL_1> <EMAIL_2> tok_abc\n"},
		{"category disabled", "rules:\n  disable: [pii]\n", "alice@shop.test ok@shop.test <TOKEN_1>\n"},
		{"allowlist value", "allowlist:\n  values: [ok@shop.test]\n", "<EMAIL_1> ok@shop.test <TOKEN_1>\n"},
		{"allowlist pattern", "allowlist:\n  patterns: ['^tok_']\n", "<EMAIL_1> <EMAIL_2> tok_abc\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeConfig(t, filepath.Join(dir, config.ProjectFile), tt.config)

			got := execute(t, Env{Rules: testRules, WorkDir: dir, Stdin: strings.NewReader(in)})

			if got.code != exitOK || got.stdout != tt.want {
				t.Errorf("clean = %d %q (stderr %q), want %d %q", got.code, got.stdout, got.stderr, exitOK, tt.want)
			}
		})
	}
}

func TestCleanSetupErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rules  []detect.Rule
		config string
		want   string
	}{
		{"unknown rule in config", testRules, "rules:\n  enable: [nope]\n", `cuddio: rules: unknown rule or category "nope"` + "\n"},
		{"invalid allowlist pattern", testRules, "allowlist:\n  patterns: ['tok_(secret']\n", "cuddio: allowlist pattern 1: invalid pattern: missing closing )\n"},
		{"placeholder format without {n}", testRules, "placeholder:\n  format: '<{label}>'\n", "cuddio: placeholder format must contain {label} and {n}\n"},
		{"invalid rule", []detect.Rule{{ID: "Bad"}}, "", `cuddio: rule "Bad": id must be lowercase letters, digits and single hyphens` + "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeConfig(t, filepath.Join(dir, config.ProjectFile), tt.config)

			got := execute(t, Env{Rules: tt.rules, WorkDir: dir, Stdin: strings.NewReader("tok_secret\n")})

			if got.code != exitError || got.stderr != tt.want || got.stdout != "" {
				t.Errorf("clean = %d %q %q, want %d, no stdout, stderr %q", got.code, got.stdout, got.stderr, exitError, tt.want)
			}
		})
	}
}

func TestCleanFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dump := filepath.Join(dir, "dump.SQL")
	writeFile(t, dump, "alice@shop.test\n")

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"text flag", []string{"--format", "text", dump}, exitOK, "<EMAIL_1>\n", ""},
		{"sql flag", []string{"--format", "sql"}, exitError, "", "cuddio: sql format is not supported yet\n"},
		{"sql by extension", []string{dump}, exitError, "", "cuddio: sql format is not supported yet\n"},
		{"invalid flag", []string{"--format", "csv"}, exitError, "", `cuddio: invalid --format "csv": want text or sql` + "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, Env{Rules: testRules, Args: tt.args})

			if got.code != tt.wantCode || got.stdout != tt.wantStdout || got.stderr != tt.wantStderr {
				t.Errorf("clean = %d %q %q, want %d %q %q", got.code, got.stdout, got.stderr, tt.wantCode, tt.wantStdout, tt.wantStderr)
			}
		})
	}
}

func TestCleanInputErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	tests := []struct {
		name string
		path string
		want string
	}{
		{"missing file", filepath.Join(dir, "missing.log"), "cuddio: open " + filepath.Join(dir, "missing.log") + ": "},
		{"directory", dir, "cuddio: " + dir + " is a directory\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, Env{Args: []string{tt.path}})

			if got.code != exitError || !strings.HasPrefix(got.stderr, tt.want) {
				t.Errorf("clean = %d %q, want %d and stderr starting %q", got.code, got.stderr, exitError, tt.want)
			}
		})
	}
}

func TestCleanLongLineFailsWithoutLeaking(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	output := filepath.Join(dir, "out.log")
	in := "tok_" + strings.Repeat("s", 2<<20) + "\n"

	got := execute(t, Env{Rules: testRules, Args: []string{"-o", output}, Stdin: strings.NewReader(in)})

	if got.code != exitError || got.stderr != "cuddio: line 1 is longer than 1048576 bytes\n" {
		t.Errorf("clean = %d %q, want %d and a line length error", got.code, got.stderr, exitError)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("failed run left files behind: %v", entries)
	}
}

func TestCleanOutput(t *testing.T) {
	t.Parallel()

	t.Run("new file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		input := filepath.Join(dir, "app.log")
		output := filepath.Join(dir, "app.clean.log")
		writeFile(t, input, "alice@shop.test\r\n")

		got := execute(t, Env{Rules: testRules, Args: []string{input, "-o", output}})

		if got.code != exitOK || got.stdout != "" || got.stderr != "" {
			t.Fatalf("clean -o = %d %q %q, want %d and no output", got.code, got.stdout, got.stderr, exitOK)
		}
		if content := readFile(t, output); content != "<EMAIL_1>\r\n" {
			t.Errorf("output file = %q, want %q", content, "<EMAIL_1>\r\n")
		}
	})

	t.Run("existing file needs --force", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		output := filepath.Join(dir, "out.log")
		writeFile(t, output, "keep me\n")

		got := execute(t, Env{Rules: testRules, Args: []string{"--output", output}, Stdin: strings.NewReader("alice@shop.test\n")})

		want := "cuddio: output " + output + " exists; use --force to replace it\n"
		if got.code != exitError || got.stderr != want {
			t.Errorf("clean -o existing = %d %q, want %d %q", got.code, got.stderr, exitError, want)
		}
		if readFile(t, output) != "keep me\n" {
			t.Error("existing output was changed without --force")
		}

		got = execute(t, Env{Rules: testRules, Args: []string{"--output", output, "--force"}, Stdin: strings.NewReader("alice@shop.test\n")})

		if got.code != exitOK || readFile(t, output) != "<EMAIL_1>\n" {
			t.Errorf("clean -o existing --force = %d %q, output %q", got.code, got.stderr, readFile(t, output))
		}
	})

	t.Run("output directory missing", func(t *testing.T) {
		t.Parallel()

		output := filepath.Join(t.TempDir(), "missing", "out.log")

		got := execute(t, Env{Args: []string{"-o", output}})

		if got.code != exitError || !strings.HasPrefix(got.stderr, "cuddio: output: ") {
			t.Errorf("clean -o into a missing directory = %d %q, want %d and an output error", got.code, got.stderr, exitError)
		}
	})

	t.Run("output is a directory", func(t *testing.T) {
		t.Parallel()

		output := t.TempDir()

		got := execute(t, Env{Args: []string{"-o", output, "--force"}})

		if got.code != exitError || !strings.HasPrefix(got.stderr, "cuddio: output: ") {
			t.Errorf("clean -o DIR --force = %d %q, want %d and an output error", got.code, got.stderr, exitError)
		}
	})

	t.Run("output path under a file", func(t *testing.T) {
		t.Parallel()

		file := filepath.Join(t.TempDir(), "file")
		writeFile(t, file, "")

		got := execute(t, Env{Args: []string{"-o", filepath.Join(file, "out.log")}})

		if got.code != exitError {
			t.Errorf("clean -o FILE/out.log = %d %q, want %d", got.code, got.stderr, exitError)
		}
	})
}

func TestCleanRefusesToOverwriteInput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	input := filepath.Join(dir, "app.log")
	link := filepath.Join(dir, "link.log")
	writeFile(t, input, "alice@shop.test\n")
	if err := os.Link(input, link); err != nil {
		t.Fatal(err)
	}

	stdin, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stdin.Close() })

	tests := []struct {
		name   string
		args   []string
		stdin  io.Reader
		output string
	}{
		{"same path", []string{input, "-o", input, "--force"}, nil, input},
		{"hard link to the input", []string{input, "-o", link, "--force"}, nil, link},
		{"stdin redirected from the output", []string{"-o", input, "--force"}, stdin, input},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execute(t, Env{Rules: testRules, Args: tt.args, Stdin: tt.stdin})

			want := "cuddio: output " + tt.output + " is the input file\n"
			if got.code != exitError || got.stderr != want {
				t.Errorf("clean = %d %q, want %d %q", got.code, got.stderr, exitError, want)
			}
			if readFile(t, input) != "alice@shop.test\n" {
				t.Fatal("input file was modified")
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		r    io.Reader
		want bool
	}{
		{"terminal", terminal{}, true},
		{"pipe", strings.NewReader(""), false},
		{"stat fails", unstattable{strings.NewReader("")}, false},
		{"regular file", fileStdin{}, false},
	}

	for _, tt := range tests {
		if got := isTerminal(tt.r); got != tt.want {
			t.Errorf("%s: isTerminal() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// fileStdin is stdin redirected from a regular file.
type fileStdin struct{ terminal }

func (fileStdin) Stat() (fs.FileInfo, error) { return fileInfo{}, nil }
