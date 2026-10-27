// Package replace turns matched values into placeholders, consistently for
// the whole of one run.
package replace

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// DefaultFormat is the placeholder format when config sets none.
const DefaultFormat = "<{label}_{n}>"

// Pair links a placeholder to the value it replaced.
type Pair struct {
	Token string
	Value string
}

// Placeholders hands out one token per distinct value: the same value always
// gets the same token, and each label is numbered from 1 in order of first
// appearance. Use one per run; it is not safe for concurrent use.
type Placeholders struct {
	format  string
	token   *regexp.Regexp
	byValue map[string]string
	counts  map[string]int
	pairs   []Pair
}

// NewPlaceholders returns a placeholder generator for format, which must
// contain {label} and {n} so every token is distinct and can be restored.
func NewPlaceholders(format string) (*Placeholders, error) {
	if !strings.Contains(format, "{label}") || !strings.Contains(format, "{n}") {
		return nil, errors.New("placeholder format must contain {label} and {n}")
	}

	return &Placeholders{format: format, token: tokenPattern(format), byValue: map[string]string{}, counts: map[string]int{}}, nil
}

// tokenPattern matches a whole token in format: {label} as uppercase
// letters, digits and underscores, {n} as a number from 1.
func tokenPattern(format string) *regexp.Regexp {
	fields := strings.NewReplacer(regexp.QuoteMeta("{label}"), `[A-Z0-9_]+`, regexp.QuoteMeta("{n}"), `[1-9][0-9]*`)
	return regexp.MustCompile("^" + fields.Replace(regexp.QuoteMeta(format)) + "$")
}

// IsToken reports whether s is a whole token in this format, such as one
// written by an earlier run, so cleaning cleaned output leaves it alone.
func (p *Placeholders) IsToken(s string) bool {
	return p.token.MatchString(s)
}

// Token returns the placeholder for value. A value seen before keeps its
// first token, even if a different rule (and label) matches it later.
func (p *Placeholders) Token(label, value string) string {
	if token, ok := p.byValue[value]; ok {
		return token
	}

	p.counts[label]++
	token := strings.NewReplacer("{label}", label, "{n}", strconv.Itoa(p.counts[label])).Replace(p.format)
	p.byValue[value] = token
	p.pairs = append(p.pairs, Pair{Token: token, Value: value})

	return token
}

// Pairs returns every token handed out so far, in order of first appearance.
func (p *Placeholders) Pairs() []Pair {
	return append([]Pair(nil), p.pairs...)
}

// Replace swaps the chosen matches in s for placeholders. Overlaps are
// resolved first (see Resolve), and tokens are handed out left to right, so
// numbering follows reading order.
func (p *Placeholders) Replace(s string, matches []Match) string {
	chosen := Resolve(s, matches)
	if len(chosen) == 0 {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, m := range chosen {
		b.WriteString(s[last:m.Start])
		b.WriteString(p.Token(m.Label, s[m.Start:m.End]))
		last = m.End
	}
	b.WriteString(s[last:])

	return b.String()
}
