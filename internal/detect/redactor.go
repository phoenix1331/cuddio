package detect

import (
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

// Redactor applies a detector's matches and whole-value replacements with
// one set of tokens, so a value gets the same token whether a rule found it
// in text or it filled a sensitive SQL column.
type Redactor struct {
	detector *Detector
	tokens   Tokens
}

// NewRedactor returns a redactor for one run.
func NewRedactor(d *Detector, t Tokens) *Redactor {
	return &Redactor{detector: d, tokens: t}
}

// Redact replaces everything the detector finds in s.
func (r *Redactor) Redact(s string) string {
	return replace.Apply(s, r.detector.Find(s), r.tokens.Token)
}

// Whole replaces all of value with a token labelled label, fitted to
// maxLen characters (0 for no limit). Blank and allowlisted values, and
// tokens from an earlier run, are returned as they are: they hold nothing
// to hide.
func (r *Redactor) Whole(label, value string, maxLen int) string {
	if strings.TrimSpace(value) == "" || r.tokens.IsToken(value) || r.detector.allows(value) {
		return value
	}

	return r.tokens.Fit(r.tokens.Token(label, value), maxLen)
}
