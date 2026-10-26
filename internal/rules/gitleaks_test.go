package rules

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/detect"
	"github.com/phoenix1331/cuddio/internal/replace"
)

// hexFake returns n deterministic lowercase hex characters.
func hexFake(n int) string {
	return strings.Repeat("0123456789abcdef", n/16+1)[:n]
}

// cleanAll runs s through every rule cuddio ships, as the CLI does with no
// config.
func cleanAll(t *testing.T, s string) string {
	t.Helper()

	reg, err := detect.NewRegistry(All())
	if err != nil {
		t.Fatal(err)
	}
	selected, err := reg.Select(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	allow, err := detect.NewAllowlist(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := replace.NewPlaceholders(replace.DefaultFormat)
	if err != nil {
		t.Fatal(err)
	}

	return p.Replace(s, detect.NewDetector(selected, allow).Find(s))
}

func gitleaksRule(t *testing.T, id string) detect.Rule {
	t.Helper()

	i := slices.IndexFunc(Gitleaks(), func(r detect.Rule) bool { return r.ID == id })
	if i < 0 {
		t.Fatalf("no vendored rule %q", id)
	}

	return Gitleaks()[i]
}

func TestGitleaksRulesLoad(t *testing.T) {
	t.Parallel()

	vendored := Gitleaks()
	if len(vendored) < 150 {
		t.Fatalf("Gitleaks() = %d rules, want the upstream set", len(vendored))
	}
	if _, err := detect.NewRegistry(vendored); err != nil {
		t.Fatalf("NewRegistry(Gitleaks()) error = %v", err)
	}
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(gitleaksVersion) {
		t.Errorf("gitleaksVersion = %q, want a release tag", gitleaksVersion)
	}

	for _, r := range vendored {
		if r.Source != detect.SourceGitleaks || r.Category != detect.Secret || r.Optional || len(r.Keywords) == 0 {
			t.Errorf("rule %s = source %q, category %q, optional %v, %d keywords; want gitleaks, secret, on, with keywords", r.ID, r.Source, r.Category, r.Optional, len(r.Keywords))
		}
	}
	for _, id := range []string{"generic-api-key", "gcp-api-key", "kubernetes-secret-yaml", "pkcs12-file"} {
		if slices.ContainsFunc(vendored, func(r detect.Rule) bool { return r.ID == id }) {
			t.Errorf("Gitleaks() includes %s, which is left out on purpose", id)
		}
	}
}

func TestAllPrefersBuiltins(t *testing.T) {
	t.Parallel()

	all := All()
	if _, err := detect.NewRegistry(all); err != nil {
		t.Fatalf("NewRegistry(All()) error = %v", err)
	}

	clashes := 0
	for _, b := range Builtin() {
		matching := slices.DeleteFunc(slices.Clone(all), func(r detect.Rule) bool { return r.ID != b.ID })
		if len(matching) != 1 || matching[0].Source == detect.SourceGitleaks {
			t.Errorf("All() has %d rules with ID %s, want only the built-in", len(matching), b.ID)
		}
		if slices.ContainsFunc(Gitleaks(), func(r detect.Rule) bool { return r.ID == b.ID }) {
			clashes++
		}
	}
	if clashes == 0 {
		t.Error("no vendored rule shares a built-in ID, so de-duplication is untested")
	}
	if want := len(Builtin()) + len(Gitleaks()) - clashes; len(all) != want {
		t.Errorf("len(All()) = %d, want %d", len(all), want)
	}
}

func TestGitleaksMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"gitlab", "token glpat-" + fake(20), "token <GITLAB_PAT_1>"},
		{"npm", "publish with npm_" + fake(36), "publish with <NPM_ACCESS_TOKEN_1>"},
		{"built-in label wins a longer match", "//registry.npmjs.org/:_authToken=npm_" + fake(36), "//registry.npmjs.org/:_authToken=<SECRET_1>"},
		{"digitalocean", "DO dop_v1_" + hexFake(64) + " ok", "DO <DIGITALOCEAN_PAT_1> ok"},
		{"shopify", "shop shpat_" + hexFake(32), "shop <SHOPIFY_ACCESS_TOKEN_1>"},
		{"sendgrid", `key "SG.` + fake(66) + `"`, `key "<SENDGRID_API_TOKEN_1>"`},
		{"leading digit label", "A3-ABC123-DEF456GHI78-JKL90-MNO12-PQR34", "<1PASSWORD_SECRET_KEY_1>"},
		{"built-in wins a tie", "ghp_" + fake(36), "<GITHUB_TOKEN_1>"},
		{"low entropy", "token glpat-aaaaaaaaaaaaaaaaaaaa", "token glpat-aaaaaaaaaaaaaaaaaaaa"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := cleanAll(t, tt.in); got != tt.want {
				t.Errorf("clean(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGitleaksValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rule   string
		secret string
		want   bool
	}{
		{"accepted", "aws-access-token", "AKIAQWERTYUI2345ABCD", true},
		{"at the entropy threshold", "aws-access-token", "AKIAAAAAAAAAAAAAAAAA", false},
		{"rule allowlist regex", "aws-access-token", "AKIAQWERTYUI2EXAMPLE", false},
		{"global allowlist regex", "npm-access-token", "${NPM_TOKEN}", false},
		{"global stopword, any case", "npm-access-token", "npm_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", false},
		{"no entropy threshold", "adafruit-api-key", "aaaabbbbccccddddeeeeffffgggghhhh", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := gitleaksRule(t, tt.rule).Accepts(tt.secret); got != tt.want {
				t.Errorf("%s.Accepts(%q) = %v, want %v", tt.rule, tt.secret, got, tt.want)
			}
		})
	}
}

func TestGitleaksRuleSetToggle(t *testing.T) {
	t.Parallel()

	selectIDs := func(rules []detect.Rule, disable ...string) string {
		reg, err := detect.NewRegistry(rules)
		if err != nil {
			t.Fatal(err)
		}
		selected, err := reg.Select(nil, disable)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, len(selected))
		for i, r := range selected {
			ids[i] = r.ID
		}
		return strings.Join(ids, ",")
	}

	if got, want := selectIDs(All(), "gitleaks"), selectIDs(Builtin()); got != want {
		t.Errorf("disable: [gitleaks] leaves %s on, want the default built-ins %s", got, want)
	}
}
