package detect

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/phoenix1331/cuddio/internal/replace"
)

func TestRedactor(t *testing.T) {
	t.Parallel()

	allow, err := NewAllowlist([]string{"ok@shop.test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	email := Rule{ID: "email", Category: PII, Label: "EMAIL", Pattern: regexp.MustCompile(`[a-z]+@[a-z.]+`)}
	p, err := replace.NewPlaceholders(replace.DefaultFormat)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRedactor(NewDetector([]Rule{email}, allow), p)

	if got := r.Redact("from a@shop.test and ok@shop.test"); got != "from <EMAIL_1> and ok@shop.test" {
		t.Errorf("Redact() = %q", got)
	}
	tests := []struct{ label, value, want string }{
		{"EMAIL", "a@shop.test", "<EMAIL_1>"},
		{"NAME", "Alice", "<NAME_1>"},
		{"NAME", "Alice", "<NAME_1>"},
		{"NAME", "", ""},
		{"NAME", "  ", "  "},
		{"EMAIL", "ok@shop.test", "ok@shop.test"},
		{"NAME", "<NAME_9>", "<NAME_9>"},
	}
	for _, tt := range tests {
		if got := r.Whole(tt.label, tt.value, 0); got != tt.want {
			t.Errorf("Whole(%q, %q) = %q, want %q", tt.label, tt.value, got, tt.want)
		}
	}
}

func TestRedactorCounts(t *testing.T) {
	t.Parallel()

	allow, err := NewAllowlist([]string{"ok@shop.test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	email := Rule{ID: "email", Category: PII, Label: "EMAIL", Pattern: regexp.MustCompile(`[a-z]+@[a-z.]+`)}
	p, err := replace.NewPlaceholders(replace.DefaultFormat)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRedactor(NewDetector([]Rule{email}, allow), p)

	r.Redact("a@shop.test, a@shop.test and ok@shop.test")
	r.Whole("EMAIL", "b@shop.test", 0)
	r.Whole("BILLING_NAME", "Alice", 0)
	r.Whole("EMAIL", "", 0)
	r.Whole("EMAIL", "ok@shop.test", 0)

	want := map[string]int{"email": 2, "column:email": 1, "column:billing_name": 1}
	got := r.Counts()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Counts() = %v, want %v", got, want)
	}
	got["email"] = 99
	if r.Counts()["email"] != 2 {
		t.Error("Counts() returned the redactor's own map")
	}
}
