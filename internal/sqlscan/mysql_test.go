package sqlscan_test

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/sqlscan"
	"github.com/phoenix1331/cuddio/internal/sqltest"
)

// tokens reads every token from in, failing the test on an error.
func tokens(tb testing.TB, in string) []sqlscan.Token {
	tb.Helper()

	var out []sqlscan.Token
	tz := sqlscan.NewMySQL(strings.NewReader(in), sqlscan.Options{})
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

// brief writes tokens as "Kind:Text" pairs, leaving out whitespace, so
// table tests stay readable.
func brief(toks []sqlscan.Token) string {
	var parts []string
	for _, t := range toks {
		if t.Kind != sqlscan.Whitespace {
			parts = append(parts, t.Kind.String()+":"+t.Text)
		}
	}

	return strings.Join(parts, " ")
}

func join(toks []sqlscan.Token) string {
	var b strings.Builder
	for _, t := range toks {
		b.WriteString(t.Text)
	}

	return b.String()
}

func TestMySQLTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"insert", "INSERT INTO `users` VALUES (1,'a',NULL);", "Word:INSERT Word:INTO Identifier:`users` Word:VALUES Punct:( Number:1 Punct:, String:'a' Punct:, Null:NULL Punct:) Punct:;"},
		{"null in any case", "null Null", "Null:null Null:Null"},
		{"word containing null", "nullable NULLIF", "Word:nullable Word:NULLIF"},
		{"doubled quote", "'it''s'", "String:'it''s'"},
		{"backslash quote", `'it\'s'`, `String:'it\'s'`},
		{"backslash before the closing quote", `'a\\' 'b'`, `String:'a\\' String:'b'`},
		{"double quoted", `"say \"hi\""`, `String:"say \"hi\""`},
		{"single quote inside double", `"it's"`, `String:"it's"`},
		{"string over lines", "'line one\nline two'", "String:'line one\nline two'"},
		{"doubled backtick", "`a``b`", "Identifier:`a``b`"},
		{"backslash in identifier is literal", "`a\\` x", "Identifier:`a\\` Word:x"},
		{"hex", "0x89504E47 X'00ff' x'AB'", "Hex:0x89504E47 Hex:X'00ff' Hex:x'AB'"},
		{"bit", "0b0101 b'1' B'0'", "Bit:0b0101 Bit:b'1' Bit:B'0'"},
		{"0x without digits is a word", "0xZZ 0x", "Word:0xZZ Word:0x"},
		{"national string", "N'Llŷr'", "String:N'Llŷr'"},
		{"word starting with x", "xid x_y", "Word:xid Word:x_y"},
		{"numbers", "42 -7 3.14 .5 1e10 2.5E-3 1e", "Number:42 Punct:- Number:7 Number:3.14 Number:.5 Number:1e10 Number:2.5E-3 Word:1e"},
		{"number then dot", "1.2.3", "Number:1.2 Number:.3"},
		{"name starting with digits", "1abc 123_x", "Word:1abc Word:123_x"},
		{"line comments", "-- note\n# hash\n--x", "Comment:-- note Comment:# hash Punct:- Punct:- Word:x"},
		{"dash dash at the end", "--", "Comment:--"},
		{"block comment", "/* a 'quoted' */ 1", "Comment:/* a 'quoted' */ Number:1"},
		{"optimizer hint", "/*+ BKA(t) */", "Comment:/*+ BKA(t) */"},
		{"directive", "/*!40101 SET NAMES utf8mb4 */;", "DirectiveStart:/*!40101 Word:SET Word:NAMES Word:utf8mb4 DirectiveEnd:*/ Punct:;"},
		{"directive holding a string", "/*!80000 '+'*/ '3E11'", "DirectiveStart:/*!80000 String:'+' DirectiveEnd:*/ String:'3E11'"},
		{"directive without version", "/*! STRAIGHT_JOIN */", "DirectiveStart:/*! Word:STRAIGHT_JOIN DirectiveEnd:*/"},
		{"star outside a directive", "2*/3", "Number:2 Punct:* Punct:/ Number:3"},
		{"comment inside a directive", "/*!50001 /* x */ VIEW */", "DirectiveStart:/*!50001 Comment:/* x */ Word:VIEW DirectiveEnd:*/"},
		{"user variable", "@saved_cs_client = @@character_set_client", "Punct:@ Word:saved_cs_client Punct:= Punct:@ Punct:@ Word:character_set_client"},
		{"unicode name", "Llŷr", "Word:Llŷr"},
		{"punctuation", "(),;=", "Punct:( Punct:) Punct:, Punct:; Punct:="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			toks := tokens(t, tt.in)
			if got := brief(toks); got != tt.want {
				t.Errorf("tokens(%q) =\n%s\nwant\n%s", tt.in, got, tt.want)
			}
			if got := join(toks); got != tt.in {
				t.Errorf("tokens do not join back to the input: %q", got)
			}
		})
	}
}

