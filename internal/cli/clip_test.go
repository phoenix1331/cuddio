package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClipboard is an in-memory clipboard that records writes.
type fakeClipboard struct {
	text              string
	writes            int
	private           bool
	readErr, writeErr error
}

func (c *fakeClipboard) Read() (string, error) {
	return c.text, c.readErr
}

func (c *fakeClipboard) Write(text string, private bool) error {
	if c.writeErr != nil {
		return c.writeErr
	}
	c.text, c.private = text, private
	c.writes++

	return nil
}

func TestCleanClip(t *testing.T) {
	t.Parallel()

	cb := &fakeClipboard{text: leakyInput}
	got := execute(t, Env{Rules: testRules, Args: []string{"--clip"}, Clipboard: cb})

	if got.code != exitOK || got.stdout != "" || got.stderr != "cuddio: clipboard cleaned\n" {
		t.Errorf("clean --clip = %d %q %q", got.code, got.stdout, got.stderr)
	}
	if want := "login <EMAIL_1> with <TOKEN_1>\r\nmail <EMAIL_2>\n"; cb.text != want || cb.private || cb.writes != 1 {
		t.Errorf("clipboard = %q (private %v, %d writes), want %q", cb.text, cb.private, cb.writes, want)
	}
	assertNoLeak(t, leakyValues, got.stdout, got.stderr, cb.text)
}

func TestCleanClipUnchanged(t *testing.T) {
	t.Parallel()

	cb := &fakeClipboard{text: "nothing secret here\n"}
	got := execute(t, Env{Rules: testRules, Args: []string{"--clip"}, Clipboard: cb})

	if got.code != exitOK || !strings.Contains(got.stderr, "nothing to hide") || cb.writes != 0 {
		t.Errorf("clean --clip = %d %q, %d writes", got.code, got.stderr, cb.writes)
	}
}

func TestCleanClipToOutput(t *testing.T) {
	t.Parallel()

	cb := &fakeClipboard{text: leakyInput}
	out := filepath.Join(t.TempDir(), "clean.txt")
	got := execute(t, Env{Rules: testRules, Args: []string{"--clip", "-o", out}, Clipboard: cb})

	if got.code != exitOK || got.stderr != "" || cb.writes != 0 || cb.text != leakyInput {
		t.Errorf("clean --clip -o = %d %q, %d writes", got.code, got.stderr, cb.writes)
	}
	if want := "login <EMAIL_1> with <TOKEN_1>\r\nmail <EMAIL_2>\n"; readFile(t, out) != want {
		t.Errorf("output file = %q", readFile(t, out))
	}
}

func TestCleanClipSQL(t *testing.T) {
	t.Parallel()

	cb := &fakeClipboard{text: "INSERT INTO users (email, notes) VALUES ('a@x.test', 'O''Brien');"}
	got := execute(t, Env{Rules: testRules, Args: []string{"--clip", "--dialect", "postgres"}, Clipboard: cb})

	if want := "INSERT INTO users (email, notes) VALUES ('<EMAIL_1>', 'O''Brien');"; got.code != exitOK || cb.text != want {
		t.Errorf("clean --clip --dialect = %d %q, clipboard %q", got.code, got.stderr, cb.text)
	}
}

func TestClipMapRestore(t *testing.T) {
	t.Parallel()

	s := newSession(t)
	cb := &fakeClipboard{text: leakyInput}
	s.env.Clipboard = cb
	if got := s.run("", "--clip", "--map"); got.code != exitOK {
		t.Fatalf("clean --clip --map = %d %q", got.code, got.stderr)
	}

	cb.text = "Reply to <EMAIL_2> about <TOKEN_1>, not <EMAIL_9>.\n"
	got := s.run("", "restore", "--clip")
	if got.code != exitOK || got.stdout != "" || got.stderr != "cuddio: clipboard restored\n" {
		t.Errorf("restore --clip = %d %q %q", got.code, got.stdout, got.stderr)
	}
	if want := "Reply to bob@shop.test about tok_s3cr3t, not <EMAIL_9>.\n"; cb.text != want || !cb.private {
		t.Errorf("clipboard = %q (private %v), want %q written privately", cb.text, cb.private, want)
	}

	cb.text = "no placeholders\n"
	if got := s.run("", "restore", "--clip"); got.code != exitOK || !strings.Contains(got.stderr, "no placeholders from the stored mapping") {
		t.Errorf("restore --clip with nothing to restore = %d %q", got.code, got.stderr)
	}
}

func TestClipOnTerminalDoesNotShowHelp(t *testing.T) {
	t.Parallel()

	cb := &fakeClipboard{text: "alice@shop.test"}
	if got := execute(t, Env{Rules: testRules, Args: []string{"--clip"}, Stdin: terminal{}, Clipboard: cb}); got.code != exitOK || cb.text != "<EMAIL_1>" {
		t.Errorf("clean --clip from a terminal = %d %q, clipboard %q", got.code, got.stdout, cb.text)
	}

	s := newSession(t)
	s.run("alice@shop.test", "--map")
	cb.text = "<EMAIL_1>"
	env := s.env
	env.Args, env.Stdin, env.CacheDir, env.Clipboard = []string{"restore", "--clip"}, terminal{}, s.cache, cb
	env.Now = func() time.Time { return s.now }
	if got := execute(t, env); got.code != exitOK || cb.text != "alice@shop.test" {
		t.Errorf("restore --clip from a terminal = %d %q, clipboard %q", got.code, got.stderr, cb.text)
	}
}

func TestClipErrors(t *testing.T) {
	t.Parallel()

	broken := errors.New("clipboard: xclip: exit status 1")
	tests := []struct {
		name string
		args []string
		cb   *fakeClipboard
		want string
	}{
		{"no clipboard", []string{"--clip"}, nil, "cuddio: no clipboard is available\n"},
		{"empty clipboard", []string{"--clip"}, &fakeClipboard{}, "cuddio: the clipboard holds no text\n"},
		{"read fails", []string{"--clip"}, &fakeClipboard{readErr: broken}, "cuddio: clipboard: xclip: exit status 1\n"},
		{"write fails", []string{"--clip"}, &fakeClipboard{text: leakyInput, writeErr: broken}, "cuddio: clipboard: xclip: exit status 1\n"},
		{"clip with a file", []string{"--clip", "app.log"}, &fakeClipboard{text: "x"}, "cuddio: --clip reads the clipboard, so it takes no FILE\n"},
		{"undetectable sql", []string{"--clip", "--format", "sql"}, &fakeClipboard{text: "alice@shop.test"}, "cuddio: cannot tell the SQL dialect from the start of the input; use --dialect mysql, postgres or sqlite\n"},
		{"sql too long", []string{"--clip", "--dialect", "mysql"}, &fakeClipboard{text: "SELECT '" + strings.Repeat("x", 17<<20) + "';"}, "cuddio: token at line 1 is longer than 16777216 bytes\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := Env{Rules: testRules, Args: tt.args}
			if tt.cb != nil {
				env.Clipboard = tt.cb
			}
			got := execute(t, env)
			if got.code != exitError || got.stdout != "" || got.stderr != tt.want {
				t.Errorf("clean = %d %q %q, want %q", got.code, got.stdout, got.stderr, tt.want)
			}
			if tt.cb != nil && tt.cb.writes != 0 {
				t.Error("the clipboard was written after an error")
			}
			assertNoLeak(t, leakyValues, got.stdout, got.stderr)
		})
	}
}
