// Package detect holds cuddio's rule engine: what a rule is, which rules are
// switched on, and how a match is confirmed.
package detect

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Category groups rules so they can be switched on or off together.
type Category string

const (
	// Secret covers credentials: API keys, tokens, passwords and private keys.
	Secret Category = "secret"
	// PII covers personally identifiable information: emails, phone numbers,
	// addresses and similar.
	PII Category = "pii"
)

// categories lists every category in the order rules are sorted by.
// Secrets come first, so a value two rules both match is labelled a secret.
var categories = []Category{Secret, PII}

// Source says where a rule comes from.
type Source string

const (
	// SourceBuiltin rules are written for cuddio. A rule with no source is
	// built in.
	SourceBuiltin Source = "built-in"
	// SourceCustom rules come from config.
	SourceCustom Source = "custom"
	// SourceGitleaks rules are vendored from gitleaks' default config.
	SourceGitleaks Source = "gitleaks"
)

// sourceRank orders rules within a category: vendored rules sort after
// built-in and custom ones, so they lose ties and a value two rules both
// match keeps cuddio's own label.
func sourceRank(s Source) int {
	if s == SourceGitleaks {
		return 1
	}

	return 0
}

// isGroupName reports whether name switches a group of rules on or off: a
// category, or a vendored rule set.
func isGroupName(name string) bool {
	return categoryRank(Category(name)) >= 0 || Source(name) == SourceGitleaks
}

// Rule finds one kind of sensitive value.
type Rule struct {
	// ID is a stable lowercase name such as "aws-access-key", used in config,
	// reports and the rules list.
	ID       string
	Category Category
	// Label names the value in placeholders, such as EMAIL in <EMAIL_1>.
	Label string
	// Pattern is an RE2 expression; Go's regexp package guarantees linear time
	// matching, so no input can make a rule hang. When it has groups named
	// "value", only the first that took part in a match is redacted, so
	// "PASSWORD=" stays readable around the hidden value.
	Pattern *regexp.Regexp
	// Keywords, when set, are lowercase literals of which at least one must
	// appear in the text (compared case-insensitively) before the pattern
	// runs. They are a cheap filter: most lines hold none, so most rules
	// never reach the regexp engine.
	Keywords []string
	// Validate, when set, must accept a match (the value group when the
	// pattern has one) before it counts.
	Validate func(match string) bool
	// Optional rules are off unless enabled by ID or category.
	Optional bool
	// Source is where the rule comes from; empty means built in.
	Source Source
}

// Accepts reports whether a pattern match is a real finding.
func (r Rule) Accepts(match string) bool {
	return r.Validate == nil || r.Validate(match)
}

var (
	validID    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	validLabel = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_]*$`)
)

// check validates r. An invalid ID or category is never repeated in the
// error: config may hold a value in the wrong place, and it could be a secret.
func (r Rule) check() error {
	if err := checkID(r.ID); err != nil {
		return err
	}
	if categoryRank(r.Category) < 0 {
		return fmt.Errorf("rule %s: category must be secret or pii", r.ID)
	}
	if !validLabel.MatchString(r.Label) {
		return fmt.Errorf("rule %s: label must be uppercase letters, digits and underscores", r.ID)
	}
	if r.Pattern == nil {
		return fmt.Errorf("rule %s: no pattern", r.ID)
	}
	for _, k := range r.Keywords {
		if k == "" || k != strings.ToLower(k) {
			return fmt.Errorf("rule %s: keywords must be non-empty and lowercase", r.ID)
		}
	}

	return nil
}

func checkID(id string) error {
	if !validID.MatchString(id) {
		return errors.New("rule id must be lowercase letters, digits and single hyphens")
	}

	return nil
}

func categoryRank(c Category) int {
	return slices.Index(categories, c)
}
