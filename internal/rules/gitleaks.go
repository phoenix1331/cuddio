package rules

import (
	"regexp"
	"slices"
	"strings"

	"github.com/phoenix1331/cuddio/internal/detect"
)

// vendoredRule is a gitleaks rule as generated into gitleaks_gen.go. Its
// pattern already names the secret's capture groups "value".
type vendoredRule struct {
	ID       string
	Pattern  string
	Keywords []string
	// Entropy, when set, is the Shannon entropy the secret must exceed.
	Entropy float64
	Allow   vendoredAllowlist
}

// vendoredAllowlist holds the parts of a gitleaks allowlist that apply to
// text with no file path: regexes matched against the secret, and stopwords
// that allow any secret containing them.
type vendoredAllowlist struct {
	Regexes   []string
	Stopwords []string
}

// allowlist is a compiled vendoredAllowlist.
type allowlist struct {
	regexes   []*regexp.Regexp
	stopwords []string
}

func compileAllowlist(v vendoredAllowlist) allowlist {
	a := allowlist{stopwords: v.Stopwords}
	for _, re := range v.Regexes {
		a.regexes = append(a.regexes, regexp.MustCompile(re))
	}

	return a
}

func (a allowlist) allows(secret string) bool {
	for _, re := range a.regexes {
		if re.MatchString(secret) {
			return true
		}
	}
	lower := strings.ToLower(secret)
	for _, word := range a.stopwords {
		if strings.Contains(lower, word) {
			return true
		}
	}

	return false
}

// Gitleaks returns the rules vendored from gitleaks' default config, all in
// the secret category and on by default. Each keeps gitleaks' checks: the
// secret must exceed the rule's entropy and pass gitleaks' global and rule
// allowlists. Patterns are checked by the generator, so compiling them here
// cannot fail.
func Gitleaks() []detect.Rule {
	global := compileAllowlist(gitleaksGlobalAllowlist())
	vendored := gitleaksRules()

	out := make([]detect.Rule, len(vendored))
	for i, v := range vendored {
		allow := compileAllowlist(v.Allow)
		entropy := v.Entropy
		out[i] = detect.Rule{
			ID:       v.ID,
			Category: detect.Secret,
			Label:    strings.ToUpper(strings.ReplaceAll(v.ID, "-", "_")),
			Pattern:  regexp.MustCompile(v.Pattern),
			Keywords: v.Keywords,
			Source:   detect.SourceGitleaks,
			Validate: func(secret string) bool {
				if entropy > 0 && detect.Entropy(secret) <= entropy {
					return false
				}
				return !global.allows(secret) && !allow.allows(secret)
			},
		}
	}

	return out
}

// All returns the built-in rules followed by the vendored gitleaks rules,
// leaving out any vendored rule with a built-in's ID.
func All() []detect.Rule {
	builtin := Builtin()
	ids := make([]string, len(builtin))
	for i, r := range builtin {
		ids[i] = r.ID
	}

	vendored := slices.DeleteFunc(Gitleaks(), func(r detect.Rule) bool {
		return slices.Contains(ids, r.ID)
	})

	return slices.Concat(builtin, vendored)
}