func TestMySQLIntroducers(t *testing.T) {
	t.Parallel()

	toks := tokens(t, "_binary 0x89, _utf8mb4 /* c */ 'x', _binary b'1', N'y', _latin1 a, 'z'")

	var got []string
	for _, tok := range toks {
		switch tok.Kind {
		case sqlscan.String, sqlscan.Hex, sqlscan.Bit:
			got = append(got, tok.Text+"="+tok.Charset)
		}
	}
	if want := "0x89=binary 'x'=utf8mb4 b'1'=binary N'y'=N 'z'="; strings.Join(got, " ") != want {
		t.Errorf("literal charsets = %q, want %q", strings.Join(got, " "), want)
	}
}

func TestMySQLValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{`'plain'`, "plain"},
		{`'it''s'`, "it's"},
		{`'it\'s'`, "it's"},
		{`"say \"hi\" or ""bye"""`, `say "hi" or "bye"`},
		{`'a\nb\r\tc\0d\Ze\bf'`, "a\nb\r\tc\x00d\x1ae\bf"},
		{`'back\\slash'`, `back\slash`},
		{`'like\%this\_'`, `like\%this\_`},
		{`'unknown\q'`, "unknownq"},
		{`N'Llŷr'`, "Llŷr"},
		{"`a``b`", "a`b"},
		{"`a\\nb`", `a\nb`},
		{`''`, ""},
	}

	for _, tt := range tests {
		toks := tokens(t, tt.in)
		if len(toks) != 1 {
			t.Fatalf("tokens(%q) = %d tokens, want 1", tt.in, len(toks))
		}
		if got := toks[0].Value(); got != tt.want {
			t.Errorf("Value() of %s = %q, want %q", tt.in, got, tt.want)
		}
	}

	if got := tokens(t, "42")[0].Value(); got != "42" {
		t.Errorf("Value() of a number = %q, want its text", got)
	}
}

func TestMySQLUnterminated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in    string
		kind  sqlscan.Kind
		value string
	}{
		{"'open", sqlscan.String, "open"},
		{`'ends in \`, sqlscan.String, `ends in \`},
		{"'doubled ''", sqlscan.String, "doubled '"},
		{"`open", sqlscan.Identifier, "open"},
		{"/* open", sqlscan.Comment, "/* open"},
		{"X'00", sqlscan.Hex, "X'00"},
		{"b'", sqlscan.Bit, "b'"},
	}

	for _, tt := range tests {
		toks := tokens(t, tt.in)
		if len(toks) != 1 || toks[0].Kind != tt.kind || !toks[0].Unterminated || toks[0].Text != tt.in {
			t.Errorf("tokens(%q) = %+v, want one unterminated %v", tt.in, toks, tt.kind)
			continue
		}
		if got := toks[0].Value(); got != tt.value {
			t.Errorf("Value() of %q = %q, want %q", tt.in, got, tt.value)
		}
	}
}

