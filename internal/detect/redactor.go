package detect

import (
	"maps"
	"strings"

	"github.com/phoenix1331/cuddio/internal/replace"
)

// Tokens hands out the values that replace what a run hides:
// replace.Placeholders or replace.Fakes.
type Tokens interface {
	// Token returns the replacement for value, the same every time.
	Token(label, value string) string
	// IsToken reports whether s is a replacement from an earlier run.
	IsToken(s string) bool
	// Fit shortens a replacement for a column holding at most maxLen
	// characters, where the kind of replacement allows it.
	Fit(token string, maxLen int) string
}

// reserver is implemented by tokens that must never hand out text the
// input already holds, such as replace.Placeholders.
type reserver interface {
	Reserve(s string)
}

// Redactor applies a detector's matches and whole-value replacements with
// one set of tokens, so a value gets the same token whether a rule found it
// in text or it filled a sensitive SQL column.
type Redactor struct {
	detector *Detector
	tokens   Tokens
	// counts holds replacements made so far, by rule ID, or "column:" and
	// the label for whole sensitive-column values.
	counts map[string]int
}

// NewRedactor returns a redactor for one run.
func NewRedactor(d *Detector, t Tokens) *Redactor {
	return &Redactor{detector: d, tokens: t, counts: map[string]int{}}
}

// Redact replaces everything the detector finds in s.
func (r *Redactor) Redact(s string) string {
	r.reserve(s)
	return replace.Apply(s, r.detector.Find(s), func(m replace.Match, value string) string {
		r.counts[m.RuleID]++
		return r.tokens.Token(m.Label, value)
	})
}

// Counts returns how many values have been replaced, by rule ID, with
// whole sensitive-column values under "column:" and their label in lower
// case. It never holds the values themselves.
func (r *Redactor) Counts() map[string]int {
	return maps.Clone(r.counts)
}

// Whole replaces all of value with a token labelled label, fitted to
// maxLen characters (0 for no limit). Blank and allowlisted values, and
// tokens from an earlier run, are returned as they are: they hold nothing
// to hide.
func (r *Redactor) Whole(label, value string, maxLen int) string {
	r.reserve(value)
	if strings.TrimSpace(value) == "" || r.tokens.IsToken(value) || r.detector.allows(value) {
		return value
	}

	r.counts["column:"+strings.ToLower(label)]++
	return r.tokens.Fit(r.tokens.Token(label, value), maxLen)
}

func (r *Redactor) reserve(s string) {
	if rv, ok := r.tokens.(reserver); ok {
		rv.Reserve(s)
	}
}
