package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phoenix1331/cuddio/internal/config"
	"github.com/phoenix1331/cuddio/internal/rules"
	"github.com/phoenix1331/cuddio/internal/sqlscan"
	"github.com/phoenix1331/cuddio/internal/sqltest"
)

// session runs commands against one cache directory and clock, as a user's
// terminal session would.
type session struct {
	t     *testing.T
	cache string
	now   time.Time
	env   Env
}

func newSession(t *testing.T) *session {
	t.Helper()

	return &session{t: t, cache: filepath.Join(t.TempDir(), "cuddio"), now: time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC), env: Env{Rules: testRules}}
}

func (s *session) run(stdin string, args ...string) result {
	s.t.Helper()

	env := s.env
	env.Args = args
	env.Stdin = strings.NewReader(stdin)
	env.CacheDir = s.cache
	env.Now = func() time.Time { return s.now }

	return execute(s.t, env)
}

func TestMapRestoreRoundTripText(t *testing.T) {
	t.Parallel()

	s := newSession(t)
	cleaned := s.run(leakyInput, "--map")
	if cleaned.code != exitOK || cleaned.stderr != "" {
		t.Fatalf("clean --map = %d %q", cleaned.code, cleaned.stderr)
	}
	assertNoLeak(t, leakyValues, cleaned.stdout)

	restored := s.run(cleaned.stdout, "restore")
	if restored.code != exitOK || restored.stdout != leakyInput || restored.stderr != "" {
		t.Errorf("restore = %d %q %q, want the original back", restored.code, restored.stdout, restored.stderr)
	}
}

func TestMapRestoreRoundTripSQL(t *testing.T) {
	t.Parallel()

	for _, name := range sqltest.Fixtures() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := newSession(t)
			s.env.Rules = rules.All()
			original := sqltest.Fixture(t, name)
			cleaned := s.run(original, "--format", "sql", "--map")
			if cleaned.code != exitOK || cleaned.stdout == original {
				t.Fatalf("clean --map = %d %q", cleaned.code, cleaned.stderr)
			}

			restored := s.run(cleaned.stdout, "restore", "--format", "sql")
			if restored.code != exitOK {
				t.Fatalf("restore = %d %q", restored.code, restored.stderr)
			}
			d, _ := sqltest.DialectOf(name)
			if err := sqltest.Check(d, restored.stdout); err != nil {
				t.Errorf("restored dump is not valid %s: %v", d, err)
			}
			if got, want := sqlValues(t, d, restored.stdout), sqlValues(t, d, original); got != want {
				t.Errorf("restored values differ from the original:\n got %s\nwant %s", got, want)
			}
			// pg_dump --inserts output holds E'' and dollar-quoted strings,
			// which come back as standard strings with the same values
			if name != "postgres-inserts.sql" && restored.stdout != original {
				t.Errorf("restored dump differs from the original byte for byte:\n%s", restored.stdout)
			}
		})
	}
}

// sqlValues lists the decoded value of every token, so dumps that quote the
// same values differently compare equal.
func sqlValues(t *testing.T, d sqltest.Dialect, dump string) string {
	t.Helper()

	var b strings.Builder
	err := sqlscan.Clean(io.Discard, strings.NewReader(dump), sqlscan.Dialect(d), valueLister{&b}, sqlscan.NewColumns(nil, nil), sqlscan.Options{})
	if err != nil {
		t.Fatal(err)
	}

	return b.String()
}

// valueLister writes every value it is shown to a builder and changes
// nothing.
type valueLister struct{ b *strings.Builder }

func (v valueLister) Redact(s string) string {
	v.b.WriteString("[" + s + "]")
	return s
}

func (v valueLister) Whole(_, value string, _ int) string {
	v.b.WriteString("[" + value + "]")
	return value
}

func TestMapCarriesNumberingAcrossRuns(t *testing.T) {
	t.Parallel()

	s := newSession(t)
	first := s.run("alice@shop.test\n", "--map")
	second := s.run("bob@shop.test and alice@shop.test\n", "--map")
	if first.stdout != "<EMAIL_1>\n" || second.stdout != "<EMAIL_2> and <EMAIL_1>\n" {
		t.Fatalf("runs = %q, %q, want numbering carried on", first.stdout, second.stdout)
	}

	answer := "Write to <EMAIL_2>, copy <EMAIL_1>, ignore <EMAIL_9>.\n"
	if got := s.run(answer, "restore"); got.stdout != "Write to bob@shop.test, copy alice@shop.test, ignore <EMAIL_9>.\n" {
		t.Errorf("restore = %q %q", got.stdout, got.stderr)
	}
}

