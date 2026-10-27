package sqlscan_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/sqlscan"
	"github.com/phoenix1331/cuddio/internal/sqltest"
)

// pgTokens reads every PostgreSQL token from in, failing the test on an
// error.
func pgTokens(tb testing.TB, in string) []sqlscan.Token {
	tb.Helper()

	var out []sqlscan.Token
	tz := sqlscan.NewPostgres(strings.NewReader(in), sqlscan.Options{})
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

func TestPostgresTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"insert", "INSERT INTO public.users (id, \"Name\") VALUES (1, 'a');", `Word:INSERT Word:INTO Word:public Punct:. Word:users Punct:( Word:id Punct:, Identifier:"Name" Punct:) Word:VALUES Punct:( Number:1 Punct:, String:'a' Punct:) Punct:;`},
		{"backslash is literal", `'a\' 'b'`, `String:'a\' String:'b'`},
		{"doubled quote", "'it''s'", "String:'it''s'"},
		{"e string", `E'it\'s' e'\\'`, `String:E'it\'s' String:e'\\'`},
		{"word ending in e", "type'x'", "Word:type String:'x'"},
		{"national", "N'x\\'", "String:N'x\\'"},
		{"double quotes are names", `"a""b"`, `Identifier:"a""b"`},
		{"dollar quoted", "$$it's$$ $fn$ a $$ b $fn$", "String:$$it's$$ String:$fn$ a $$ b $fn$"},
		{"dollar inside dollar body", "$$$x$$", "String:$$$x$$"},
		{"empty dollar quote", "$$$$", "String:$$$$"},
		{"parameters are not dollar quotes", "$1 $2a$", "Word:$1 Word:$2a$"},
		{"dollar in a name", "a$b$c", "Word:a$b$c"},
		{"dollar then a non-tag byte", "$-$", "Word:$ Punct:- Word:$"},
		{"dollar at the end", "x $", "Word:x Word:$"},
		{"comment without a space", "--x\n1", "Comment:--x Number:1"},
		{"nested comment", "/* a /* b */ c */ 1", "Comment:/* a /* b */ c */ Number:1"},
		{"no directives", "/*!40101 x */", "Comment:/*!40101 x */"},
		{"hash and backtick", "#`", "Punct:# Punct:`"},
		{"cast", "'1'::int", "String:'1' Punct:: Punct:: Word:int"},
		{"meta-command", "\\restrict abc\nSELECT 1;\n\\connect shop", `Meta:\restrict abc Word:SELECT Number:1 Punct:; Meta:\connect shop`},
		{"backslash mid-line", "SELECT \\x", `Word:SELECT Punct:\ Word:x`},
		{"copy block", "COPY t (a, b) FROM stdin;\n1\t\\N\n\\.\nSELECT 1;", "Word:COPY Word:t Punct:( Word:a Punct:, Word:b Punct:) Word:FROM Word:stdin Punct:; CopyData:1\t\\N\n CopyEnd:\\.\n Word:SELECT Number:1 Punct:;"},
		{"copy data starting with spaces", "COPY t FROM STDIN; -- c\n  1\n\n\\.", "Word:COPY Word:t Word:FROM Word:STDIN Punct:; Comment:-- c CopyData:  1\n CopyData:\n CopyEnd:\\."},
		{"copy with crlf", "COPY t FROM stdin;\r\n'x'\r\n\\.\r\n", "Word:COPY Word:t Word:FROM Word:stdin Punct:; CopyData:'x'\r\n CopyEnd:\\.\r\n"},
		{"copy to stdout", "COPY t TO stdout;\n'x'", "Word:COPY Word:t Word:TO Word:stdout Punct:; String:'x'"},
		{"stdin outside copy", "SELECT FROM stdin;\n'x'", "Word:SELECT Word:FROM Word:stdin Punct:; String:'x'"},
		{"copy without an end", "COPY t FROM stdin;\na\tb", "Word:COPY Word:t Word:FROM Word:stdin Punct:; CopyData:a\tb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			toks := pgTokens(t, tt.in)
			if got := brief(toks); got != tt.want {
				t.Errorf("pgTokens(%q) =\n%s\nwant\n%s", tt.in, got, tt.want)
			}
			if got := join(toks); got != tt.in {
				t.Errorf("tokens do not join back to the input: %q", got)
			}
		})
	}
}

func TestPostgresValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{`'plain'`, "plain"},
		{`'it''s'`, "it's"},
		{`'back\slash'`, `back\slash`},
		{`N'x\n'`, `x\n`},
		{`E'it\'s ''x'''`, "it's 'x'"},
		{`E'\b\f\n\r\t\q\\'`, "\b\f\n\r\tq\\"},
		{`E'\101\7\0601'`, "A\a01"},
		{`E'\x41\x4g\xz'`, "A\x04gxz"},
		{`E'\u0041\U0001F600\u12'`, "A\U0001F600u12"},
		{`E'\uD800'`, "\uFFFD"},
		{`E'\UFFFFFFFF'`, "\uFFFD"},
		{`E'end\`, `end\`},
		{"$$a'b$$", "a'b"},
		{"$q$a$$b$q$", "a$$b"},
		{"$q$open", "open"},
		{`"a""b"`, `a"b`},
		{"'open", "open"},
	}

	for _, tt := range tests {
		toks := pgTokens(t, tt.in)
		if len(toks) != 1 {
			t.Fatalf("pgTokens(%q) = %d tokens (%s), want 1", tt.in, len(toks), brief(toks))
		}
		if got := toks[0].Value(); got != tt.want {
			t.Errorf("Value() of %s = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPostgresUnterminated(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"'a", "E'a\\", "$$a", "$t$a$t", `"a`, "/* a /* b */"} {
		toks := pgTokens(t, in)
		if len(toks) != 1 || !toks[0].Unterminated {
			t.Errorf("pgTokens(%q) = %s, want one unterminated token", in, brief(toks))
		}
	}
}

func TestPostgresMaxToken(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		"COPY t FROM stdin;\n" + strings.Repeat("a", 100) + "\n\\.\n",
		"$$" + strings.Repeat("a", 100) + "$$",
		"/* /* " + strings.Repeat("a", 100) + " */ */",
		"E'" + strings.Repeat("a", 100) + "'",
	} {
		tz := sqlscan.NewPostgres(strings.NewReader(in), sqlscan.Options{MaxToken: 64})
		var err error
		for err == nil {
			_, err = tz.Next()
		}
		if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "aaaa") {
			t.Errorf("%.20q: error = %v, want a length error that does not repeat the value", in, err)
		}
	}
}

// TestPostgresMaxTokenAtEveryLength cuts every PostgreSQL-only token off
// at every length, so each step that reads more than one byte meets the
// limit. The COPY case cuts the whitespace before its data.
func TestPostgresMaxTokenAtEveryLength(t *testing.T) {
	t.Parallel()

	// longest is the length of the longest token in each input
	for in, longest := range map[string]int{"E'a'": 4, "/* /* */ */": 11, "$tag$x$tag$": 11, "\\x y": 4, "COPY t FROM stdin;          \n": 11} {
		for limit := 1; limit < longest; limit++ {
			tz := sqlscan.NewPostgres(strings.NewReader(in), sqlscan.Options{MaxToken: limit})
			var err error
			for err == nil {
				_, err = tz.Next()
			}
			if !strings.Contains(err.Error(), "is longer than") {
				t.Errorf("Next() on %q with MaxToken %d error = %v, want a too-long error", in, limit, err)
			}
		}
	}
}

func TestPostgresReadErrors(t *testing.T) {
	t.Parallel()

	broken := errors.New("disk on fire")
	for _, in := range []string{"COPY t FROM stdin;\nab", "$$ab", "$tag", "/* /* ab", "E'ab", "E", "/*", "/* *", "/* /"} {
		tz := sqlscan.NewPostgres(&brokenReader{data: in, err: broken}, sqlscan.Options{})
		var err error
		for err == nil {
			_, err = tz.Next()
		}
		if !errors.Is(err, broken) && !errors.Is(err, io.EOF) {
			t.Errorf("Next() on %q then a read error = %v", in, err)
		}
	}
}

func TestPostgresRoundTrip(t *testing.T) {
	t.Parallel()

	var generated strings.Builder
	if err := sqltest.WriteDump(&generated, sqltest.Postgres, 300); err != nil {
		t.Fatal(err)
	}

	for name, in := range map[string]string{
		"postgres.sql":         sqltest.Fixture(t, "postgres.sql"),
		"postgres-inserts.sql": sqltest.Fixture(t, "postgres-inserts.sql"),
		"generated":            generated.String(),
	} {
		toks := pgTokens(t, in)
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
		if counts[sqlscan.Meta] != 2 {
			t.Errorf("%s: %d meta-commands, want 2 (\\restrict and \\unrestrict)", name, counts[sqlscan.Meta])
		}
		if counts[sqlscan.CopyData] > 0 && counts[sqlscan.CopyEnd] == 0 {
			t.Errorf("%s: COPY data without an end", name)
		}
	}
}

func FuzzPostgresTokens(f *testing.F) {
	f.Add(sqltest.Fixture(f, "postgres.sql"))
	f.Add(sqltest.Fixture(f, "postgres-inserts.sql"))
	f.Add("E'\\x4' $a$ $ $a$ /* /* */ \"x\" \\m\nCOPY t FROM stdin;\n\\N\n\\.")

	f.Fuzz(func(t *testing.T, in string) {
		toks := pgTokens(t, in)
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
