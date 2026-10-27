package sqlscan_test

import (
	"testing"

	"github.com/phoenix1331/cuddio/internal/sqlscan"
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
		{"orders", "billing_name", "BILLING_NAME"},
		{"ORDERS", "Billing_Name", "BILLING_NAME"},
		{"sessions", "id", "ID"},
		{"sessions", "payload", "PAYLOAD"},
		{"sessions", "user_agent", "USER_AGENT"},
		{"users", "id", ""},
		{"products", "product_name", ""},
		{"users", "billing_name", ""},
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