func TestMySQLLines(t *testing.T) {
	t.Parallel()

	toks := tokens(t, "a\n'x\ny'\r\n-- c\nb")
	var got []string
	for _, tok := range toks {
		if tok.Kind != sqlscan.Whitespace {
			got = append(got, fmt.Sprintf("%s@%d", strings.SplitN(tok.Text, "\n", 2)[0], tok.Line))
		}
	}
	if want := "a@1 'x@2 -- c@4 b@5"; strings.Join(got, " ") != want {
		t.Errorf("token lines = %q, want %q", strings.Join(got, " "), want)
	}
}

func TestMySQLMaxToken(t *testing.T) {
	t.Parallel()

	tz := sqlscan.NewMySQL(strings.NewReader("1,\n'"+strings.Repeat("s", 64)+"'"), sqlscan.Options{MaxToken: 32})
	var err error
	for err == nil {
		_, err = tz.Next()
	}
	if err.Error() != "token at line 2 is longer than 32 bytes" {
		t.Errorf("Next() error = %v, want a too-long error for line 2", err)
	}

	// Every kind of token is capped, not only strings
	for _, in := range []string{strings.Repeat(" ", 64), strings.Repeat("a", 64), strings.Repeat("9", 64), "/*" + strings.Repeat("c", 64), "0x" + strings.Repeat("f", 64), "X'" + strings.Repeat("f", 64)} {
		tz := sqlscan.NewMySQL(strings.NewReader(in), sqlscan.Options{MaxToken: 32})
		if _, err := tz.Next(); err == nil || !strings.Contains(err.Error(), "longer than 32 bytes") {
			t.Errorf("Next() on %.10q... error = %v, want a too-long error", in, err)
		}
	}
}

// TestMySQLMaxTokenAtEveryLength cuts every kind of token off at every
// length, so each step that reads more than one byte meets the limit.
func TestMySQLMaxTokenAtEveryLength(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"/*!40101", "/* c */", "'a''b'", `'a\'b'`, "0x0f", "0b01", "1.5e+10", "12e5", "X'0f'", "N'x'", "abc"} {
		for limit := 1; limit < len(in); limit++ {
			tz := sqlscan.NewMySQL(strings.NewReader(in), sqlscan.Options{MaxToken: limit})
			_, err := tz.Next()
			if err == nil || !strings.Contains(err.Error(), "is longer than") {
				t.Errorf("Next() on %q with MaxToken %d error = %v, want a too-long error", in, limit, err)
			}
		}
	}
}

// brokenReader returns data, then err.
type brokenReader struct {
	data string
	err  error
}

func (r *brokenReader) Read(p []byte) (int, error) {
	if r.data == "" {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]

	return n, nil
}

func TestMySQLReadErrors(t *testing.T) {
	t.Parallel()

	broken := errors.New("broken pipe")
	for _, in := range []string{"", "'open", "`open", "/* open", "/*", "0x", "abc", "'a\\", "X'0", "N'x", "''", "1e", "1."} {
		tz := sqlscan.NewMySQL(&brokenReader{data: in, err: broken}, sqlscan.Options{})
		var err error
		for err == nil {
			_, err = tz.Next()
		}
		if !errors.Is(err, broken) {
			t.Errorf("Next() on %q then a read error = %v, want the read error", in, err)
		}
	}
}

func TestMySQLRoundTrip(t *testing.T) {
	t.Parallel()

	var generated strings.Builder
	if err := sqltest.WriteDump(&generated, sqltest.MySQL, 300); err != nil {
		t.Fatal(err)
	}

	for name, in := range map[string]string{"fixture": sqltest.Fixture(t, "mysql.sql"), "generated": generated.String()} {
		toks := tokens(t, in)
		if join(toks) != in {
			t.Errorf("%s: tokens do not join back to the input", name)
		}

		counts := map[sqlscan.Kind]int{}
		for _, tok := range toks {
			counts[tok.Kind]++
			if tok.Unterminated {
				t.Errorf("%s: unterminated %v at line %d", name, tok.Kind, tok.Line)
			}
		}
		if counts[sqlscan.DirectiveStart] != counts[sqlscan.DirectiveEnd] {
			t.Errorf("%s: %d directive starts, %d ends", name, counts[sqlscan.DirectiveStart], counts[sqlscan.DirectiveEnd])
		}
		if counts[sqlscan.String] == 0 || counts[sqlscan.Null] == 0 {
			t.Errorf("%s: found %d strings and %d NULLs, want some of each", name, counts[sqlscan.String], counts[sqlscan.Null])
		}
	}
}

