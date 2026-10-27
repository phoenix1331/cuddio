package sqlscan_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/sqlscan"
)

// stubRedactor redacts "alice@shop.test" (and "LINE", "OPEN" and "CLOSE"
// as tokens holding a line break, "/*" or "*/", for the comment cases), and
// replaces whole values with <LABEL>.
// "keep" stands in for an allowlisted value.
type stubRedactor struct{}

func (stubRedactor) Redact(s string) string {
	s = strings.ReplaceAll(s, "alice@shop.test", "<EMAIL_1>")
	s = strings.ReplaceAll(s, "LINE", "<A\nB>")
	s = strings.ReplaceAll(s, "OPEN", "</*>")
	s = strings.ReplaceAll(s, "BANG", "!<B>")
	s = strings.ReplaceAll(s, "--", "<DASHES>")
	return strings.ReplaceAll(s, "CLOSE", "<*/>")
}

func (stubRedactor) Whole(label, value string) string {
	if value == "keep" {
		return value
	}

	return "<" + label + ">"
}

func cleanMySQL(tb testing.TB, rd sqlscan.Redactor, cols *sqlscan.Columns, in string) string {
	tb.Helper()

	var out bytes.Buffer
	if err := sqlscan.CleanMySQL(&out, strings.NewReader(in), rd, cols, sqlscan.Options{}); err != nil {
		tb.Fatalf("CleanMySQL(%q) error = %v", in, err)
	}

	return out.String()
}

const usersTable = "CREATE TABLE `users` (\n  `id` int NOT NULL,\n  `name` varchar(255) DEFAULT 'x,y',\n  `email` varchar(255),\n  `notes` text,\n  PRIMARY KEY (`id`),\n  UNIQUE KEY `users_email_unique` (`email`)\n);\n"

func TestCleanMySQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{
			name: "columns from create table",
			in:   usersTable + "INSERT INTO `users` VALUES (1,'Alice','a@x.test','hi'),(2,NULL,'b@x.test','alice@shop.test');",
			want: usersTable + "INSERT INTO `users` VALUES (1,'<NAME>','<EMAIL>','hi'),(2,NULL,'<EMAIL>','<EMAIL_1>');",
		},
		{
			name: "explicit column list beats create table",
			in:   usersTable + "INSERT INTO users (notes, email) VALUES ('Alice', 'x');",
			want: usersTable + "INSERT INTO users (notes, email) VALUES ('Alice', '<EMAIL>');",
		},
		{
			name: "unknown table gets pattern rules only",
			in:   "INSERT INTO t VALUES ('Alice','alice@shop.test');",
			want: "INSERT INTO t VALUES ('Alice','<EMAIL_1>');",
		},
		{
			name: "unknown table with a column list",
			in:   "INSERT IGNORE INTO t (`id`,`phone`) VALUES (1,447700900123),(2,-5),(3,0x0A),(4,b'1'),(5,_binary 'p');",
			want: "INSERT IGNORE INTO t (`id`,`phone`) VALUES (1,'<PHONE>'),(2,-'<PHONE>'),(3,'<PHONE>'),(4,'<PHONE>'),(5,_binary '<PHONE>');",
		},
		{
			name: "qualified names",
			in:   "CREATE TABLE IF NOT EXISTS `shop`.`t` (`a` int, `password` text);\nINSERT INTO `shop`.`t` VALUES (1,'p');",
			want: "CREATE TABLE IF NOT EXISTS `shop`.`t` (`a` int, `password` text);\nINSERT INTO `shop`.`t` VALUES (1,'<PASSWORD>');",
		},
		{
			name: "temporary table and replace into",
			in:   "CREATE TEMPORARY TABLE t (email text);\nREPLACE INTO t VALUES ('x');",
			want: "CREATE TEMPORARY TABLE t (email text);\nREPLACE INTO t VALUES ('<EMAIL>');",
		},
		{
			name: "nested values are not whole values",
			in:   "INSERT INTO t (email, n) VALUES (CONCAT('a', 'b'), 'x');",
			want: "INSERT INTO t (email, n) VALUES (CONCAT('a', 'b'), 'x');",
		},
		{
			name: "values past the last column",
			in:   "INSERT INTO t (email) VALUES ('a', 'b');",
			want: "INSERT INTO t (email) VALUES ('<EMAIL>', 'b');",
		},
		{
			name: "row constructor",
			in:   "INSERT INTO t (id, email) VALUES ROW(1, 'a'), ROW(2, 'b');",
			want: "INSERT INTO t (id, email) VALUES ROW(1, '<EMAIL>'), ROW(2, '<EMAIL>');",
		},
		{
			name: "on duplicate key update",
			in:   "INSERT INTO t (email) VALUES ('a') ON DUPLICATE KEY UPDATE email = 'b';",
			want: "INSERT INTO t (email) VALUES ('<EMAIL>') ON DUPLICATE KEY UPDATE email = 'b';",
		},
		{
			name: "insert set and select",
			in:   "INSERT INTO t SET email = 'a';\nINSERT INTO t SELECT 'a';\nINSERT;",
			want: "INSERT INTO t SET email = 'a';\nINSERT INTO t SELECT 'a';\nINSERT;",
		},
		{
			name: "each statement starts afresh",
			in:   "INSERT INTO t (email) VALUES ('a');\nSELECT 'a', 'alice@shop.test';\nINSERT INTO u VALUES ('a');",
			want: "INSERT INTO t (email) VALUES ('<EMAIL>');\nSELECT 'a', '<EMAIL_1>';\nINSERT INTO u VALUES ('a');",
		},
		{
			name: "create table forms without columns",
			in:   "CREATE TABLE t LIKE u;\nCREATE INDEX i ON t (email);\nCREATE VIEW v AS SELECT 1;\nINSERT INTO t VALUES ('a');",
			want: "CREATE TABLE t LIKE u;\nCREATE INDEX i ON t (email);\nCREATE VIEW v AS SELECT 1;\nINSERT INTO t VALUES ('a');",
		},
		{
			name: "statements that are not understood",
			in:   "CREATE TABLE 't' (email text);\nINSERT INTO 't' VALUES ('a');\nINSERT INTO t (email) VALUES ('b');",
			want: "CREATE TABLE 't' (email text);\nINSERT INTO 't' VALUES ('a');\nINSERT INTO t (email) VALUES ('<EMAIL>');",
		},
		{
			name: "a later create table replaces columns",
			in:   "CREATE TABLE t (email text);\nCREATE TABLE t (id int, email text);\nINSERT INTO t VALUES (1, 'a');",
			want: "CREATE TABLE t (email text);\nCREATE TABLE t (id int, email text);\nINSERT INTO t VALUES (1, '<EMAIL>');",
		},
		{
			name: "values the redactor keeps are copied as they were",
			in:   "INSERT INTO t (email, phone) VALUES (\"keep\", 0x6B656570);",
			want: "INSERT INTO t (email, phone) VALUES (\"keep\", '<PHONE>');",
		},
		{
			name: "quote character and prefix kept",
			in:   "INSERT INTO t (email) VALUES (\"it's\"), (N'x'), (_utf8mb4\"y\");",
			want: "INSERT INTO t (email) VALUES (\"<EMAIL>\"), (N'<EMAIL>'), (_utf8mb4\"<EMAIL>\");",
		},
		{
			name: "pattern matches are escaped on the way back",
			in:   `SELECT 'O\'Brien \"x\" \\ alice@shop.test\n\0\Z\%', "a""alice@shop.test";`,
			want: `SELECT 'O\'Brien \"x\" \\ <EMAIL_1>\n\0\Z\\%', "a\"<EMAIL_1>";`,
		},
		{
			name: "inside executable comments",
			in:   "/*!40000 INSERT INTO t (email) VALUES ('a') */;",
			want: "/*!40000 INSERT INTO t (email) VALUES ('<EMAIL>') */;",
		},
		{
			name: "comments",
			in:   "-- alice@shop.test\n# alice@shop.test\n/* alice@shop.test */ /* alice@shop.test",
			want: "-- <EMAIL_1>\n# <EMAIL_1>\n/* <EMAIL_1> */ /* <EMAIL_1>",
		},
		{
			name: "comments a placeholder would break",
			in:   "/*BANG*/ -- LINE\n/* CLOSE */ /* CLOSE\n",
			want: "/**/ /**/\n/**/ /**/",
		},
		{
			name: "comment markers are never redacted",
			in:   "-- --\n#--\n/*--*/ --",
			want: "-- <DASHES>\n#<DASHES>\n/*<DASHES>*/ --",
		},
		{
			name: "unterminated string",
			in:   "SELECT 'alice@shop.test",
			want: "SELECT '<EMAIL_1>'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cleanMySQL(t, stubRedactor{}, sqlscan.NewColumns(nil, nil), tt.in)
			if got != tt.want {
				t.Errorf("CleanMySQL()\n got %s\nwant %s", got, tt.want)
			}
		})
	}
}

