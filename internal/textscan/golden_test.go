package textscan_test

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/detect"
	"github.com/phoenix1331/cuddio/internal/replace"
	"github.com/phoenix1331/cuddio/internal/rules"
	"github.com/phoenix1331/cuddio/internal/textscan"
)

var update = flag.Bool("update", false, "rewrite golden files with the current output")

// fake returns n deterministic characters that look random, as in the rules
// tests. Every fake value starts with fakePrefix, so a test can prove none
// survived cleaning.
func fake(n int) string {
	const chars = "Ab3Cd5Ef7Gh9Jk2Lm4Np6Qr8St0Uv1Wx"

	var b strings.Builder
	for i := range n {
		b.WriteByte(chars[i%len(chars)])
	}

	return b.String()
}

const fakePrefix = "Ab3Cd5Ef"

var templateTag = regexp.MustCompile(`\{\{(?:fake ([0-9]+)|private-key)\}\}`)

// expand fills in a fixture template. Vendor-format tokens and key blocks
// are written as {{fake N}} and {{private-key}}, so nothing committed looks
// like a live secret to GitHub push protection or a user's scanner.
func expand(template string) string {
	return templateTag.ReplaceAllStringFunc(template, func(tag string) string {
		m := templateTag.FindStringSubmatch(tag)
		if m[1] == "" {
			return "-----BEGIN RSA PRIVATE KEY-----\n" + fake(64) + "\n" + fake(64) + "\n" + fake(20) + "==\n-----END RSA PRIVATE KEY-----"
		}
		n, _ := strconv.Atoi(m[1])
		return fake(n)
	})
}

// newDetector returns the built-in rules that are on by default, with the
// built-in allowlist. Compiling the rules is the slow part, so tests build
// one detector and reuse it.
func newDetector(tb testing.TB) *detect.Detector {
	tb.Helper()

	reg, err := detect.NewRegistry(rules.Builtin())
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

// newRedactor returns cuddio's default pipeline for one run: det with fresh
// default placeholders.
func newRedactor(tb testing.TB, det *detect.Detector) func(string) string {
	tb.Helper()

	p, err := replace.NewPlaceholders(replace.DefaultFormat)
	if err != nil {
		tb.Fatal(err)
	}

	return func(s string) string { return p.Replace(s, det.Find(s)) }
}

func clean(tb testing.TB, det *detect.Detector, in string) string {
	tb.Helper()

	var out bytes.Buffer
	if err := textscan.Clean(&out, strings.NewReader(in), newRedactor(tb, det), textscan.Options{}); err != nil {
		tb.Fatalf("Clean() error = %v", err)
	}

	return out.String()
}

func fixtures(tb testing.TB) []string {
	tb.Helper()

	paths, err := filepath.Glob(filepath.Join("testdata", "text", "*"))
	if err != nil {
		tb.Fatal(err)
	}
	var inputs []string
	for _, p := range paths {
		if filepath.Ext(p) != ".golden" {
			inputs = append(inputs, p)
		}
	}
	if len(inputs) == 0 {
		tb.Fatal("no fixtures in testdata/text")
	}

	return inputs
}

func readFixture(tb testing.TB, path string) string {
	tb.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}

	return expand(string(data))
}

// TestGolden cleans each fixture with LF and CRLF line endings and compares
// the result with its .golden file. Run with -update to accept new output,
// then review the diff.
func TestGolden(t *testing.T) {
	t.Parallel()

	det := newDetector(t)
	for _, path := range fixtures(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			in := readFixture(t, path)
			got := clean(t, det, in)
			golden := path + ".golden"

			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/textscan -run TestGolden -update)", err)
			}

			if got != string(want) {
				t.Errorf("LF output differs from %s:\n%s", golden, got)
			}
			crlf := clean(t, det, strings.ReplaceAll(in, "\n", "\r\n"))
			if wantCRLF := strings.ReplaceAll(string(want), "\n", "\r\n"); crlf != wantCRLF {
				t.Errorf("CRLF output differs from %s with CRLF endings:\n%q", golden, crlf)
			}
			if strings.Contains(got, fakePrefix) || strings.Contains(got, "{{") {
				t.Errorf("a fake secret survived cleaning:\n%s", got)
			}
		})
	}
}

