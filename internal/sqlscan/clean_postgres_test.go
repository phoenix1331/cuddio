package sqlscan_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/sqlscan"
)

func cleanPostgres(tb testing.TB, rd sqlscan.Redactor, cols *sqlscan.Columns, in string) string {
	tb.Helper()

	var out bytes.Buffer
	if err := sqlscan.CleanPostgres(&out, strings.NewReader(in), rd, cols, sqlscan.Options{}); err != nil {
		tb.Fatalf("CleanPostgres(%q) error = %v", in, err)
	}

	return out.String()
}

const pgUsers = "CREATE TABLE public.users (\n    id bigint NOT NULL,\n    name text,\n    email text,\n    notes text,\n    CONSTRAINT c CHECK (id > 0)\n);\n"

func TestCleanPostgres(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{
			name: "columns from create table",
			in:   pgUsers + "INSERT INTO public.users VALUES (1, 'Alice', 'a@x.test', 'alice@shop.test');",
			want: pgUsers + "INSERT INTO public.users VALUES (1, '<NAME>', '<EMAIL>', '<EMAIL_1>');",
		},
		{
			name: "string forms",
			in:   `INSERT INTO t (email, n) VALUES ($$it's$$, 'x'), (E'a\nb', $q$alice@shop.test$q$), ('back\slash', E'alice@shop.test\\'), (N'x', 'x');`,
			want: `INSERT INTO t (email, n) VALUES ('<EMAIL>', 'x'), (E'<EMAIL>', '<EMAIL_1>'), ('<EMAIL>', E'<EMAIL_1>\\'), (N'<EMAIL>', 'x');`,
		},
		{
			name: "quotes in a changed value are doubled",
			in:   `SELECT 'O''Brien alice@shop.test', E'O\'Brien \\ alice@shop.test', $$O'Brien \ alice@shop.test$$;`,
			want: `SELECT 'O''Brien <EMAIL_1>', E'O''Brien \\ <EMAIL_1>', 'O''Brien \ <EMAIL_1>';`,
		},
		{
			name: "overriding system value",
			in:   "INSERT INTO t (id, email) OVERRIDING SYSTEM VALUE VALUES (1, 'a') ON CONFLICT DO NOTHING;",
			want: "INSERT INTO t (id, email) OVERRIDING SYSTEM VALUE VALUES (1, '<EMAIL>') ON CONFLICT DO NOTHING;",
		},
		{
			name: "quoted names",
			in:   "INSERT INTO \"Shop\".\"Users\" (\"Email\") VALUES ('a');",
			want: "INSERT INTO \"Shop\".\"Users\" (\"Email\") VALUES ('<EMAIL>');",
		},
		{
			name: "unlogged table",
			in:   "CREATE UNLOGGED TABLE t (phone text);\nINSERT INTO t VALUES ('1');",
			want: "CREATE UNLOGGED TABLE t (phone text);\nINSERT INTO t VALUES ('<PHONE>');",
		},
		{
			name: "partition of has no columns",
			in:   "CREATE TABLE p PARTITION OF t FOR VALUES IN (1);\nINSERT INTO p VALUES ('a');",
			want: "CREATE TABLE p PARTITION OF t FOR VALUES IN (1);\nINSERT INTO p VALUES ('a');",
		},
		{
			name: "copy with a column list",
			in:   "COPY public.t (id, email, notes) FROM stdin;\n1\ta@x.test\tsee alice@shop.test\n2\t\\N\t\n\\.\n",
			want: "COPY public.t (id, email, notes) FROM stdin;\n1\t<EMAIL>\tsee <EMAIL_1>\n2\t\\N\t\n\\.\n",
		},
		{
			name: "copy columns from create table",
			in:   pgUsers + "COPY public.users FROM stdin;\n1\tAlice\ta\\tb\tx\n\\.\n",
			want: pgUsers + "COPY public.users FROM stdin;\n1\t<NAME>\t<EMAIL>\tx\n\\.\n",
		},
		{
			name: "copy escapes",
			in:   "COPY t (notes, n) FROM stdin;\nalice@shop.test\\tx\\\\y\\101\\x41\\v\\u\tz\\N\n\\.\n",
			want: "COPY t (notes, n) FROM stdin;\n<EMAIL_1>\\tx\\\\yAA\\vu\tz\\N\n\\.\n",
		},
		{
			name: "copy unchanged rows keep their bytes",
			in:   "COPY t (notes) FROM stdin;\n\\x41\\101\r\n\\.\r\n",
			want: "COPY t (notes) FROM stdin;\n\\x41\\101\r\n\\.\r\n",
		},
		{
			name: "copy with options is checked whole",
			in:   "COPY t (email) FROM stdin WITH (FORMAT csv);\n\"alice@shop.test\",\"x\"\n\\.\n",
			want: "COPY t (email) FROM stdin WITH (FORMAT csv);\n\"<EMAIL_1>\",\"x\"\n\\.\n",
		},
		{
			name: "copy columns end with the block",
			in:   "COPY t (email) FROM stdin;\na\n\\.\nINSERT INTO t VALUES ('a');",
			want: "COPY t (email) FROM stdin;\n<EMAIL>\n\\.\nINSERT INTO t VALUES ('a');",
		},
		{
			name: "copy to and from a file",
			in:   "COPY t TO stdout;\nCOPY (SELECT 1) TO stdout;\nCOPY t FROM '/tmp/x';\nSELECT 'alice@shop.test';",
			want: "COPY t TO stdout;\nCOPY (SELECT 1) TO stdout;\nCOPY t FROM '/tmp/x';\nSELECT '<EMAIL_1>';",
		},
		{
			name: "meta-commands",
			in:   "\\restrict alice@shop.test\n\\connect alice@shop.test\n\\echo LINE\n\\unrestrict alice@shop.test\n",
			want: "\\restrict alice@shop.test\n\\connect <EMAIL_1>\n\\echo\n\\unrestrict alice@shop.test\n",
		},
		{
			name: "comment and meta-command markers are never redacted",
			in:   "----\n\\echo --\n\\q\n/*--*/",
			want: "--<DASHES>\n\\echo <DASHES>\n\\q\n/*<DASHES>*/",
		},
		{
			name: "nested comments a placeholder would break",
			in:   "/* a /* alice@shop.test */ */ /* OPEN */ /* CLOSE */",
			want: "/* a /* <EMAIL_1> */ */ /**/ /**/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cleanPostgres(t, stubRedactor{}, sqlscan.NewColumns(nil, nil), tt.in)
			if got != tt.want {
				t.Errorf("CleanPostgres()\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

// lineBreaker puts a line break in every redaction, as a placeholder format
// with one would.
type lineBreaker struct{}

func (lineBreaker) Redact(s string) string       { return strings.ReplaceAll(s, "x", "<\r\n>") }
func (lineBreaker) Whole(_, value string) string { return value }

func TestCleanPostgresRawCopyKeepsRows(t *testing.T) {
	t.Parallel()

	in := "COPY t FROM stdin (FORMAT csv);\nx,y\n\\.\n"
	want := "COPY t FROM stdin (FORMAT csv);\n<  >,y\n\\.\n"
	if got := cleanPostgres(t, lineBreaker{}, sqlscan.NewColumns(nil, nil), in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleanPostgresUnchangedRoundTrips(t *testing.T) {
	t.Parallel()

	in := pgUsers + "INSERT INTO public.users VALUES (1, E'\\x41', $$a$$, 'b\\');\r\n\\restrict k\r\nCOPY public.users FROM stdin;\r\n1\t\\N\t\\\\\tx\r\n\\.\r\n-- note\n/* a /* b */ */"
	if got := cleanPostgres(t, identity{}, sqlscan.NewColumns(nil, nil), in); got != in {
		t.Errorf("CleanPostgres() changed input it did not redact:\n got %q\nwant %q", got, in)
	}
}
