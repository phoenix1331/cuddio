package sqlscan_test

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/detect"
	"github.com/phoenix1331/cuddio/internal/replace"
	"github.com/phoenix1331/cuddio/internal/rules"
	"github.com/phoenix1331/cuddio/internal/sqlscan"
	"github.com/phoenix1331/cuddio/internal/sqltest"
)

var update = flag.Bool("update", false, "rewrite golden files with the current output")

// newDetector returns the built-in rules that are on by default, with the
// built-in allowlist. Compiling the rules is the slow part, so tests build
// one detector and reuse it.
func newDetector(tb testing.TB) *detect.Detector {
	tb.Helper()

	reg, err := detect.NewRegistry(rules.All())
	if err != nil {
		tb.Fatal(err)
	}
	selected, err := reg.Select(nil, nil)
	if err != nil {
		tb.Fatal(err)
	}
	allow, err := detect.NewAllowlist(nil, nil)
	if err != nil {
		tb.Fatal(err)
	}

	return detect.NewDetector(selected, allow)
}

// newRedactor returns cuddio's default pipeline for one run.
func newRedactor(tb testing.TB, det *detect.Detector) *detect.Redactor {
	tb.Helper()

	p, err := replace.NewPlaceholders(replace.DefaultFormat)
	if err != nil {
		tb.Fatal(err)
	}

	return detect.NewRedactor(det, p)
}

// goldenColumns adds the one Laravel column the defaults miss in the
// fixture, the way a project's .cuddio.yaml would.
func goldenColumns() *sqlscan.Columns {
	return sqlscan.NewColumns(nil, map[string][]string{"orders": {"billing_name"}})
}

// survivors are fixture values that must not appear in cleaned output.
func survivors() []string {
	return []string{
		"alice@", "bob@", "carys@", "Brien", "Bobby", "Llŷr", "07700", "Heol y Frenhines", "Mill Lane", "CF10", "SW1A",
		"$2y$", "4242 4242", "198.51.100.23", "203.0.113.9", "2001:db8::7", "sk_live_", "YTo",
		sqltest.Fake(60, 8), sqltest.Fake(60, 9), sqltest.Hex(64, 1), sqltest.Hex(64, 2), sqltest.Fake(40, 4), sqltest.Fake(40, 6),
	}
}

// cleaners are the dialects CleanX functions, by sqltest dialect.
var cleaners = map[sqltest.Dialect]func(io.Writer, io.Reader, sqlscan.Redactor, *sqlscan.Columns, sqlscan.Options) error{
	sqltest.MySQL:    sqlscan.CleanMySQL,
	sqltest.Postgres: sqlscan.CleanPostgres,
}

func cleanDump(tb testing.TB, d sqltest.Dialect, rd sqlscan.Redactor, cols *sqlscan.Columns, in string) string {
	tb.Helper()

	var out bytes.Buffer
	if err := cleaners[d](&out, strings.NewReader(in), rd, cols, sqlscan.Options{}); err != nil {
		tb.Fatalf("clean %s: %v", d, err)
	}

	return out.String()
}

// TestGolden cleans each MySQL and PostgreSQL fixture with the default
// rules and columns, compares the result with its .golden file, and checks
// the output is still a valid dump. Run with -update to accept new output,
// then review the diff.
func TestGolden(t *testing.T) {
	t.Parallel()

	det := newDetector(t)
	for _, name := range sqltest.Fixtures() {
		d, _ := sqltest.DialectOf(name)
		if cleaners[d] == nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := cleanDump(t, d, newRedactor(t, det), goldenColumns(), sqltest.Fixture(t, name))
			golden := filepath.Join("testdata", name+".golden")

			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/sqlscan -run TestGolden -update)", err)
			}
			if got != string(want) {
				t.Errorf("output differs from %s:\n%s", golden, got)
			}

			if err := sqltest.Check(d, got); err != nil {
				t.Errorf("cleaned dump is not valid %s: %v", d, err)
			}
			for _, s := range survivors() {
				if strings.Contains(got, s) {
					t.Errorf("%q survived cleaning", s)
				}
			}
		})
	}
}

func TestGeneratedDumpsStayValid(t *testing.T) {
	t.Parallel()

	det := newDetector(t)
	for d := range cleaners {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()

			var dump strings.Builder
			if err := sqltest.WriteDump(&dump, d, 300); err != nil {
				t.Fatal(err)
			}
			got := cleanDump(t, d, newRedactor(t, det), sqlscan.NewColumns(nil, nil), dump.String())

			if err := sqltest.Check(d, got); err != nil {
				t.Errorf("cleaned dump is not valid %s: %v", d, err)
			}
			for _, s := range []string{"@shop.test", "203.0.113.", "YTo"} {
				if strings.Contains(got, s) {
					t.Errorf("%q survived cleaning", s)
				}
			}
		})
	}
}

