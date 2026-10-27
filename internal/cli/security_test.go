package cli

import (
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phoenix1331/cuddio/internal/config"
)

// The values testRules match in leakyInput. None of them may appear in
// anything cuddio prints or writes.
var leakyValues = []string{"alice@shop.test", "tok_s3cr3t", "bob@shop.test"}

const leakyInput = "login alice@shop.test with tok_s3cr3t\r\nmail bob@shop.test\n"

// assertNoLeak fails when any of values appears in outputs.
func assertNoLeak(t *testing.T, values []string, outputs ...string) {
	t.Helper()

	for _, out := range outputs {
		for _, v := range values {
			if strings.Contains(out, v) {
				t.Errorf("output repeats %q: %q", v, out)
			}
		}
	}
}

// fileState is what must not change about an input file.
type fileState struct {
	sum     [sha256.Size]byte
	mode    fs.FileMode
	modTime time.Time
}

func stateOf(t *testing.T, path string) fileState {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}

	return fileState{sum: sha256.Sum256([]byte(readFile(t, path))), mode: info.Mode(), modTime: info.ModTime()}
}

// failingReader returns data, then err, like a pipe that breaks mid-stream.
type failingReader struct {
	data string
	err  error
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.data == "" {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]

	return n, nil
}

func TestCleanNeverLeaksOrChangesInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// args builds the arguments from the input path and a free output path
		args     func(input, output string) []string
		config   string
		setup    func(t *testing.T, input, output string)
		stdin    func(input string) io.Reader
		wantCode int
	}{
		{name: "file to stdout", args: func(in, _ string) []string { return []string{in} }},
		{name: "file to output", args: func(in, out string) []string { return []string{in, "-o", out} }},
		{
			name:  "replace output with --force",
			args:  func(in, out string) []string { return []string{in, "-o", out, "--force"} },
			setup: func(t *testing.T, _, out string) { writeFile(t, out, "old\n") },
		},
		{
			name: "redirected stdin to output",
			args: func(_, out string) []string { return []string{"-o", out} },
			stdin: func(in string) io.Reader {
				f, err := os.Open(in)
				if err != nil {
					panic(err)
				}
				return f
			},
		},
		{
			name:     "output exists without --force",
			args:     func(in, out string) []string { return []string{in, "-o", out} },
			setup:    func(t *testing.T, _, out string) { writeFile(t, out, "old\n") },
			wantCode: exitError,
		},
		{name: "output is the input", args: func(in, _ string) []string { return []string{in, "-o", in, "--force"} }, wantCode: exitError},
		{
			name: "output is a hard link to the input",
			args: func(in, out string) []string { return []string{in, "-o", out, "--force"} },
			setup: func(t *testing.T, in, out string) {
				if err := os.Link(in, out); err != nil {
					t.Skipf("hard links unavailable: %v", err)
				}
			},
			wantCode: exitError,
		},
		{
			name: "output is a symlink to the input",
			args: func(in, out string) []string { return []string{in, "-o", out, "--force"} },
			setup: func(t *testing.T, in, out string) {
				if err := os.Symlink(in, out); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
			wantCode: exitError,
		},
		{name: "output directory missing", args: func(in, out string) []string { return []string{in, "-o", filepath.Join(out, "x", "y")} }, wantCode: exitError},
		{name: "sql format", args: func(in, _ string) []string { return []string{in, "--format", "sql"} }, wantCode: exitError},
		{name: "fake mode from config", args: func(in, _ string) []string { return []string{in} }, config: "mode: fake\n"},
		{name: "fake mode to output", args: func(in, out string) []string { return []string{in, "--mode", "fake", "-o", out} }},
		{name: "invalid mode", args: func(in, _ string) []string { return []string{in, "--mode", "alice@shop.test"} }, wantCode: exitError},
		{
			name:     "config with values in the wrong places",
			args:     func(in, _ string) []string { return []string{in} },
			config:   "allowlist:\n  alice@shop.test: true\nrules:\n  enable: [tok_s3cr3t]\n",
			wantCode: exitError,
		},
		{name: "unknown rule from config", args: func(in, _ string) []string { return []string{in} }, config: "rules:\n  disable: [bob@shop.test]\n", wantCode: exitError},
		{
			name:     "invalid custom rule",
			args:     func(in, _ string) []string { return []string{in} },
			config:   "rules:\n  custom:\n    - {id: 'tok_s3cr3t', category: 'alice@shop.test', label: X, pattern: 'bob@shop.test('}\n",
			wantCode: exitError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			input := filepath.Join(dir, "app.log")
			output := filepath.Join(dir, "out.log")
			writeFile(t, input, leakyInput)
			writeConfig(t, filepath.Join(dir, config.ProjectFile), tt.config)
			if tt.setup != nil {
				tt.setup(t, input, output)
			}
			var stdin io.Reader
			if tt.stdin != nil {
				stdin = tt.stdin(input)
				if c, ok := stdin.(io.Closer); ok {
					t.Cleanup(func() { c.Close() })
				}
			}
			before := stateOf(t, input)

			got := execute(t, Env{Rules: testRules, WorkDir: dir, Args: tt.args(input, output), Stdin: stdin})

			if got.code != tt.wantCode {
				t.Errorf("exit code = %d (stderr %q), want %d", got.code, got.stderr, tt.wantCode)
			}
			if tt.wantCode == exitOK && got.stderr != "" {
				t.Errorf("stderr = %q, want nothing on success", got.stderr)
			}
			assertNoLeak(t, leakyValues, got.stdout, got.stderr)
			if stateOf(t, input) != before {
				t.Error("input file changed")
			}
			if content, err := os.ReadFile(output); err == nil && tt.wantCode == exitOK {
				assertNoLeak(t, leakyValues, string(content))
			}
		})
	}
}

