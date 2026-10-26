package detect

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/phoenix1331/cuddio/internal/replace"
)

func TestDetectorFind(t *testing.T) {
	t.Parallel()

	email := Rule{ID: "email", Category: PII, Label: "EMAIL", Pattern: regexp.MustCompile(`[a-z]+@[a-z.]+`)}
	card := Rule{ID: "card", Category: PII, Label: "CARD", Pattern: regexp.MustCompile(`\d{16}`), Validate: Luhn}
	digits := Rule{ID: "digits", Category: PII, Label: "NUM", Pattern: regexp.MustCompile(`\d*`)}
	assign := Rule{ID: "assign", Category: Secret, Label: "SECRET", Pattern: regexp.MustCompile(`PASS=(?:"(?P<value>[^"]*)"|(?P<value>\S+))`), Validate: func(v string) bool { return v != "null" }}
	allow, err := NewAllowlist([]string{"ok@example.org"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		rules []Rule
		allow *Allowlist
		in    string
		want  []replace.Match
	}{
		{"no rules", nil, nil, "alice@shop.test", nil},
		{"every occurrence", []Rule{email}, nil, "a@b.c and d@e.f", []replace.Match{
			{Start: 0, End: 5, Label: "EMAIL"},
			{Start: 10, End: 15, Label: "EMAIL"},
		}},
		{"allowlisted value skipped", []Rule{email}, allow, "ok@example.org bob@shop.test", []replace.Match{
			{Start: 15, End: 28, Label: "EMAIL"},
		}},
		{"nil allowlist permits nothing", []Rule{email}, nil, "ok@example.org", []replace.Match{
			{Start: 0, End: 14, Label: "EMAIL"},
		}},
		{"validator rejects", []Rule{card}, nil, "4111111111111112 4111111111111111", []replace.Match{
			{Start: 17, End: 33, Label: "CARD"},
		}},
		{"priority is the rule's position", []Rule{email, card}, nil, "4111111111111111 x@y.z", []replace.Match{
			{Start: 17, End: 22, Label: "EMAIL"},
			{Start: 0, End: 16, Label: "CARD", Priority: 1},
		}},
		{"empty matches ignored", []Rule{digits}, nil, "a1b", []replace.Match{
			{Start: 1, End: 2, Label: "NUM"},
		}},
		{"first value group", []Rule{assign}, nil, `PASS=hunter2 PASS="two words"`, []replace.Match{
			{Start: 5, End: 12, Label: "SECRET"},
			{Start: 19, End: 28, Label: "SECRET"},
		}},
		{"empty value group ignored", []Rule{assign}, nil, `PASS=""`, nil},
		{"validator sees the value group", []Rule{assign}, nil, `PASS=null`, nil},
		{"allowlist sees the value group", []Rule{assign}, allow, `PASS=ok@example.org`, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NewDetector(tt.rules, tt.allow).Find(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Find(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestDetectorKeywords(t *testing.T) {
	t.Parallel()

	token := Rule{ID: "token", Category: Secret, Label: "TOKEN", Pattern: regexp.MustCompile(`(?i)tok_[a-z]+`), Keywords: []string{"tok_"}}
	det := NewDetector([]Rule{token}, nil)

	for in, want := range map[string]int{"tok_abc": 1, "TOK_ABC": 1, "no keyword here": 0} {
		if got := len(det.Find(in)); got != want {
			t.Errorf("Find(%q) = %d matches, want %d", in, got, want)
		}
	}
}
