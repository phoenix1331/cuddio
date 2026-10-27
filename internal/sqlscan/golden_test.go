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

// TestMySQLGolden cleans the MySQL fixture with the default rules and
// columns, compares the result with its .golden file, and checks the output
// is still a valid dump. Run with -update to accept new output, then review
// the diff.
func TestMySQLGolden(t *testing.T) {
	t.Parallel()

	in := sqltest.Fixture(t, "mysql.sql")
	got := cleanMySQL(t, newRedactor(t, newDetector(t)), goldenColumns(), in)
	golden := filepath.Join("testdata", "mysql.sql.golden")

	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/sqlscan -run TestMySQLGolden -update)", err)
	}
	if got != string(want) {
		t.Errorf("output differs from %s:\n%s", golden, got)
	}

	if err := sqltest.Check(sqltest.MySQL, got); err != nil {
		t.Errorf("cleaned dump is not valid MySQL: %v", err)
	}
	for _, s := range survivors() {
		if strings.Contains(got, s) {
			t.Errorf("%q survived cleaning", s)
		}
	}
}

func TestMySQLGeneratedDumpStaysValid(t *testing.T) {
	t.Parallel()

	var dump strings.Builder
	if err := sqltest.WriteDump(&dump, sqltest.MySQL, 300); err != nil {
		t.Fatal(err)
	}
	got := cleanMySQL(t, newRedactor(t, newDetector(t)), sqlscan.NewColumns(nil, nil), dump.String())

	if err := sqltest.Check(sqltest.MySQL, got); err != nil {
		t.Errorf("cleaned dump is not valid MySQL: %v", err)
	}
	for _, s := range []string{"@shop.test", "203.0.113.", "YTo"} {
		if strings.Contains(got, s) {
			t.Errorf("%q survived cleaning", s)
		}
	}
}

// FuzzCleanMySQL checks that cleaning never panics, gives the input back
// byte for byte when nothing is redacted, and turns a valid dump into a
// valid dump.
func FuzzCleanMySQL(f *testing.F) {
	f.Add(sqltest.Fixture(f, "mysql.sql"))
	f.Add(usersTable + "INSERT INTO `users` VALUES (1,'Alice','alice@shop.test','x\\'y'),(2,NULL,-1,0x0A);")
	f.Add("INSERT INTO t (phone) VALUES (_binary 'a'), (N\"b\"), (b'1');\n-- 203.0.113.9\n/* alice@shop.test */")
	det := newDetector(f)

	f.Fuzz(func(t *testing.T, in string) {
		cols := sqlscan.NewColumns(nil, nil)
		if got := cleanMySQL(t, identity{}, cols, in); got != in {
			t.Fatalf("identity clean changed the input:\n in %q\nout %q", in, got)
		}

		out := cleanMySQL(t, newRedactor(t, det), cols, in)
		if sqltest.Check(sqltest.MySQL, in) != nil {
			return
		}
		if err := sqltest.Check(sqltest.MySQL, out); err != nil {
			t.Fatalf("valid input became invalid: %v\n in %q\nout %q", err, in, out)
		}
	})
}

func BenchmarkCleanMySQL(b *testing.B) {
	var dump strings.Builder
	if err := sqltest.WriteDump(&dump, sqltest.MySQL, 5000); err != nil {
		b.Fatal(err)
	}
	det := newDetector(b)
	cols := sqlscan.NewColumns(nil, nil)

	b.SetBytes(int64(dump.Len()))
	b.ReportAllocs()
	for b.Loop() {
		if err := sqlscan.CleanMySQL(io.Discard, strings.NewReader(dump.String()), newRedactor(b, det), cols, sqlscan.Options{}); err != nil {
			b.Fatal(err)
		}
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

// TestCleanMySQLMemoryIsBounded streams about 10 MiB of generated dump
// through the cleaner and checks the live heap stays flat, as
// TestMySQLMemoryIsBounded does for the tokenizer alone.
func TestCleanMySQLMemoryIsBounded(t *testing.T) {
	const maxGrowth = 1 << 20

	var stats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&stats)
	baseline := stats.HeapAlloc

	w := &gcWriter{}
	if err := sqlscan.CleanMySQL(w, dumpReader(16000), constant{}, sqlscan.NewColumns(nil, nil), sqlscan.Options{}); err != nil {
		t.Fatal(err)
	}

	if w.written < 4*maxGrowth {
		t.Fatalf("wrote only %d KiB, too little to show a bound", w.written>>10)
	}
	growth := int64(w.peak) - int64(baseline)
	t.Logf("live heap grew by %d KiB writing %d KiB", growth>>10, w.written>>10)
	if growth > maxGrowth {
		t.Errorf("live heap grew by %d KiB writing %d KiB, want at most %d KiB", growth>>10, w.written>>10, maxGrowth>>10)
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
