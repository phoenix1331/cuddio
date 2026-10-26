package textscan

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// mark wraps every chunk it is given in brackets, so tests can see exactly
// how the input was split and what was passed to redact.
func mark(s string) string { return "[" + s + "]" }

func TestClean(t *testing.T) {
	t.Parallel()

	const key = "-----BEGIN PRIVATE KEY-----\nMIIB\nAbCd\n-----END PRIVATE KEY-----"

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"one line", "hello\n", "[hello]\n"},
		{"missing final newline", "a\nb", "[a]\n[b]"},
		{"CRLF kept and not passed to redact", "a\r\nb\r\n", "[a]\r\n[b]\r\n"},
		{"mixed endings", "a\r\nb\nc", "[a]\r\n[b]\n[c]"},
		{"lone CR stays in the line", "a\rb\n", "[a\rb]\n"},
		{"blank lines", "\n\na\n", "[]\n[]\n[a]\n"},
		{"PEM block is one chunk", "before\n" + key + "\nafter\n", "[before]\n[" + key + "]\n[after]\n"},
		{
			"PEM block with CRLF",
			"-----BEGIN PRIVATE KEY-----\r\nMIIB\r\n-----END PRIVATE KEY-----\r\n",
			"[-----BEGIN PRIVATE KEY-----\r\nMIIB\r\n-----END PRIVATE KEY-----]\r\n",
		},
		{"PEM block at end of input without newline", key, "[" + key + "]"},
		{"PEM block mid-line", `key="-----BEGIN EC PRIVATE KEY-----` + "\nMHc\n-----END EC PRIVATE KEY-----\"\n", "[key=\"-----BEGIN EC PRIVATE KEY-----\nMHc\n-----END EC PRIVATE KEY-----\"]\n"},
		{"single-line PEM block", "-----BEGIN X----- abc -----END X-----\nnext\n", "[-----BEGIN X----- abc -----END X-----]\n[next]\n"},
		{"unterminated PEM block passed whole", "-----BEGIN PRIVATE KEY-----\nMIIB\nAbCd\n", "[-----BEGIN PRIVATE KEY-----\nMIIB\nAbCd]\n"},
		{"END without BEGIN is an ordinary line", "-----END X-----\n", "[-----END X-----]\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			if err := Clean(&out, strings.NewReader(tt.in), mark, Options{}); err != nil {
				t.Fatalf("Clean() error = %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("Clean() = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestCleanIdentityRoundTrips(t *testing.T) {
	t.Parallel()

	in := "a\r\n-----BEGIN RSA PRIVATE KEY-----\r\nx\r\n-----END RSA PRIVATE KEY-----\r\n\nlast"
	var out bytes.Buffer
	if err := Clean(&out, strings.NewReader(in), func(s string) string { return s }, Options{}); err != nil {
		t.Fatalf("Clean() error = %v", err)
	}
	if out.String() != in {
		t.Errorf("Clean() = %q, want the input unchanged", out.String())
	}
}

func TestCleanBlockLimit(t *testing.T) {
	t.Parallel()

	in := "-----BEGIN PRIVATE KEY-----\n" + strings.Repeat("A", 40) + "\n" + "after\n"
	var out bytes.Buffer
	if err := Clean(&out, strings.NewReader(in), mark, Options{MaxBlock: 50}); err != nil {
		t.Fatalf("Clean() error = %v", err)
	}

	want := "[-----BEGIN PRIVATE KEY-----\n" + strings.Repeat("A", 40) + "]\n[after]\n"
	if out.String() != want {
		t.Errorf("Clean() = %q, want %q", out.String(), want)
	}
}

func TestCleanLineLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		maxLine int
		in      string
		wantErr string
	}{
		{"line at the limit, ending included", 20, strings.Repeat("a", 19) + "\n", ""},
		{"final line at the limit", 20, strings.Repeat("a", 20), ""},
		{"line over the limit", 20, "ok\n" + strings.Repeat("a", 20) + "\n", "line 2 is longer than 20 bytes"},
		{"limit below bufio's minimum", 4, strings.Repeat("a", 15) + "\n", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Clean(io.Discard, strings.NewReader(tt.in), mark, Options{MaxLine: tt.maxLine})
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Clean() error = %v, want nil", err)
			}
			if tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr) {
				t.Fatalf("Clean() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestCleanLineLimitDoesNotEchoTheLine(t *testing.T) {
	t.Parallel()

	in := "password=" + strings.Repeat("s", 64) + "\n"
	err := Clean(io.Discard, strings.NewReader(in), mark, Options{MaxLine: 32})
	if err == nil || strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "sss") {
		t.Errorf("Clean() error = %v, want a line number and limit only", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("disk on fire") }

type failingWriter struct{ after int }

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.after <= 0 {
		return 0, errors.New("pipe closed")
	}
	w.after--
	return len(p), nil
}

func TestCleanIOErrors(t *testing.T) {
	t.Parallel()

	if err := Clean(io.Discard, failingReader{}, mark, Options{}); err == nil || err.Error() != "read: disk on fire" {
		t.Errorf("read failure: error = %v, want %q", err, "read: disk on fire")
	}

	if err := Clean(&failingWriter{}, strings.NewReader("a\n"), mark, Options{}); err == nil || err.Error() != "write: pipe closed" {
		t.Errorf("write failure: error = %v, want %q", err, "write: pipe closed")
	}
}

func TestCleanStopsAtFirstWriteError(t *testing.T) {
	t.Parallel()

	// bufio.Writer holds 4096 bytes, so lines this long write through and the
	// error surfaces from the redacted body or the line ending.
	long := strings.Repeat("x", 5000)
	for _, in := range []string{long + "\n", "a\n" + long + "\n"} {
		err := Clean(&failingWriter{}, strings.NewReader(in), func(s string) string { return s }, Options{})
		if err == nil || err.Error() != "write: pipe closed" {
			t.Errorf("error = %v, want %q", err, "write: pipe closed")
		}
	}
}
