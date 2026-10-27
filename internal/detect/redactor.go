package detect

import (
	"strings"

	"github.com/phoenix1331/cuddio/internal/replace"
)

// Redactor applies a detector's matches and whole-value replacements with
// one set of placeholders, so a value gets the same token whether a rule
// found it in text or it filled a sensitive SQL column.
type Redactor struct {
	detector     *Detector
	placeholders *replace.Placeholders
}

// NewRedactor returns a redactor for one run.
func NewRedactor(d *Detector, p *replace.Placeholders) *Redactor {
	return &Redactor{detector: d, placeholders: p}
}

// Redact replaces everything the detector finds in s.
func (r *Redactor) Redact(s string) string {
	return r.placeholders.Replace(s, r.detector.Find(s))
}

// Whole replaces all of value with a token labelled label. Blank and
// allowlisted values, and tokens from an earlier run, are returned as they
// are: they hold nothing to hide.
func (r *Redactor) Whole(label, value string) string {
	if strings.TrimSpace(value) == "" || r.placeholders.IsToken(value) || r.detector.allows(value) {
		return value
	}

	return r.placeholders.Token(label, value)
}