// fuzzClean checks that cleaning never panics, gives the input back byte
// for byte when nothing is redacted, and turns a valid dump into a valid
// dump.
func fuzzClean(f *testing.F, d sqltest.Dialect) {
	det := newDetector(f)

	f.Fuzz(func(t *testing.T, in string) {
		cols := sqlscan.NewColumns(nil, nil)
		if got := cleanDump(t, d, identity{}, cols, in); got != in {
			t.Fatalf("identity clean changed the input:\n in %q\nout %q", in, got)
		}

		out := cleanDump(t, d, newRedactor(t, det), cols, in)
		if sqltest.Check(d, in) != nil {
			return
		}
		if err := sqltest.Check(d, out); err != nil {
			t.Fatalf("valid input became invalid: %v\n in %q\nout %q", err, in, out)
		}
	})
}

func FuzzCleanMySQL(f *testing.F) {
	f.Add(sqltest.Fixture(f, "mysql.sql"))
	f.Add(usersTable + "INSERT INTO `users` VALUES (1,'Alice','alice@shop.test','x\\'y'),(2,NULL,-1,0x0A);")
	f.Add("INSERT INTO t (phone) VALUES (_binary 'a'), (N\"b\"), (b'1');\n-- 203.0.113.9\n/* alice@shop.test */")
	fuzzClean(f, sqltest.MySQL)
}

func FuzzCleanPostgres(f *testing.F) {
	f.Add(sqltest.Fixture(f, "postgres.sql"))
	f.Add(sqltest.Fixture(f, "postgres-inserts.sql"))
	f.Add(pgUsers + "INSERT INTO public.users VALUES (1, E'O\\'Brien', $$alice@shop.test$$, 'x');\nCOPY public.users (email, notes) FROM stdin;\nalice@shop.test\t203.0.113.9\\t\\N\n\\.\n")
	fuzzClean(f, sqltest.Postgres)
}

func BenchmarkClean(b *testing.B) {
	det := newDetector(b)
	cols := sqlscan.NewColumns(nil, nil)
	for _, d := range []sqltest.Dialect{sqltest.MySQL, sqltest.Postgres} {
		b.Run(string(d), func(b *testing.B) {
			var dump strings.Builder
			if err := sqltest.WriteDump(&dump, d, 5000); err != nil {
				b.Fatal(err)
			}

			b.SetBytes(int64(dump.Len()))
			b.ReportAllocs()
			for b.Loop() {
				if err := cleaners[d](io.Discard, strings.NewReader(dump.String()), newRedactor(b, det), cols, sqlscan.Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// constant replaces every value with the same token, so the redactor holds
// no state and the memory test measures the cleaner alone. Placeholder
// state grows with distinct values by design.
type constant struct{}

func (constant) Redact(s string) string   { return strings.ReplaceAll(s, "@", "<EMAIL_1>") }
func (constant) Whole(_, _ string) string { return "<X_1>" }

// gcWriter discards output, forcing a collection every 512 KiB written and
// recording the peak live heap.
type gcWriter struct {
	written, next, peak uint64
}

func (w *gcWriter) Write(p []byte) (int, error) {
	w.written += uint64(len(p))
	if w.written >= w.next {
		w.next += 512 << 10
		var stats runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&stats)
		w.peak = max(w.peak, stats.HeapAlloc)
	}

	return len(p), nil
}

// TestCleanMemoryIsBounded streams about 5 MiB of generated dump per
// dialect through the cleaner and checks the live heap stays flat, as
// TestMySQLMemoryIsBounded does for the tokenizer alone. Not parallel, so
// other tests do not share the heap being measured.
func TestCleanMemoryIsBounded(t *testing.T) {
	const maxGrowth = 1 << 20

	for _, d := range []sqltest.Dialect{sqltest.MySQL, sqltest.Postgres} {
		var stats runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&stats)
		baseline := stats.HeapAlloc

		w := &gcWriter{}
		if err := cleaners[d](w, dumpReader(d, 16000), constant{}, sqlscan.NewColumns(nil, nil), sqlscan.Options{}); err != nil {
			t.Fatal(err)
		}

		if w.written < 4*maxGrowth {
			t.Fatalf("%s: wrote only %d KiB, too little to show a bound", d, w.written>>10)
		}
		growth := int64(w.peak) - int64(baseline)
		t.Logf("%s: live heap grew by %d KiB writing %d KiB", d, growth>>10, w.written>>10)
		if growth > maxGrowth {
			t.Errorf("%s: live heap grew by %d KiB writing %d KiB, want at most %d KiB", d, growth>>10, w.written>>10, maxGrowth>>10)
		}
	}
}

func TestCleanMySQLMatchesTextRules(t *testing.T) {
	t.Parallel()

	// A value redacted by a pattern in one row and by its column in another
	// gets one token, whichever comes first.
	in := "INSERT INTO t (notes, email) VALUES ('from alice@shop.test', 'alice@shop.test');"
	want := "INSERT INTO t (notes, email) VALUES ('from <EMAIL_1>', '<EMAIL_1>');"
	if got := cleanMySQL(t, newRedactor(t, newDetector(t)), sqlscan.NewColumns(nil, nil), in); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	var out bytes.Buffer
	again := sqlscan.CleanMySQL(&out, strings.NewReader(want), newRedactor(t, newDetector(t)), sqlscan.NewColumns(nil, nil), sqlscan.Options{})
	if again != nil || out.String() != want {
		t.Errorf("cleaning cleaned output changed it: %q, %v", out.String(), again)
	}
}
