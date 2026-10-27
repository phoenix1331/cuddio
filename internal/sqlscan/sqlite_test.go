package sqlscan_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/sqlscan"
	"github.com/phoenix1331/cuddio/internal/sqltest"
)

func sqliteTokens(tb testing.TB, in string) []sqlscan.Token {
	tb.Helper()

	var out []sqlscan.Token
	tz := sqlscan.NewSQLite(strings.NewReader(in), sqlscan.Options{})
	for {
		tok, err := tz.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			tb.Fatalf("Next() error = %v", err)
		}
		out = append(out, tok)
	}
}

func TestSQLiteTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"insert", `INSERT INTO "users" VALUES(1,'a',X'0f',NULL);`, `Word:INSERT Word:INTO Identifier:"users" Word:VALUES Punct:( Number:1 Punct:, String:'a' Punct:, Hex:X'0f' Punct:, Null:NULL Punct:) Punct:;`},
		{"backslash is literal", `'a\' 'b'`, `String:'a\' String:'b'`},
		{"names", "[my table] `t` \"a\"\"b\"", "Identifier:[my table] Identifier:`t` Identifier:\"a\"\"b\""},
		{"comment without a space", "--x\n1", "Comment:--x Number:1"},
		{"comments do not nest", "/* a /* b */ c", "Comment:/* a /* b */ Word:c"},
		{"no directives", "/*!40101 x */", "Comment:/*!40101 x */"},
		{"hash", "#", "Punct:#"},
		{"line break wrapper", `replace('a\nb','\n',char(10))`, `Word:replace Punct:( String:'a\nb' Punct:, String:'\n' Punct:, Word:char Punct:( Number:10 Punct:) Punct:)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			toks := sqliteTokens(t, tt.in)
			if got := brief(toks); got != tt.want {
				t.Errorf("sqliteTokens(%q) =\n%s\nwant\n%s", tt.in, got, tt.want)
			}
			if got := join(toks); got != tt.in {
				t.Errorf("tokens do not join back to the input: %q", got)
			}
		})
	}
}

func TestSQLiteValue(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{`'it''s'`: "it's", `'a\n'`: `a\n`, "[a b]": "a b", "[open": "open", `"a""b"`: `a"b`} {
		toks := sqliteTokens(t, in)
		if len(toks) != 1 {
			t.Fatalf("sqliteTokens(%q) = %s, want one token", in, brief(toks))
		}
		if got := toks[0].Value(); got != want {
			t.Errorf("Value() of %s = %q, want %q", in, got, want)
		}
	}
}

func TestSQLiteBracketErrors(t *testing.T) {
	t.Parallel()

	tz := sqlscan.NewSQLite(strings.NewReader("["+strings.Repeat("a", 100)+"]"), sqlscan.Options{MaxToken: 64})
	if _, err := tz.Next(); err == nil || !strings.Contains(err.Error(), "is longer than") {
		t.Errorf("long name: error = %v, want a too-long error", err)
	}

	broken := errors.New("disk on fire")
	tz = sqlscan.NewSQLite(&brokenReader{data: "[ab", err: broken}, sqlscan.Options{})
	if _, err := tz.Next(); !errors.Is(err, broken) || !strings.HasPrefix(err.Error(), "read: ") {
		t.Errorf("read error inside a name = %v, want it wrapped with read:", err)
	}
}

