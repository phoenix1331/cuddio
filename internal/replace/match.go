package replace

import (
	"cmp"
	"slices"
)

// Match is a span of text a rule found.
type Match struct {
	// Start and End are byte offsets, End exclusive.
	Start, End int
	Label      string
	// Priority is the rule's position in the registry; lower wins a tie.
	Priority int
}

// Resolve picks non-overlapping matches from candidates and returns them in
// order of position. The longest span wins an overlap, so a whole DSN beats
// the password inside it; between equal lengths the lower priority wins,
// then the earlier start. Empty or out-of-range spans are dropped.
func Resolve(s string, candidates []Match) []Match {
	valid := make([]Match, 0, len(candidates))
	for _, m := range candidates {
		if m.Start >= 0 && m.Start < m.End && m.End <= len(s) {
			valid = append(valid, m)
		}
	}
	slices.SortFunc(valid, func(a, b Match) int {
		return cmp.Or(
			cmp.Compare(b.End-b.Start, a.End-a.Start),
			cmp.Compare(a.Priority, b.Priority),
			cmp.Compare(a.Start, b.Start),
		)
	})

	var chosen []Match
	for _, m := range valid {
		if !overlapsAny(m, chosen) {
			chosen = append(chosen, m)
		}
	}
	slices.SortFunc(chosen, func(a, b Match) int { return cmp.Compare(a.Start, b.Start) })

	return chosen
}

func overlapsAny(m Match, chosen []Match) bool {
	for _, c := range chosen {
		if m.Start < c.End && c.Start < m.End {
			return true
		}
	}

	return false
}
