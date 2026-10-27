package sqlscan_test

import (
	"strconv"
	"testing"

	"github.com/phoenix1331/cuddio/internal/sqlscan"
	"github.com/phoenix1331/cuddio/internal/sqltest"
)

func TestColumnsLabel(t *testing.T) {
	t.Parallel()

	cols := sqlscan.NewColumns([]string{"Notes"}, map[string][]string{"Orders": {"billing_name"}, "sessions": {"user_agent"}})
	tests := []struct {
		table, column string
		want          string
	}{
		{"users", "email", "EMAIL"},
		{"users", "EMAIL", "EMAIL"},
		{"users", "mobile", "PHONE"},
		{"users", "name", "NAME"},
		{"users", "remember_token", "TOKEN"},
		{"orders", "billing_email", "EMAIL"},
		{"orders", "shipping_address", "ADDRESS"},
		{"settings", "smtp_password", "PASSWORD"},
		{"logins", "last_ip_address", "IP"},
		{"orders", "notes", "NOTES"},
		{"anything", "NOTES", "NOTES"},
		{"orders", "billing_name", "NAME"},
		{"ORDERS", "Billing_Name", "NAME"},
		{"users", "full_name", "NAME"},
		{"users", "display_name", "NAME"},
		{"users", "last_name", "NAME"},
		{"orders", "old_billing_name", "NAME"},
		{"orders", "customer_first_name", "NAME"},
		{"sessions", "id", "ID"},
		{"sessions", "payload", "PAYLOAD"},
		{"sessions", "user_agent", "USER_AGENT"},
		{"users", "id", ""},
		{"products", "product_name", ""},
		{"users", "company_name", ""},
		{"users", "emails", ""},
		{"users", "email_verified_at", ""},
	}
	for _, tt := range tests {
		got, ok := cols.Label(tt.table, tt.column)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("Label(%q, %q) = %q, %v, want %q", tt.table, tt.column, got, ok, tt.want)
		}
	}
}

func TestColumnsLabelOfOddNames(t *testing.T) {
	t.Parallel()

	cols := sqlscan.NewColumns([]string{"e-mail 2", "näme"}, nil)
	for column, want := range map[string]string{"e-mail 2": "E_MAIL_2", "näme": "N_ME"} {
		if got, _ := cols.Label("t", column); got != want {
			t.Errorf("Label(%q) = %q, want %q", column, got, want)
		}
	}
}

// lengthStub shows the length Whole was given.
type lengthStub struct{ stubRedactor }

func (lengthStub) Whole(label, _ string, maxLen int) string {
	return "<" + label + ":" + strconv.Itoa(maxLen) + ">"
}

func TestColumnLengths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    sqltest.Dialect
		in   string
		want string
	}{
		{
			name: "mysql types",
			d:    sqltest.MySQL,
			in:   "CREATE TABLE t (`email` varchar(32) NOT NULL, `phone` CHAR(4), `postcode` text, `secret` decimal(10,2), `token` enum('a','b'), `address` varchar(1x));\nINSERT INTO t VALUES ('a','b','c',1,'d','e');",
			want: "CREATE TABLE t (`email` varchar(32) NOT NULL, `phone` CHAR(4), `postcode` text, `secret` decimal(10,2), `token` enum('a','b'), `address` varchar(1x));\nINSERT INTO t VALUES ('<EMAIL:32>','<PHONE:4>','<POSTCODE:0>','<SECRET:0>','<TOKEN:0>','<ADDRESS:0>');",
		},
		{
			name: "explicit column list",
			d:    sqltest.MySQL,
			in:   "CREATE TABLE t (email varchar(9), phone varchar(7));\nINSERT INTO t (phone, email) VALUES ('a','b');",
			want: "CREATE TABLE t (email varchar(9), phone varchar(7));\nINSERT INTO t (phone, email) VALUES ('<PHONE:7>','<EMAIL:9>');",
		},
		{
			name: "postgres types and copy",
			d:    sqltest.Postgres,
			in:   "CREATE TABLE public.t (\n    email character varying(255),\n    card_last4 character(4),\n    phone character varying\n);\nCOPY public.t (email, phone) FROM stdin;\na\tb\n\\.\n",
			want: "CREATE TABLE public.t (\n    email character varying(255),\n    card_last4 character(4),\n    phone character varying\n);\nCOPY public.t (email, phone) FROM stdin;\n<EMAIL:255>\t<PHONE:0>\n\\.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := cleanDump(t, tt.d, lengthStub{}, sqlscan.NewColumns(nil, nil), tt.in); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}
