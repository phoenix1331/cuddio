package detect

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
)

// builtinAllowed are values cuddio leaves alone by default: reserved
// documentation domains, loopback addresses and keys vendors publish in
// their docs. Matching is exact and case-sensitive, so anything that only
// looks similar is still redacted.
var builtinAllowed = []string{
	// RFC 2606 reserved domains and the conventional test address
	"example.com",
	"example.org",
	"example.net",
	"example.invalid",
	"test@example.com",

	// Loopback
	"127.0.0.1",
	"::1",

	// AWS documentation keys
	"AKIAIOSFODNN7EXAMPLE",
	"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",

	// Stripe documentation keys
	"sk_test_4eC39HqLyjWDarjtT1zdp7dc",
	"pk_test_TYooMQauvdEDq54NiTphI7jx",
}

// Allowlist holds values that are never redacted, even when a rule matches
// them.
type Allowlist struct {
	values   map[string]struct{}
	patterns []*regexp.Regexp
}

// NewAllowlist returns the built-in allowlist extended with exact values and
// RE2 patterns from config. A pattern matches anywhere in a value unless it
// is anchored with ^ and $.
func NewAllowlist(values, patterns []string) (*Allowlist, error) {
	a := &Allowlist{values: make(map[string]struct{}, len(builtinAllowed)+len(values))}
	for _, list := range [][]string{builtinAllowed, values} {
		for _, v := range list {
			a.values[v] = struct{}{}
		}
	}

	for i, p := range patterns {
		re, err := compilePattern(p)
		if err != nil {
			return nil, fmt.Errorf("allowlist pattern %d: %w", i+1, err)
		}
		a.patterns = append(a.patterns, re)
	}

	return a, nil
}

// Allows reports whether value is allowlisted.
func (a *Allowlist) Allows(value string) bool {
	if _, ok := a.values[value]; ok {
		return true
	}
	for _, re := range a.patterns {
		if re.MatchString(value) {
			return true
		}
	}

	return false
}

// compilePattern compiles an RE2 pattern from config. regexp's own errors
// quote the offending part of the pattern, which may hold a secret the user
// is allowlisting, so only the kind of problem is kept.
func compilePattern(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pattern)
	if err == nil {
		return re, nil
	}

	problem := "unrecognised syntax"
	if syntaxErr, ok := errors.AsType[*syntax.Error](err); ok {
		problem = string(syntaxErr.Code)
	}

	return nil, fmt.Errorf("invalid pattern: %s", problem)
}
