package detect

import (
	"slices"

	"github.com/phoenix1331/cuddio/internal/replace"
)

// Detector runs a set of rules over text and reports what they find.
type Detector struct {
	rules []Rule
	allow *Allowlist
}

// NewDetector returns a detector for rules, in priority order (as Select
// returns them), skipping any match allow permits. A nil allow permits nothing.
func NewDetector(rules []Rule, allow *Allowlist) *Detector {
	return &Detector{rules: slices.Clone(rules), allow: allow}
}

// Find returns every accepted match in s, overlaps included; a rule's
// priority is its position in the detector. replace.Resolve picks between
// overlapping matches.
func (d *Detector) Find(s string) []replace.Match {
	var matches []replace.Match
	for priority, rule := range d.rules {
		for _, loc := range rule.Pattern.FindAllStringIndex(s, -1) {
			value := s[loc[0]:loc[1]]
			if value == "" || !rule.Accepts(value) || (d.allow != nil && d.allow.Allows(value)) {
				continue
			}
			matches = append(matches, replace.Match{Start: loc[0], End: loc[1], Label: rule.Label, Priority: priority})
		}
	}

	return matches
}
