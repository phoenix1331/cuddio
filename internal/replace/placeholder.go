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
	format string
	// token matches a whole token; inText finds tokens inside text, with
	// the label and number as groups; prefix is the literal text before
	// the first field, a cheap check before inText runs.
	token, inText *regexp.Regexp
	prefix        string
	byValue       map[string]string
	counts        map[string]int
	pairs         []Pair
	// issued holds every token handed out or seeded; reserved holds
	// token-shaped text seen in the input, which Token never hands out.
	issued, reserved map[string]bool
	ambiguous        bool
}

// NewPlaceholders returns a placeholder generator for format, which must
// contain {label} and {n} so every token is distinct and can be restored.
func NewPlaceholders(format string) (*Placeholders, error) {
	if !strings.Contains(format, "{label}") || !strings.Contains(format, "{n}") {
		return nil, errors.New("placeholder format must contain {label} and {n}")
	}

	body := tokenPattern(format)
	prefix, _, _ := strings.Cut(format, "{")

	return &Placeholders{
		format:   format,
		token:    regexp.MustCompile("^" + body + "$"),
		inText:   regexp.MustCompile(body),
		prefix:   prefix,
		byValue:  map[string]string{},
		counts:   map[string]int{},
		issued:   map[string]bool{},
		reserved: map[string]bool{},
	}, nil
}

// tokenPattern matches a token in format: {label} as uppercase letters,
// digits and underscores, {n} as a number from 1, captured as "label" and
// "n".
func tokenPattern(format string) string {
	fields := strings.NewReplacer(regexp.QuoteMeta("{label}"), `(?P<label>[A-Z0-9_]+)`, regexp.QuoteMeta("{n}"), `(?P<n>[1-9][0-9]*)`)
	return fields.Replace(regexp.QuoteMeta(format))
}

// Seed loads pairs from an earlier run, so their values keep their tokens
// and new tokens carry on from the highest number per label. A token that
// does not parse in this format (the format changed) still keeps its value.
func (p *Placeholders) Seed(pairs []Pair) {
	for _, pair := range pairs {
		if _, ok := p.byValue[pair.Value]; ok || p.issued[pair.Token] {
			continue
		}
		p.byValue[pair.Value] = pair.Token
		p.issued[pair.Token] = true
		p.pairs = append(p.pairs, pair)
		if m := p.token.FindStringSubmatch(pair.Token); m != nil {
			label := m[p.token.SubexpIndex("label")]
			n, _ := strconv.Atoi(m[p.token.SubexpIndex("n")])
			p.counts[label] = max(p.counts[label], n)
		}
	}
}

// Reserve notes every token-shaped piece of s, text from the input, so
// Token never hands out a token the input already holds. If s holds a
// token that was already handed out, the run is ambiguous: restore could
// not tell the input's text from the value the token stands for.
func (p *Placeholders) Reserve(s string) {
	if !strings.Contains(s, p.prefix) {
		return
	}
	for _, token := range p.inText.FindAllString(s, -1) {
		if p.issued[token] {
			p.ambiguous = true
		}
		p.reserved[token] = true
	}
}

// Ambiguous reports whether the input held a token this run (or a seeded
// one) also handed out for a real value. A mapping from such a run must
// not be stored.
func (p *Placeholders) Ambiguous() bool {
	return p.ambiguous
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

	var token string
	for token == "" || p.issued[token] || p.reserved[token] {
		p.counts[label]++
		token = strings.NewReplacer("{label}", label, "{n}", strconv.Itoa(p.counts[label])).Replace(p.format)
	}
	p.byValue[value] = token
	p.issued[token] = true
	p.pairs = append(p.pairs, Pair{Token: token, Value: value})

	return token
}

// Pairs returns every token handed out so far, in order of first appearance.
func (p *Placeholders) Pairs() []Pair {
	return append([]Pair(nil), p.pairs...)
}

// Fit returns token unchanged: restore needs a placeholder whole, so it is
// never shortened to fit a column.
func (p *Placeholders) Fit(token string, _ int) string {
	return token
}

// Replace swaps the chosen matches in s for placeholders (see Apply).
func (p *Placeholders) Replace(s string, matches []Match) string {
	return Apply(s, matches, p.Token)
}

// Apply swaps the chosen matches in s for what token returns for each.
// Overlaps are resolved first (see Resolve), and tokens are handed out left
// to right, so numbering follows reading order.
func Apply(s string, matches []Match, token func(label, value string) string) string {
	chosen := Resolve(s, matches)
	if len(chosen) == 0 {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, m := range chosen {
		b.WriteString(s[last:m.Start])
		b.WriteString(token(m.Label, s[m.Start:m.End]))
		last = m.End
	}
	b.WriteString(s[last:])

	return b.String()
}