func TestKindString(t *testing.T) {
	t.Parallel()

	if got := sqlscan.Punct.String(); got != "Punct" {
		t.Errorf("Punct.String() = %q", got)
	}
	if got := sqlscan.Kind(99).String(); got != "Kind(99)" {
		t.Errorf("Kind(99).String() = %q", got)
	}
}

func FuzzMySQLTokens(f *testing.F) {
	f.Add(sqltest.Fixture(f, "mysql.sql"))
	f.Add("/*!40101 '+'*/ _binary 0x0 X'' b'1 N'\\'' `a``` -- x\n#y\n1.e5 .5e-1 0b2")
	f.Add("'\\")

	f.Fuzz(func(t *testing.T, in string) {
		toks := tokens(t, in)
		if got := join(toks); got != in {
			t.Fatalf("tokens do not join back to the input:\n in %q\nout %q", in, got)
		}

		line := 1
		for _, tok := range toks {
			if tok.Text == "" {
				t.Fatalf("empty %v token in %q", tok.Kind, in)
			}
			if tok.Line != line {
				t.Fatalf("%v token %q on line %d, want %d", tok.Kind, tok.Text, tok.Line, line)
			}
			line += strings.Count(tok.Text, "\n")
			_ = tok.Value()
		}
	})
}

// dumpReader streams a generated dump through a pipe, so the dump is never
// held in memory.
func dumpReader(users int) io.Reader {
	r, w := io.Pipe()
	go func() {
		w.CloseWithError(sqltest.WriteDump(w, sqltest.MySQL, users))
	}()

	return r
}

func drain(tb testing.TB, r io.Reader) int64 {
	tb.Helper()

	tz := sqlscan.NewMySQL(r, sqlscan.Options{})
	var n int64
	for {
		tok, err := tz.Next()
		if errors.Is(err, io.EOF) {
			return n
		}
		if err != nil {
			tb.Fatal(err)
		}
		n += int64(len(tok.Text))
	}
}

func BenchmarkMySQLTokens(b *testing.B) {
	var dump strings.Builder
	if err := sqltest.WriteDump(&dump, sqltest.MySQL, 5000); err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(dump.Len()))
	b.ReportAllocs()
	for b.Loop() {
		drain(b, strings.NewReader(dump.String()))
	}
}

// TestMySQLMemoryIsBounded streams about 10 MiB of generated dump, forcing a
// collection every 512 KiB and recording the live heap each time. Live heap
// is what the tokenizer keeps; total heap size would mostly measure how far
// garbage piles up between collections.
func TestMySQLMemoryIsBounded(t *testing.T) {
	const (
		every     = 512 << 10
		maxGrowth = 1 << 20
	)

	var stats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&stats)
	baseline := stats.HeapAlloc

	tz := sqlscan.NewMySQL(dumpReader(16000), sqlscan.Options{})
	var read, next, peak uint64
	for {
		tok, err := tz.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		read += uint64(len(tok.Text))
		if read >= next {
			next += every
			runtime.GC()
			runtime.ReadMemStats(&stats)
			peak = max(peak, stats.HeapAlloc)
		}
	}

	if read < 4*maxGrowth {
		t.Fatalf("streamed only %d KiB, too little to show a bound", read>>10)
	}
	growth := int64(peak) - int64(baseline)
	t.Logf("live heap grew by %d KiB reading %d KiB", growth>>10, read>>10)
	if growth > maxGrowth {
		t.Errorf("live heap grew by %d KiB reading %d KiB of dump, want at most %d KiB", growth>>10, read>>10, maxGrowth>>10)
	}
}
