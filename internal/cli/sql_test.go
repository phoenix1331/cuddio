package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/config"
)

func TestCleanSQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		config string
		in     string
		want   string
	}{
		{
			name: "mysql detected",
			in:   "-- MySQL dump 10.13\nINSERT INTO `t` (email, notes) VALUES ('a@x.test','see bob@shop.test');\n",
			want: "-- MySQL dump 10.13\nINSERT INTO `t` (email, notes) VALUES ('<EMAIL_1>','see <EMAIL_2>');\n",
		},
		{
			name: "postgres detected",
			in:   "-- PostgreSQL database dump\nCOPY public.t (email, notes) FROM stdin;\nalice@shop.test\tvia bob@shop.test\n\\.\n",
			want: "-- PostgreSQL database dump\nCOPY public.t (email, notes) FROM stdin;\n<EMAIL_1>\tvia <EMAIL_2>\n\\.\n",
		},
		{
			name: "sqlite detected",
			in:   "PRAGMA foreign_keys=OFF;\nBEGIN TRANSACTION;\nCREATE TABLE \"t\" (\"phone\" varchar);\nINSERT INTO t VALUES(replace('07700\\n900123','\\n',char(10)));\nCOMMIT;\n",
			want: "PRAGMA foreign_keys=OFF;\nBEGIN TRANSACTION;\nCREATE TABLE \"t\" (\"phone\" varchar);\nINSERT INTO t VALUES(replace('<PHONE_1>','\\n',char(10)));\nCOMMIT;\n",
		},
		{
			name: "dialect flag decides how backslashes read",
			args: []string{"--dialect", "postgres"},
			in:   "INSERT INTO t (notes) VALUES ('a\\', 'alice@shop.test');\n",
			want: "INSERT INTO t (notes) VALUES ('a\\', '<EMAIL_1>');\n",
		},
		{
			name: "dialect alias in any case",
			args: []string{"--dialect", "PostgreSQL", "--format", "sql"},
			in:   "SELECT $$alice@shop.test$$;",
			want: "SELECT '<EMAIL_1>';",
		},
		{
			name:   "sensitive columns from config",
			args:   []string{"--dialect", "mysql"},
			config: "sql:\n  columns: [notes]\n  tables:\n    t: [code]\n",
			in:     "INSERT INTO t (notes, code) VALUES ('hello', 'x1');\nINSERT INTO u (code) VALUES ('x1');\n",
			want:   "INSERT INTO t (notes, code) VALUES ('<NOTES_1>', '<CODE_1>');\nINSERT INTO u (code) VALUES ('x1');\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeConfig(t, filepath.Join(dir, config.ProjectFile), tt.config)
			args := tt.args
			if len(args) == 0 {
				args = []string{"--format", "sql"}
			}

			got := execute(t, Env{Rules: testRules, WorkDir: dir, Args: args, Stdin: strings.NewReader(tt.in)})

			if got.code != exitOK || got.stdout != tt.want {
				t.Errorf("clean = %d %q (stderr %q), want %d %q", got.code, got.stdout, got.stderr, exitOK, tt.want)
			}
		})
	}
}

func TestCleanSQLFileToOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	in := filepath.Join(dir, "dump.sql")
	out := filepath.Join(dir, "clean.sql")
	writeFile(t, in, "PRAGMA foreign_keys=OFF;\nINSERT INTO users (email) VALUES('alice@shop.test');\n")

	got := execute(t, Env{Rules: testRules, Args: []string{in, "-o", out}})

	want := "PRAGMA foreign_keys=OFF;\nINSERT INTO users (email) VALUES('<EMAIL_1>');\n"
	if got.code != exitOK || got.stdout != "" || readFile(t, out) != want {
		t.Errorf("clean = %d %q %q, output %q, want %q", got.code, got.stdout, got.stderr, readFile(t, out), want)
	}
}

func TestCleanSQLErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown dialect", []string{"--dialect", "oracle"}, "cuddio: invalid --dialect: want mysql, postgres or sqlite\n"},
		{"dialect with text format", []string{"--dialect", "mysql", "--format", "text"}, "cuddio: --dialect only applies to --format sql\n"},
		{"undetectable dialect", []string{"--format", "sql"}, "cuddio: cannot tell the SQL dialect from the start of the input; use --dialect mysql, postgres or sqlite\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, Env{Rules: testRules, Args: tt.args, Stdin: strings.NewReader("SELECT 'alice@shop.test';\n")})

			if got.code != exitError || got.stdout != "" || got.stderr != tt.want {
				t.Errorf("clean = %d %q %q, want %d %q", got.code, got.stdout, got.stderr, exitError, tt.want)
			}
		})
	}
}