// identity changes nothing, so cleaning must give the input back exactly.
type identity struct{}

func (identity) Redact(s string) string       { return s }
func (identity) Whole(_, value string) string { return value }

func TestCleanMySQLUnchangedRoundTrips(t *testing.T) {
	t.Parallel()

	in := usersTable + "INSERT INTO `users` VALUES (1,'O\\'Brien',\"a\"\"b\",N'\\%'),(2,NULL,0x0A,b'1');\r\n-- note\r\n/*!40101 SET x='y' */;"
	if got := cleanMySQL(t, identity{}, sqlscan.NewColumns(nil, nil), in); got != in {
		t.Errorf("CleanMySQL() changed input it did not redact:\n got %q\nwant %q", got, in)
	}
}

type failingWriter struct{ after int }

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.after <= 0 {
		return 0, errors.New("disk full")
	}
	w.after--

	return len(p), nil
}

func TestCleanMySQLErrors(t *testing.T) {
	t.Parallel()

	cols := sqlscan.NewColumns(nil, nil)
	long := "SELECT '" + strings.Repeat("a", 100) + "';"
	err := sqlscan.CleanMySQL(&bytes.Buffer{}, strings.NewReader(long), stubRedactor{}, cols, sqlscan.Options{MaxToken: 64})
	if err == nil || !strings.Contains(err.Error(), "line 1") || strings.Contains(err.Error(), "aaaa") {
		t.Errorf("long token: error = %v, want one naming the line and not the value", err)
	}

	// Large enough to fill bufio's 4 KiB buffer, so a write happens mid-stream
	big := strings.Repeat("SELECT 1;\n", 1000)
	if err := sqlscan.CleanMySQL(&failingWriter{}, strings.NewReader(big), stubRedactor{}, cols, sqlscan.Options{}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("failing writer mid-stream: error = %v, want disk full", err)
	}
	if err := sqlscan.CleanMySQL(&failingWriter{}, strings.NewReader("SELECT 1;"), stubRedactor{}, cols, sqlscan.Options{}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("failing writer at flush: error = %v, want disk full", err)
	}
}
