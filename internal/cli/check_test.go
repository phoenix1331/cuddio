package cli

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/config"
	"github.com/phoenix1331/cuddio/internal/detect"
)

func TestCheckAndReport(t *testing.T) {
	t.Parallel()

	sql := "-- MySQL dump\nINSERT INTO users (name, email, notes) VALUES ('Alice', 'a@x.test', 'tok_s3cr3t'), ('Bob', 'b@x.test', NULL);\n"
	tests := []struct {
		name     string
		args     []string
		in       string
		wantCode int
		want     string
	}{
		{"check finds values", []string{"--check"}, leakyInput, exitFindings, "3 values would be hidden\n"},
		{"check finds one value", []string{"--check"}, "mail alice@shop.test\n", exitFindings, "1 value would be hidden\n"},
		{"check finds nothing", []string{"--check"}, "all clear\n", exitOK, "nothing to hide\n"},
		{"report", []string{"--report"}, leakyInput, exitOK, "RULE   COUNT\nemail  2\ntoken  1\ntotal  3\n"},
		{"report with nothing", []string{"--report"}, "all clear\n", exitOK, "RULE   COUNT\ntotal  0\n"},
		{"check and report", []string{"--check", "--report"}, leakyInput, exitFindings, "RULE   COUNT\nemail  2\ntoken  1\ntotal  3\n3 values would be hidden\n"},
		{"sql columns", []string{"--report", "--format", "sql"}, sql, exitOK, "RULE          COUNT\ncolumn:email  2\ncolumn:name   2\ntoken         1\ntotal         5\n"},
		{"check ignores fake mode", []string{"--check", "--mode", "fake"}, leakyInput, exitFindings, "3 values would be hidden\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, Env{Rules: testRules, Args: tt.args, Stdin: strings.NewReader(tt.in)})
			if got.code != tt.wantCode || got.stdout != tt.want || got.stderr != "" {
				t.Errorf("clean = %d %q %q, want %d %q", got.code, got.stdout, got.stderr, tt.wantCode, tt.want)
			}
			assertNoLeak(t, append(leakyValues, "a@x.test", "Alice"), got.stdout, got.stderr)
		})
	}
}

func TestCheckFileAndClipboard(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	in := filepath.Join(dir, "app.log")
	writeFile(t, in, leakyInput)
	before := stateOf(t, in)
	if got := execute(t, Env{Rules: testRules, Args: []string{in, "--check"}}); got.code != exitFindings || got.stdout != "3 values would be hidden\n" {
		t.Errorf("check FILE = %d %q %q", got.code, got.stdout, got.stderr)
	}
	if stateOf(t, in) != before {
		t.Error("input file changed")
	}

	cb := &fakeClipboard{text: leakyInput}
	got := execute(t, Env{Rules: testRules, Args: []string{"--clip", "--report"}, Clipboard: cb})
	if got.code != exitOK || !strings.Contains(got.stdout, "total  3") || cb.writes != 0 || cb.text != leakyInput {
		t.Errorf("report --clip = %d %q, %d writes", got.code, got.stdout, cb.writes)
	}
}

func TestCheckErrors(t *testing.T) {
	t.Parallel()

	out := filepath.Join(t.TempDir(), "out.log")
	for _, args := range [][]string{{"--check", "-o", out}, {"--report", "--map"}} {
		got := execute(t, Env{Rules: testRules, Args: args, Stdin: strings.NewReader(leakyInput), CacheDir: t.TempDir()})
		if got.code != exitError || got.stderr != "cuddio: --check and --report print counts only, so they take no --output or --map\n" {
			t.Errorf("%v = %d %q", args, got.code, got.stderr)
		}
	}

	// A broken input is an error (2), not a finding (1), even after values
	broken := &failingReader{data: leakyInput, err: errors.New("broken pipe")}
	got := execute(t, Env{Rules: testRules, Args: []string{"--check"}, Stdin: broken})
	if got.code != exitError || got.stdout != "" || got.stderr != "cuddio: read: broken pipe\n" {
		t.Errorf("check of a broken input = %d %q %q", got.code, got.stdout, got.stderr)
	}
}

