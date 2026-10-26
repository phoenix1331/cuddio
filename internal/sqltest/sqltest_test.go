package sqltest

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/detect"
	"github.com/phoenix1331/cuddio/internal/rules"
)

func TestFixturesAreValid(t *testing.T) {
	t.Parallel()

	names := Fixtures()
	if len(names) != 4 {
		t.Fatalf("Fixtures() = %v, want the four dumps", names)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d, ok := DialectOf(name)
			if !ok {
				t.Fatalf("no dialect for %s", name)
			}
			dump := Fixture(t, name)
			if strings.Contains(dump, "{{") {
				t.Error("fixture has a template tag left after expanding")
			}
			if err := Check(d, dump); err != nil {
				t.Errorf("Check(%s) error = %v", d, err)
			}
		})
	}
}

// Fixture files are committed, so before expansion they must hold nothing a
// secret scanner (GitHub push protection, gitleaks) would flag. PII is fine:
// it is all fake and uses reserved ranges.
func TestFixtureTemplatesHoldNoSecrets(t *testing.T) {
	t.Parallel()

	reg, err := detect.NewRegistry(rules.All())
	if err != nil {
		t.Fatal(err)
	}
	selected, err := reg.Select(nil, []string{"pii", "high-entropy-assignment"})
	if err != nil {
		t.Fatal(err)
	}
	det := detect.NewDetector(selected, nil)

	for _, name := range Fixtures() {
		data, err := fixtureFS.ReadFile("testdata/sql/" + name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range det.Find(string(data)) {
			// A template tag in the match is filled in only at test time
			if !strings.Contains(string(data[m.Start:m.End]), "{{") {
				t.Errorf("%s: rule %s matches committed text at byte %d", name, m.Label, m.Start)
			}
		}
	}
}

func TestDialectOf(t *testing.T) {
	t.Parallel()

	if d, ok := DialectOf("postgres-inserts.sql"); !ok || d != Postgres {
		t.Errorf("DialectOf(postgres-inserts.sql) = %q, %v", d, ok)
	}
	if _, ok := DialectOf("oracle.sql"); ok {
		t.Error("DialectOf(oracle.sql) found a dialect")
	}
}

func TestExpand(t *testing.T) {
	t.Parallel()

	got := Expand("a {{fake 4}} b {{fake 4 1}} c {{hex 6 2}} d")
	if want := "a Ab3C b f7Gh c abcdef d"; got != want {
		t.Errorf("Expand() = %q, want %q", got, want)
	}
	if Fake(60, 8) == Fake(60, 9) || Hex(64, 1) == Hex(64, 2) {
		t.Error("different seeds give the same value")
	}
}

func TestCheckRejectsBrokenDumps(t *testing.T) {
	t.Parallel()

	const mysqlTable = "CREATE TABLE `t` (`id` int, `email` varchar(9));\n"
	const pgTable = "CREATE TABLE public.t (id int, email text);\n"

	tests := []struct {
		name    string
		dialect Dialect
		dump    string
		want    string
	}{
		{"mysql unquoted placeholder", MySQL, mysqlTable + "INSERT INTO `t` VALUES (1,<EMAIL_1>);\n", "mysql: "},
		{"mysql broken quote", MySQL, mysqlTable + "INSERT INTO `t` VALUES (1,'it's');\n", "mysql: "},
		{"mysql escaped closing quote", MySQL, mysqlTable + "INSERT INTO `t` VALUES (1,'a\\');\n", "mysql: "},
		{"mysql row too short", MySQL, mysqlTable + "INSERT INTO `t` VALUES (1,'a'),(2);\n", "mysql: table t row 2: 1 values, want 2"},
		{"mysql column list", MySQL, mysqlTable + "INSERT INTO `t` (`id`) VALUES (1,'a');\n", "mysql: table t row 1: 2 values, want 1"},
		{"postgres unquoted placeholder", Postgres, pgTable + "INSERT INTO public.t VALUES (1, <EMAIL_1>);\n", "postgres: "},
		{"postgres row too long", Postgres, pgTable + "INSERT INTO public.t VALUES (1, 'a', 'b');\n", "postgres: table t row 1: 3 values, want 2"},
		{"postgres copy field missing", Postgres, pgTable + "COPY public.t (id, email) FROM stdin;\n1\n\\.\n", "postgres: table t COPY row 1: 1 fields, want 2"},
		{"postgres copy without column list", Postgres, pgTable + "COPY public.t FROM stdin;\n1\ta\tb\n\\.\n", "postgres: table t COPY row 1: 3 fields, want 2"},
		{"postgres copy escaped tab", Postgres, pgTable + "COPY public.t (id, email) FROM stdin;\n1\\\ta\n\\.\n", "postgres: table t COPY row 1: 1 fields, want 2"},
		{"postgres copy lone backslash", Postgres, pgTable + "COPY public.t (id, email) FROM stdin;\n1\ta\\\n\\.\n", "postgres: table t COPY row 1: ends in a lone backslash"},
		{"postgres copy unterminated", Postgres, pgTable + "COPY public.t (id, email) FROM stdin;\n1\ta\n", "postgres: COPY at line 2 has no \\. terminator"},
		{"sqlite broken quote", SQLite, "CREATE TABLE t (id int, email text);\nINSERT INTO t VALUES(1,'it's');\n", "sqlite: "},
		{"sqlite row too long", SQLite, "CREATE TABLE t (id int, email text);\nINSERT INTO t VALUES(1,'a','b');\n", "sqlite: "},
		{"unknown dialect", "oracle", "", `unknown dialect "oracle"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Check(tt.dialect, tt.dump)
			if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Errorf("Check() error = %v, want it to start with %q", err, tt.want)
			}
		})
	}
}

func TestCheckAcceptsClientCommands(t *testing.T) {
	t.Parallel()

	mysql := "CREATE TABLE `t` (`id` int);\nDELIMITER ;;\n/*!50003 CREATE*/ /*!50003 TRIGGER `t_bi` BEFORE INSERT ON `t` FOR EACH ROW SET NEW.id = NEW.id + 1 */;;\nDELIMITER ;\nINSERT INTO `t` VALUES (1);\nINSERT INTO `t` VALUES (1,2);\n"
	if err := Check(MySQL, mysql); err == nil || !strings.Contains(err.Error(), "row 1: 2 values, want 1") {
		t.Errorf("Check(mysql with DELIMITER) error = %v, want the bad row after the block found", err)
	}
	mysql = strings.TrimSuffix(mysql, "INSERT INTO `t` VALUES (1,2);\n")
	if err := Check(MySQL, mysql); err != nil {
		t.Errorf("Check(mysql with DELIMITER) error = %v", err)
	}

	postgres := "\\restrict abc\r\nCREATE TABLE public.t (id int, note text);\r\nCOPY public.t (id, note) FROM stdin;\r\n1\t\\N\r\n\\.\r\n\\unrestrict abc\r\n"
	if err := Check(Postgres, postgres); err != nil {
		t.Errorf("Check(postgres with meta-commands and CRLF) error = %v", err)
	}

	// A COPY TO, or one reading a file, has no inline data block
	if err := Check(Postgres, "COPY public.t TO stdout;\n"); err != nil {
		t.Errorf("Check(COPY TO) error = %v", err)
	}
}

func TestCopyWithoutDataBlock(t *testing.T) {
	t.Parallel()

	// A COPY header that splitPostgres does not recognise (lowercase) still
	// parses as COPY FROM stdin, so its data is missing
	err := Check(Postgres, "copy public.t (id) from stdin;\n")
	if err == nil || err.Error() != "postgres: COPY statement without a data block" {
		t.Errorf("Check() error = %v, want a missing data block error", err)
	}
}

func TestSQLiteJoinsOnEmail(t *testing.T) {
	t.Parallel()

	db, err := OpenSQLite(Fixture(t, "sqlite.sql"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM orders o JOIN users u ON u.email = o.billing_email`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("orders joined to users on email = %d, want 3", n)
	}
}

func TestWriteDump(t *testing.T) {
	t.Parallel()

	for _, d := range []Dialect{MySQL, Postgres, SQLite} {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()

			var small, again, large bytes.Buffer
			if err := WriteDump(&small, d, 150); err != nil {
				t.Fatal(err)
			}
			if err := WriteDump(&again, d, 150); err != nil {
				t.Fatal(err)
			}
			if err := WriteDump(&large, d, 1500); err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(small.Bytes(), again.Bytes()) {
				t.Error("WriteDump is not deterministic")
			}
			if err := Check(d, small.String()); err != nil {
				t.Errorf("Check(generated) error = %v", err)
			}
			if !strings.Contains(small.String(), "user150@shop.test") {
				t.Error("generated dump lacks the last user")
			}
			if ratio := float64(large.Len()) / float64(small.Len()); ratio < 9 || ratio > 11 {
				t.Errorf("10x the users gives %.1fx the bytes, want about 10x", ratio)
			}
		})
	}
}

func TestWriteDumpErrors(t *testing.T) {
	t.Parallel()

	if err := WriteDump(&bytes.Buffer{}, "oracle", 1); err == nil {
		t.Error("WriteDump(oracle) error = nil")
	}
	if err := WriteDump(failingWriter{}, MySQL, 100); err == nil {
		t.Error("WriteDump to a failing writer error = nil")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }
