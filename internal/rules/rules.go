// Package rules defines cuddio's built-in detection rules.
package rules

import (
	"slices"

	"github.com/phoenix1331/cuddio/internal/detect"
)

// Builtin returns the rules cuddio ships with, freshly built on each call so
// callers can never change another caller's rules.
func Builtin() []detect.Rule {
	return slices.Concat(secrets(), pii())
}

// inCategory sets the category of every rule in rules.
func inCategory(c detect.Category, rules []detect.Rule) []detect.Rule {
	for i := range rules {
		rules[i].Category = c
	}

	return rules
}
