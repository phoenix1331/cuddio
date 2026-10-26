package detect

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

func rule(id string, cat Category, optional bool) Rule {
	return Rule{ID: id, Category: cat, Label: "VALUE", Pattern: regexp.MustCompile(`x`), Optional: optional}
}

func ids(rules []Rule) string {
	names := make([]string, len(rules))
	for i, r := range rules {
		names[i] = r.ID
	}

	return strings.Join(names, ",")
}

func testRegistry(t *testing.T) *Registry {
	t.Helper()

	reg, err := NewRegistry([]Rule{
		rule("phone", PII, false),
		rule("uk-postcode", PII, true),
		rule("aws-access-key", Secret, false),
		rule("email", PII, false),
		rule("jwt", Secret, false),
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	return reg
}

func TestRegistryOrder(t *testing.T) {
	t.Parallel()

	want := "aws-access-key,jwt,email,phone,uk-postcode"
	if got := ids(testRegistry(t).Rules()); got != want {
		t.Errorf("Rules() = %s, want %s", got, want)
	}

	shuffled := testRegistry(t).Rules()
	slices.Reverse(shuffled)
	reg, err := NewRegistry(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(reg.Rules()); got != want {
		t.Errorf("Rules() from reversed input = %s, want %s", got, want)
	}
}

func TestRegistryRulesReturnsACopy(t *testing.T) {
	t.Parallel()

	reg := testRegistry(t)
	reg.Rules()[0].ID = "changed"

	if got := reg.Rules()[0].ID; got != "aws-access-key" {
		t.Errorf("Rules()[0].ID = %q after changing a returned copy", got)
	}
}

func TestSelect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enable  []string
		disable []string
		want    string
	}{
		{"defaults leave optional rules off", nil, nil, "aws-access-key,jwt,email,phone"},
		{"enable an optional rule by id", []string{"uk-postcode"}, nil, "aws-access-key,jwt,email,phone,uk-postcode"},
		{"enable a category turns on its optional rules", []string{"pii"}, nil, "aws-access-key,jwt,email,phone,uk-postcode"},
		{"disable by id", nil, []string{"jwt"}, "aws-access-key,email,phone"},
		{"disable a category", nil, []string{"secret"}, "email,phone"},
		{"id overrides a disabled category", []string{"email"}, []string{"pii"}, "aws-access-key,jwt,email"},
		{"id overrides an enabled category", []string{"pii"}, []string{"phone"}, "aws-access-key,jwt,email,uk-postcode"},
		{"disable wins over enable for the same id", []string{"jwt"}, []string{"jwt"}, "aws-access-key,email,phone"},
		{"disable wins over enable for the same category", []string{"secret"}, []string{"secret"}, "email,phone"},
		{"everything off", nil, []string{"secret", "pii"}, ""},
	}

	reg := testRegistry(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := reg.Select(tt.enable, tt.disable)
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if ids(got) != tt.want {
				t.Errorf("Select(%v, %v) = %s, want %s", tt.enable, tt.disable, ids(got), tt.want)
			}
		})
	}
}

func TestSelectRejectsUnknownNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enable  []string
		disable []string
		want    string
	}{
		{"near miss of a rule", []string{"emial"}, nil, `unknown rule or category "emial" in enable or disable; did you mean "email"?`},
		{"near miss of a category", nil, []string{"secrets"}, `unknown rule or category "secrets" in enable or disable; did you mean "secret"?`},
		{"value in the wrong list", []string{"sk_live_DO_NOT_ECHO"}, nil, "unknown rule or category in enable or disable"},
		{"lowercase hex key", nil, []string{"4f1c2b9e8d7a6f5e4d3c2b1a0f9e8d7c"}, "unknown rule or category in enable or disable"},
	}

	reg := testRegistry(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := reg.Select(tt.enable, tt.disable)
			if err == nil || err.Error() != tt.want {
				t.Errorf("Select(%v, %v) error = %v, want %q", tt.enable, tt.disable, err, tt.want)
			}
		})
	}
}

func TestNewRegistryRejectsBadRules(t *testing.T) {
	t.Parallel()

	good := rule("email", PII, false)
	noPattern := good
	noPattern.Pattern = nil
	badLabel := good
	badLabel.Label = "email"
	badCategory := good
	badCategory.Category = "personal"
	upperKeyword := good
	upperKeyword.Keywords = []string{"Mail"}
	emptyKeyword := good
	emptyKeyword.Keywords = []string{""}

	tests := []struct {
		name  string
		rules []Rule
		want  string
	}{
		{"empty id", []Rule{rule("", PII, false)}, "id must be"},
		{"uppercase id", []Rule{rule("Email", PII, false)}, "id must be"},
		{"double hyphen", []Rule{rule("aws--key", Secret, false)}, "id must be"},
		{"trailing hyphen", []Rule{rule("aws-", Secret, false)}, "id must be"},
		{"unknown category", []Rule{badCategory}, "rule email: category must be secret or pii"},
		{"lowercase label", []Rule{badLabel}, "label must be"},
		{"no pattern", []Rule{noPattern}, "no pattern"},
		{"uppercase keyword", []Rule{upperKeyword}, "keywords must be non-empty and lowercase"},
		{"empty keyword", []Rule{emptyKeyword}, "keywords must be non-empty and lowercase"},
		{"id is a category", []Rule{rule("pii", PII, false)}, "id is a category name"},
		{"duplicate id", []Rule{good, rule("email", Secret, false)}, "defined more than once"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewRegistry(tt.rules)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("NewRegistry() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestAccepts(t *testing.T) {
	t.Parallel()

	plain := rule("email", PII, false)
	card := Rule{ID: "card", Category: PII, Label: "CARD", Pattern: regexp.MustCompile(`\d+`), Validate: Luhn}

	tests := []struct {
		name  string
		rule  Rule
		match string
		want  bool
	}{
		{"no validator accepts any match", plain, "anything", true},
		{"validator accepts", card, "4242424242424242", true},
		{"validator rejects", card, "4242424242424241", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.rule.Accepts(tt.match); got != tt.want {
				t.Errorf("Accepts(%q) = %v, want %v", tt.match, got, tt.want)
			}
		})
	}
}