func TestMapReservesPlaceholdersInTheInput(t *testing.T) {
	t.Parallel()

	s := newSession(t)
	in := "the template says <EMAIL_1>, the user is alice@shop.test\n"
	cleaned := s.run(in, "--map")
	if cleaned.code != exitOK || cleaned.stdout != "the template says <EMAIL_1>, the user is <EMAIL_2>\n" {
		t.Fatalf("clean --map = %d %q %q", cleaned.code, cleaned.stdout, cleaned.stderr)
	}
	if got := s.run(cleaned.stdout, "restore"); got.stdout != in {
		t.Errorf("restore = %q, want %q", got.stdout, in)
	}
}

func TestMapRefusesAmbiguousInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		first string
		in    string
	}{
		{"placeholder after its value in one run", "", "alice@shop.test\nthen later <EMAIL_1>\n"},
		{"re-cleaning cleaned output", "alice@shop.test\n", "<EMAIL_1> and bob@shop.test\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newSession(t)
			if tt.first != "" {
				if got := s.run(tt.first, "--map"); got.code != exitOK {
					t.Fatal(got.stderr)
				}
			}
			got := s.run(tt.in, "--map")
			if got.code != exitError || !strings.Contains(got.stderr, "the mapping was not stored") {
				t.Errorf("clean --map = %d %q, want an ambiguity error", got.code, got.stderr)
			}
			if got.stdout == "" {
				t.Error("no output was written")
			}
			assertNoLeak(t, leakyValues, got.stdout, got.stderr)

			// The mapping from before the ambiguous run is left as it was
			restored := s.run("<EMAIL_1> <EMAIL_2>", "restore")
			if tt.first != "" {
				if want := "alice@shop.test <EMAIL_2>"; restored.code != exitOK || restored.stdout != want {
					t.Errorf("restore = %d %q %q, want %q", restored.code, restored.stdout, restored.stderr, want)
				}
				return
			}
			if restored.code != exitError {
				t.Errorf("restore after a refused first mapping = %d %q, want no mapping", restored.code, restored.stdout)
			}
		})
	}
}

func TestRestoreToOutputFile(t *testing.T) {
	t.Parallel()

	s := newSession(t)
	s.run("alice@shop.test\n", "--map")
	out := filepath.Join(t.TempDir(), "answer.md")

	got := s.run("Hi <EMAIL_1>\n", "restore", "-o", out)
	if got.code != exitOK || got.stdout != "" || readFile(t, out) != "Hi alice@shop.test\n" {
		t.Errorf("restore -o = %d %q %q, file %q", got.code, got.stdout, got.stderr, readFile(t, out))
	}
	if again := s.run("x", "restore", "-o", out); again.code != exitError {
		t.Error("restore -o over an existing file without --force succeeded")
	}
}

func TestRestoreFromFile(t *testing.T) {
	t.Parallel()

	s := newSession(t)
	s.run("alice@shop.test\n", "--map")
	in := filepath.Join(t.TempDir(), "answer.md")
	writeFile(t, in, "Hi <EMAIL_1>\r\n")

	if got := s.run("", "restore", in); got.code != exitOK || got.stdout != "Hi alice@shop.test\r\n" {
		t.Errorf("restore FILE = %d %q %q", got.code, got.stdout, got.stderr)
	}
}

func TestRestoreErrors(t *testing.T) {
	t.Parallel()

	t.Run("no mapping", func(t *testing.T) {
		t.Parallel()

		got := newSession(t).run("<EMAIL_1>", "restore")
		if got.code != exitError || got.stderr != "cuddio: no stored mapping to restore from: clean the input with --map first (mappings expire after 1h0m0s)\n" {
			t.Errorf("restore = %d %q", got.code, got.stderr)
		}
	})

	t.Run("expired mapping", func(t *testing.T) {
		t.Parallel()

		s := newSession(t)
		s.run("alice@shop.test\n", "--map")
		s.now = s.now.Add(time.Hour)
		if got := s.run("<EMAIL_1>", "restore"); got.code != exitError || strings.Contains(got.stdout, "alice") {
			t.Errorf("restore after the TTL = %d %q", got.code, got.stdout)
		}
	})

	t.Run("ttl from config", func(t *testing.T) {
		t.Parallel()

		s := newSession(t)
		dir := t.TempDir()
		writeConfig(t, filepath.Join(dir, config.ProjectFile), "mapping:\n  ttl: 5m\n")
		s.env.WorkDir = dir
		s.run("alice@shop.test\n", "--map")
		s.now = s.now.Add(4 * time.Minute)
		if got := s.run("<EMAIL_1>", "restore"); got.stdout != "alice@shop.test" {
			t.Errorf("restore within the TTL = %q %q", got.stdout, got.stderr)
		}
		s.now = s.now.Add(2 * time.Minute)
		if got := s.run("<EMAIL_1>", "restore"); got.code != exitError || !strings.Contains(got.stderr, "5m0s") {
			t.Errorf("restore after the TTL = %d %q", got.code, got.stderr)
		}
	})

	t.Run("corrupt mapping", func(t *testing.T) {
		t.Parallel()

		s := newSession(t)
		if err := os.MkdirAll(s.cache, 0o700); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(s.cache, "mapping.json"), "alice@shop.test")
		got := s.run("<EMAIL_1>", "restore")
		if got.code != exitError || !strings.Contains(got.stderr, "unreadable") {
			t.Errorf("restore = %d %q", got.code, got.stderr)
		}
		assertNoLeak(t, leakyValues, got.stdout, got.stderr)
	})

	t.Run("bad flags", func(t *testing.T) {
		t.Parallel()

		if got := newSession(t).run("", "restore", "--dialect", "oracle"); got.code != exitError {
			t.Errorf("restore --dialect oracle = %d", got.code)
		}
	})

	t.Run("terminal shows help", func(t *testing.T) {
		t.Parallel()

		got := execute(t, Env{Rules: testRules, Args: []string{"restore"}, Stdin: terminal{}, CacheDir: t.TempDir()})
		if got.code != exitOK || !strings.Contains(got.stdout, "restore [FILE]") {
			t.Errorf("restore on a terminal = %d %q", got.code, got.stdout)
		}
	})
}

func TestMapErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		config string
		want   string
	}{
		{"fake mode", []string{"--map", "--mode", "fake"}, "", "cuddio: --map needs placeholder mode: fake values cannot be restored\n"},
		{"storage disabled", []string{"--map"}, "mapping:\n  disabled: true\n", "cuddio: --map: mapping storage is disabled in config\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newSession(t)
			dir := t.TempDir()
			writeConfig(t, filepath.Join(dir, config.ProjectFile), tt.config)
			s.env.WorkDir = dir
			got := s.run(leakyInput, tt.args...)
			if got.code != exitError || got.stdout != "" || got.stderr != tt.want {
				t.Errorf("clean = %d %q %q, want %q", got.code, got.stdout, got.stderr, tt.want)
			}
			if _, err := os.Stat(s.cache); err == nil {
				t.Error("the mapping store was created")
			}
		})
	}
}

func TestMapCorruptMappingStartsAgain(t *testing.T) {
	t.Parallel()

	s := newSession(t)
	if err := os.MkdirAll(s.cache, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(s.cache, "mapping.json"), "{")

	got := s.run("alice@shop.test\n", "--map")
	if got.code != exitOK || got.stdout != "<EMAIL_1>\n" || !strings.Contains(got.stderr, "warning: the stored mapping was unreadable") {
		t.Errorf("clean --map = %d %q %q", got.code, got.stdout, got.stderr)
	}
	if restored := s.run("<EMAIL_1>", "restore"); restored.stdout != "alice@shop.test" {
		t.Errorf("restore = %q %q", restored.stdout, restored.stderr)
	}
}

func TestNoCacheDirectory(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"--map"}, {"restore"}, {"forget"}} {
		got := execute(t, Env{Rules: testRules, Args: args, Stdin: strings.NewReader("alice@shop.test")})
		if got.code != exitError || got.stderr != "cuddio: no user cache directory to keep mappings in\n" {
			t.Errorf("%v = %d %q", args, got.code, got.stderr)
		}
	}
}

func TestForget(t *testing.T) {
	t.Parallel()

	s := newSession(t)
	s.run("alice@shop.test\n", "--map")
	if got := s.run("", "forget"); got.code != exitOK || got.stdout != "Stored mappings deleted.\n" {
		t.Fatalf("forget = %d %q %q", got.code, got.stdout, got.stderr)
	}
	if got := s.run("<EMAIL_1>", "restore"); got.code != exitError {
		t.Errorf("restore after forget = %d %q", got.code, got.stdout)
	}
	if got := s.run("", "forget"); got.code != exitOK {
		t.Errorf("forget with nothing stored = %d %q", got.code, got.stderr)
	}
	if got := s.run("", "forget", "extra"); got.code != exitError {
		t.Errorf("forget with an argument = %d", got.code)
	}

	blocked := newSession(t)
	blocked.cache = filepath.Join(t.TempDir(), "file")
	writeFile(t, blocked.cache, "")
	if got := blocked.run("", "forget"); got.code != exitError {
		t.Errorf("forget with a file in place of the store = %d", got.code)
	}
}

func TestMapRunFailures(t *testing.T) {
	t.Parallel()

	s := newSession(t)
	missing := filepath.Join(t.TempDir(), "missing.log")
	if got := s.run("", missing, "--map"); got.code != exitError {
		t.Errorf("clean --map of a missing file = %d", got.code)
	}
	if _, err := os.Stat(s.cache); err == nil {
		t.Error("a failed run stored a mapping")
	}

	blocked := newSession(t)
	blocked.cache = filepath.Join(t.TempDir(), "file")
	writeFile(t, blocked.cache, "")
	if got := blocked.run("alice@shop.test\n", "--map"); got.code != exitError || got.stdout != "" {
		t.Errorf("clean --map with an unreadable store = %d %q", got.code, got.stdout)
	}
}