// FuzzClean checks that the scanner never panics, passes text through
// byte for byte when nothing is redacted, and leaves nothing a rule still
// matches. A match in the cleaned output is allowed only where it touches a
// placeholder: replacing a value changes the context of its neighbours (a
// "::2" after "0" is not an address; after "<IP_1>" it is), which says
// nothing about what survived.
func FuzzClean(f *testing.F) {
	for _, path := range fixtures(f) {
		f.Add(readFixture(f, path))
	}
	f.Add("a\r\nb\rc\n\n-----BEGIN X-----\nno end")
	f.Add("DB_PASSWORD=hunter2\nmysql://app:hunter2@db.shop.test/x alice@shop.test")
	f.Add("203.0.113.9 203.0.113.10")
	det := newDetector(f)

	f.Fuzz(func(t *testing.T, in string) {
		var same bytes.Buffer
		if err := textscan.Clean(&same, strings.NewReader(in), func(s string) string { return s }, textscan.Options{}); err != nil {
			t.Skip("line longer than the limit")
		}
		if same.String() != in {
			t.Fatalf("identity clean changed the input:\n in %q\nout %q", in, same.String())
		}

		out := clean(t, det, in)
		for _, line := range strings.Split(out, "\n") {
			for _, m := range det.Find(line) {
				if touchesPlaceholder(line, m.Start, m.End) {
					continue
				}
				t.Errorf("%q survived cleaning:\n in %q\nout %q", line[m.Start:m.End], in, out)
			}
		}
	})
}

func touchesPlaceholder(line string, start, end int) bool {
	return strings.ContainsAny(line[start:end], "<>") || strings.HasSuffix(line[:start], ">") || strings.HasPrefix(line[end:], "<")
}

// logLines are the lines generated logs cycle through: a fixed set of
// values, so placeholder state stays bounded however long the log is.
func logLines(tb testing.TB) []string {
	tb.Helper()

	var lines []string
	for _, path := range fixtures(tb) {
		lines = append(lines, strings.SplitAfter(readFixture(tb, path), "\n")...)
	}
	for i := range 50 {
		lines = append(lines, fmt.Sprintf("[2026-10-26 10:%02d:00] production.INFO: user%d@shop.test from 203.0.113.%d viewed /orders/%d\n", i, i, i, 1000+i))
	}

	return lines
}

// logReader generates size bytes of log without holding them in memory.
type logReader struct {
	lines   []string
	next    int
	pending string
	left    int
}

func (r *logReader) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, io.EOF
	}
	if r.pending == "" {
		r.pending = r.lines[r.next%len(r.lines)]
		r.next++
	}
	n := copy(p[:min(len(p), r.left)], r.pending)
	r.pending = r.pending[n:]
	r.left -= n

	return n, nil
}

func BenchmarkClean(b *testing.B) {
	const size = 4 << 20
	lines := logLines(b)
	redact := newRedactor(b, newDetector(b))

	b.SetBytes(size)
	b.ReportAllocs()
	for b.Loop() {
		if err := textscan.Clean(io.Discard, &logReader{lines: lines, left: size}, redact, textscan.Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

// TestCleanMemoryIsBounded streams three times more log than the heap may
// grow by, after a warm-up run has allocated the line buffer and regexp
// caches. HeapSys only grows, so after the run it reflects the peak. Kept
// small because the race detector slows matching about tenfold.
func TestCleanMemoryIsBounded(t *testing.T) {
	const (
		warmUp    = 256 << 10
		size      = 3 << 20
		maxGrowth = 1 << 20
	)

	lines := logLines(t)
	redact := newRedactor(t, newDetector(t))
	run := func(n int) {
		if err := textscan.Clean(io.Discard, &logReader{lines: lines, left: n}, redact, textscan.Options{}); err != nil {
			t.Fatal(err)
		}
	}
	run(warmUp)

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	run(size)
	runtime.ReadMemStats(&after)

	if growth := int64(after.HeapSys) - int64(before.HeapSys); growth > maxGrowth {
		t.Errorf("heap grew by %d KiB cleaning %d MiB, want at most %d KiB", growth>>10, size>>20, maxGrowth>>10)
	}
}