func TestCleanSQLite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{
			name: "columns from create table",
			in:   "CREATE TABLE IF NOT EXISTS \"users\" (\"id\" integer, \"email\" varchar, \"notes\" text, primary key (\"id\"));\nINSERT INTO users VALUES(1,'a@x.test','it''s alice@shop.test');",
			want: "CREATE TABLE IF NOT EXISTS \"users\" (\"id\" integer, \"email\" varchar, \"notes\" text, primary key (\"id\"));\nINSERT INTO users VALUES(1,'<EMAIL>','it''s <EMAIL_1>');",
		},
		{
			name: "value wrapped for line breaks",
			in:   "INSERT INTO t (address, notes) VALUES(replace('1 High St\\nTown','\\n',char(10)),unistr('a\\u000ab'));",
			want: "INSERT INTO t (address, notes) VALUES(replace('<ADDRESS>','\\n',char(10)),unistr('a\\u000ab'));",
		},
		{
			name: "only the first wrapper argument is the value",
			in:   "INSERT INTO t (address) VALUES(REPLACE('x','y','z')), (unistr('w')), (lower('v'));",
			want: "INSERT INTO t (address) VALUES(REPLACE('<ADDRESS>','y','z')), (unistr('<ADDRESS>')), (lower('v'));",
		},
		{
			name: "bracketed names",
			in:   "INSERT INTO [t] ([phone]) VALUES ('1');",
			want: "INSERT INTO [t] ([phone]) VALUES ('<PHONE>');",
		},
		{
			name: "blobs in sensitive columns",
			in:   "INSERT INTO t (secret) VALUES (X'0f');",
			want: "INSERT INTO t (secret) VALUES ('<SECRET>');",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cleanDump(t, sqltest.SQLite, stubRedactor{}, sqlscan.NewColumns(nil, nil), tt.in)
			if got != tt.want {
				t.Errorf("CleanSQLite()\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, header string
		want         sqlscan.Dialect
	}{
		{"mysqldump", "-- MySQL dump 10.13  Distrib 8.4.3\n", sqlscan.MySQL},
		{"mariadb-dump", "-- MariaDB dump 10.19\n/*M!999999\\- enable the sandbox mode */\n", sqlscan.MySQL},
		{"directive only", "/*!40101 SET NAMES utf8mb4 */;\n", sqlscan.MySQL},
		{"backticks only", "INSERT INTO `t` VALUES (1);", sqlscan.MySQL},
		{"pg_dump", "--\n-- PostgreSQL database dump\n--\n", sqlscan.Postgres},
		{"pg_dump restrict", "\\restrict abc\n", sqlscan.Postgres},
		{"copy only", "COPY t (a) FROM stdin;\n1\n\\.\n", sqlscan.Postgres},
		{"pg settings", "SET standard_conforming_strings = on;\nSELECT pg_catalog.set_config('search_path', '', false);", sqlscan.Postgres},
		{"sqlite3 .dump", "PRAGMA foreign_keys=OFF;\nBEGIN TRANSACTION;\n", sqlscan.SQLite},
		{"sqlite sequence", "DELETE FROM sqlite_sequence;", sqlscan.SQLite},
		{"first marker wins", "-- PostgreSQL database dump\nINSERT INTO t VALUES ('`-- MySQL dump`');", sqlscan.Postgres},
		{"none", "INSERT INTO t VALUES (1);", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		got, ok := sqlscan.Detect([]byte(tt.header))
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("%s: Detect() = %q, %v, want %q", tt.name, got, ok, tt.want)
		}
	}

	for _, name := range sqltest.Fixtures() {
		want, _ := sqltest.DialectOf(name)
		if got, _ := sqlscan.Detect([]byte(sqltest.Fixture(t, name))); string(got) != string(want) {
			t.Errorf("Detect(%s) = %q, want %q", name, got, want)
		}
	}
}

func TestParseDialect(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]sqlscan.Dialect{
		"mysql": sqlscan.MySQL, "MariaDB": sqlscan.MySQL, "postgres": sqlscan.Postgres, "PostgreSQL": sqlscan.Postgres,
		"pg": sqlscan.Postgres, "sqlite": sqlscan.SQLite, "SQLite3": sqlscan.SQLite, "oracle": "", "": "",
	} {
		got, ok := sqlscan.ParseDialect(name)
		if got != want || ok != (want != "") {
			t.Errorf("ParseDialect(%q) = %q, %v, want %q", name, got, ok, want)
		}
	}
}

func TestClean(t *testing.T) {
	t.Parallel()

	cols := sqlscan.NewColumns(nil, nil)
	for _, d := range []sqlscan.Dialect{sqlscan.MySQL, sqlscan.Postgres, sqlscan.SQLite} {
		var out bytes.Buffer
		if err := sqlscan.Clean(&out, strings.NewReader("SELECT 'alice@shop.test';"), d, stubRedactor{}, cols, sqlscan.Options{}); err != nil || out.String() != "SELECT '<EMAIL_1>';" {
			t.Errorf("Clean(%s) = %q, %v", d, out.String(), err)
		}
	}
	if err := sqlscan.Clean(io.Discard, strings.NewReader(""), "oracle", stubRedactor{}, cols, sqlscan.Options{}); err == nil {
		t.Error("Clean() with an unknown dialect succeeded")
	}
}