// brokenWriter fails every write.
type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("stdout closed") }

func TestPrintCountsWriteErrors(t *testing.T) {
	t.Parallel()

	a := newApp(Env{Stdout: brokenWriter{}, Stderr: brokenWriter{}})
	for _, tc := range []struct {
		counts        map[string]int
		check, report bool
	}{
		{map[string]int{"email": 1}, false, true},
		{map[string]int{"email": 1}, true, false},
		{nil, true, false},
	} {
		if err := a.printCounts(tc.counts, tc.check, tc.report); err == nil || errors.Is(err, errFindings) {
			t.Errorf("printCounts(%v, %v, %v) = %v, want the write error", tc.counts, tc.check, tc.report, err)
		}
	}
}

func TestRulesCommand(t *testing.T) {
	t.Parallel()

	rules := append([]detect.Rule{
		{ID: "postcode", Category: detect.PII, Label: "POSTCODE", Pattern: regexp.MustCompile(`[A-Z]{2}[0-9]`), Optional: true},
		{ID: "npm-token", Category: detect.Secret, Label: "NPM_TOKEN", Pattern: regexp.MustCompile(`npm_x`), Source: detect.SourceGitleaks},
	}, testRules...)
	custom := "rules:\n  disable: [email]\n  custom:\n    - {id: order-ref, category: pii, label: ORDER, pattern: 'SO-[0-9]+'}\n"

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"all", []string{"rules"}, "" +
			"ID         CATEGORY  LABEL      DEFAULT  ENABLED  SOURCE\n" +
			"token      secret    TOKEN      on       on       built-in\n" +
			"npm-token  secret    NPM_TOKEN  on       on       gitleaks\n" +
			"email      pii       EMAIL      on       off      built-in\n" +
			"order-ref  pii       ORDER      on       on       custom\n" +
			"postcode   pii       POSTCODE   off      off      built-in\n"},
		{"pii only", []string{"rules", "--category", "pii"}, "" +
			"ID         CATEGORY  LABEL     DEFAULT  ENABLED  SOURCE\n" +
			"email      pii       EMAIL     on       off      built-in\n" +
			"order-ref  pii       ORDER     on       on       custom\n" +
			"postcode   pii       POSTCODE  off      off      built-in\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeConfig(t, filepath.Join(dir, config.ProjectFile), custom)
			got := execute(t, Env{Rules: rules, WorkDir: dir, Args: tt.args})
			if got.code != exitOK || got.stdout != tt.want || got.stderr != "" {
				t.Errorf("rules = %d (stderr %q)\n%s\nwant\n%s", got.code, got.stderr, got.stdout, tt.want)
			}
		})
	}
}

func TestRulesCommandErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		config string
		want   string
	}{
		{"unknown category", []string{"rules", "--category", "names"}, "", "cuddio: invalid --category: want secret or pii\n"},
		{"argument", []string{"rules", "email"}, "", `cuddio: unknown command "email" for "cuddio rules"` + "\n"},
		{"bad config toggle", []string{"rules"}, "rules:\n  disable: [emial]\n", `cuddio: rules: unknown rule or category "emial" in enable or disable; did you mean "email"?` + "\n"},
		{"bad custom rule", []string{"rules"}, "rules:\n  custom:\n    - {id: x, category: pii, label: X, pattern: '('}\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeConfig(t, filepath.Join(dir, config.ProjectFile), tt.config)
			got := execute(t, Env{Rules: testRules, WorkDir: dir, Args: tt.args})
			if got.code != exitError || got.stdout != "" || (tt.want != "" && got.stderr != tt.want) {
				t.Errorf("rules = %d %q %q, want %q", got.code, got.stdout, got.stderr, tt.want)
			}
		})
	}
}
