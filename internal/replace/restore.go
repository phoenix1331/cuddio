package replace

import (
	"cmp"
	"slices"
	"strings"
)

// Restorer puts real values back in place of their tokens.
type Restorer struct {
	replacer *strings.Replacer
}

// NewRestorer returns a restorer for pairs. Longer tokens are tried first,
// so with a format that has nothing after {n}, EMAIL_10 is never read as
// EMAIL_1 followed by a 0.
func NewRestorer(pairs []Pair) *Restorer {
	sorted := slices.Clone(pairs)
	slices.SortFunc(sorted, func(a, b Pair) int {
		return cmp.Or(cmp.Compare(len(b.Token), len(a.Token)), cmp.Compare(a.Token, b.Token))
	})
	oldnew := make([]string, 0, 2*len(sorted))
	for _, p := range sorted {
		oldnew = append(oldnew, p.Token, p.Value)
	}

	return &Restorer{replacer: strings.NewReplacer(oldnew...)}
}

// Restore replaces every known token in s with its value. Unknown tokens,
// such as ones an AI made up, are left as they are.
func (r *Restorer) Restore(s string) string {
	return r.replacer.Replace(s)
}
