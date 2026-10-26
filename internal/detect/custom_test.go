package detect

import (
	"strings"
	"testing"
)

func TestCustomRule(t *testing.T) {
	t.Parallel()

	rule, err := CustomRule("acme-token", "secret", "ACME", `acme_(?P<value>[0-9a-f]{8})`)
	if err != nil {
		t.Fatalf("CustomRule() error = %v", err)
	}
	if rule.ID != "acme-token" || rule.Category != Secret || rule.Label != "ACME" || rule.Optional {
		t.Errorf("CustomRule() = %+v", rule)
	}
	if got := NewDetector([]Rule{rule}, nil).Find("key acme_deadbeef"); len(got) != 1 || got[0].Start != 9 {
		t.Errorf("Find() with the custom rule = %+v, want the value group only", got)
	}
}

func TestCustomRuleErrors(t *testing.T) {
	t.Parallel()

	const secret = "sk_live_DO_NOT_ECHO"

	tests := []struct {
		name                         string
		id, category, label, pattern string
		want                         string
	}{
		{"invalid regex", "acme", "secret", "ACME", secret + "(", "rule acme: invalid pattern: missing closing )"},
		{"lookahead is not RE2", "acme", "secret", "ACME", secret + "(?=x)", "rule acme: invalid pattern: invalid or unsupported Perl syntax"},
		{"empty pattern", "acme", "secret", "ACME", "", "rule acme: no pattern"},
		{"unknown category", "acme", "credentials", "ACME", "x", `rule acme: unknown category "credentials"`},
		{"missing category", "acme", "", "ACME", "x", `rule acme: unknown category ""`},
		{"lowercase label", "acme", "pii", "acme", "x", "rule acme: label must be uppercase letters, digits and underscores"},
		{"bad id", "Acme Token", "pii", "ACME", "x", `rule "Acme Token": id must be lowercase letters, digits and single hyphens`},
		{"id is a category", "pii", "pii", "ACME", "x", "rule pii: id is a category name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := CustomRule(tt.id, tt.category, tt.label, tt.pattern)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("CustomRule() error = %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "sk_live") {
				t.Errorf("error repeats the pattern: %q", err)
			}
		})
	}
}

func TestWithCustom(t *testing.T) {
	t.Parallel()

	builtin := []Rule{rule("email", PII, false), rule("jwt", Secret, false)}
	custom := []Rule{rule("acme", Secret, false), {ID: "email", Category: PII, Label: "MAIL"}}

	got := WithCustom(builtin, custom)

	if ids(got) != "jwt,acme,email" || got[2].Label != "MAIL" {
		t.Errorf("WithCustom() = %s (email label %q), want jwt,acme,email with the custom email rule", ids(got), got[2].Label)
	}
	if ids(builtin) != "email,jwt" {
		t.Errorf("WithCustom() changed its input: %s", ids(builtin))
	}
}