func TestCleanStreamErrorsNeverLeak(t *testing.T) {
	t.Parallel()

	long := "tok_s3cr3t " + strings.Repeat("x", 2<<20) + " alice@shop.test\n"

	tests := []struct {
		name  string
		stdin io.Reader
		want  string
	}{
		{"read fails after a secret", &failingReader{data: leakyInput + "tok_s3cr3t", err: errors.New("broken pipe")}, "cuddio: read: broken pipe\n"},
		{"line too long after clean lines", strings.NewReader(leakyInput + long), "cuddio: line 3 is longer than 1048576 bytes\n"},
		{"line too long with a secret in it", strings.NewReader(long), "cuddio: line 1 is longer than 1048576 bytes\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, Env{Rules: testRules, Stdin: tt.stdin})

			if got.code != exitError || got.stderr != tt.want {
				t.Errorf("clean = %d %q, want %d %q", got.code, got.stderr, exitError, tt.want)
			}
			assertNoLeak(t, leakyValues, got.stdout, got.stderr)
		})
	}
}

// leakySQL holds leakyValues in string values, a sensitive column and a
// comment, in a form every dialect reads.
const leakySQL = "-- from bob@shop.test\nINSERT INTO users (email, notes) VALUES ('alice@shop.test', 'key tok_s3cr3t');\n"

func TestCleanSQLNeverLeaksOrChangesInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     func(input, output string) []string
		wantCode int
	}{
		{name: "detected file to stdout", args: func(in, _ string) []string { return []string{in} }},
		{name: "file to output", args: func(in, out string) []string { return []string{in, "-o", out} }},
		{name: "mysql", args: func(in, _ string) []string { return []string{in, "--dialect", "mysql"} }},
		{name: "postgres", args: func(in, _ string) []string { return []string{in, "--dialect", "postgres"} }},
		{name: "sqlite", args: func(in, _ string) []string { return []string{in, "--dialect", "sqlite"} }},
		{name: "invalid dialect", args: func(in, _ string) []string { return []string{in, "--dialect", "alice@shop.test"} }, wantCode: exitError},
		{name: "undetectable", args: func(in, _ string) []string { return []string{in, "--format", "sql"} }, wantCode: exitError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			input := filepath.Join(dir, "dump.sql")
			output := filepath.Join(dir, "clean.sql")
			content := "PRAGMA foreign_keys=OFF;\n" + leakySQL
			if tt.name == "undetectable" {
				content = leakySQL
			}
			writeFile(t, input, content)
			before := stateOf(t, input)

			got := execute(t, Env{Rules: testRules, Args: tt.args(input, output)})

			if got.code != tt.wantCode {
				t.Errorf("exit code = %d (stderr %q), want %d", got.code, got.stderr, tt.wantCode)
			}
			assertNoLeak(t, leakyValues, got.stdout, got.stderr)
			if stateOf(t, input) != before {
				t.Error("input file changed")
			}
			if out, err := os.ReadFile(output); err == nil {
				assertNoLeak(t, leakyValues, string(out))
			}
		})
	}
}

