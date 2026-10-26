package detect

import (
	"slices"

	"github.com/phoenix1331/cuddio/internal/replace"
)

// Detector runs a set of rules over text and reports what they find.
type Detector struct {
	rules []Rule
	// groups holds, per rule, the submatch indexes of its "value" groups.
	groups [][]int
	allow  *Allowlist
}

// NewDetector returns a detector for rules, in priority order (as Select
// returns them), skipping any match allow permits. A nil allow permits nothing.
func NewDetector(rules []Rule, allow *Allowlist) *Detector {
	d := &Detector{rules: slices.Clone(rules), groups: make([][]int, len(rules)), allow: allow}
	for i, rule := range d.rules {
		for g, name := range rule.Pattern.SubexpNames() {
			if name == valueGroup {
				d.groups[i] = append(d.groups[i], g)
			}
		}
	}

	return d
}

const valueGroup = "value"

// Find returns every accepted match in s, overlaps included; a rule's
// priority is its position in the detector. replace.Resolve picks between
// overlapping matches.
func (d *Detector) Find(s string) []replace.Match {
	var matches []replace.Match
	for priority, rule := range d.rules {
		for _, loc := range rule.Pattern.FindAllStringSubmatchIndex(s, -1) {
			start, end := span(loc, d.groups[priority])
			value := s[start:end]
			if value == "" || !rule.Accepts(value) || (d.allow != nil && d.allow.Allows(value)) {
				continue
			}
			matches = append(matches, replace.Match{Start: start, End: end, Label: rule.Label, Priority: priority})
		}
	}

	return matches
}

// span returns the first value group that took part in the match, or the
// whole match when there is none.
func span(loc, groups []int) (start, end int) {
	for _, g := range groups {
		if loc[2*g] >= 0 {
			return loc[2*g], loc[2*g+1]
		}
	}

	return loc[0], loc[1]
}