func TestCleanSQLStreamErrorsNeverLeak(t *testing.T) {
	t.Parallel()

	long := "-- MySQL dump\nSELECT 'tok_s3cr3t " + strings.Repeat("x", 17<<20) + "';\n"
	tests := []struct {
		name  string
		stdin io.Reader
		want  string
	}{
		{"read fails after a secret", &failingReader{data: "-- MySQL dump\n" + leakySQL + "SELECT 'tok_s3cr3t", err: errors.New("broken pipe")}, "cuddio: read: broken pipe\n"},
		{"value too long", strings.NewReader(long), "cuddio: token at line 2 is longer than 16777216 bytes\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, Env{Rules: testRules, Args: []string{"--format", "sql"}, Stdin: tt.stdin})

			if got.code != exitError || got.stderr != tt.want {
				t.Errorf("clean = %d %q, want %d %q", got.code, got.stderr, exitError, tt.want)
			}
			assertNoLeak(t, leakyValues, got.stdout, got.stderr)
		})
	}
}

// utf16 encodes s as UTF-16 with a byte order mark, as Windows PowerShell
// 5.1 writes files.
func utf16(s string, bigEndian bool) string {
	var b strings.Builder
	if bigEndian {
		b.WriteString("\xfe\xff")
	} else {
		b.WriteString("\xff\xfe")
	}
	for _, r := range s {
		hi, lo := byte(r>>8), byte(r)
		if bigEndian {
			hi, lo = lo, hi
		}
		b.WriteByte(lo)
		b.WriteByte(hi)
	}

	return b.String()
}

func TestUTF16InputIsRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		args func(dir string) []string
	}{
		{"file, little endian", utf16(leakyInput, false), func(dir string) []string { return []string{filepath.Join(dir, "in.log")} }},
		{"file, big endian, to output", utf16(leakyInput, true), func(dir string) []string {
			return []string{filepath.Join(dir, "in.log"), "-o", filepath.Join(dir, "out.log")}
		}},
		{"check", utf16(leakyInput, false), func(dir string) []string { return []string{filepath.Join(dir, "in.log"), "--check"} }},
		{"sql", utf16("INSERT INTO t (email) VALUES ('alice@shop.test');", false), func(dir string) []string {
			return []string{filepath.Join(dir, "in.log"), "--dialect", "mysql"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "in.log"), tt.in)
			got := execute(t, Env{Rules: testRules, Args: tt.args(dir)})

			if got.code != exitError || got.stdout != "" || !strings.Contains(got.stderr, "UTF-16") || !strings.Contains(got.stderr, "Set-Content -Encoding utf8") {
				t.Errorf("clean = %d %q %q, want a UTF-16 error", got.code, got.stdout, got.stderr)
			}
			if _, err := os.Stat(filepath.Join(dir, "out.log")); err == nil {
				t.Error("an output file was created")
			}
		})
	}

	got := execute(t, Env{Rules: testRules, Stdin: strings.NewReader(utf16(leakyInput, false))})
	if got.code != exitError || got.stdout != "" {
		t.Errorf("UTF-16 on stdin = %d %q", got.code, got.stdout)
	}
}

func TestUTF8WithBOMIsCleaned(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"\xef\xbb\xbf" + leakyInput, "\xff", "\xfe"} {
		got := execute(t, Env{Rules: testRules, Stdin: strings.NewReader(in)})
		if got.code != exitOK {
			t.Errorf("clean %q = %d %q", in[:1], got.code, got.stderr)
		}
		assertNoLeak(t, leakyValues, got.stdout)
	}
}
