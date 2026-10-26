package detect

import "fmt"

// CustomRule builds a rule from config. The pattern must compile as RE2;
// a compile error reports only the kind of problem, never the pattern, which
// may contain the very value being searched for.
func CustomRule(id, category, label, pattern string) (Rule, error) {
	if pattern == "" {
		return Rule{}, fmt.Errorf("rule %s: no pattern", id)
	}
	re, err := compilePattern(pattern)
	if err != nil {
		return Rule{}, fmt.Errorf("rule %s: %w", id, err)
	}

	rule := Rule{ID: id, Category: Category(category), Label: label, Pattern: re}
	if err := rule.check(); err != nil {
		return Rule{}, err
	}
	if categoryRank(Category(id)) >= 0 {
		return Rule{}, fmt.Errorf("rule %s: id is a category name", id)
	}

	return rule, nil
}

// WithCustom returns rules with custom appended, except that a custom rule
// with the same ID as one in rules replaces it, so config can retune a noisy
// built-in.
func WithCustom(rules, custom []Rule) []Rule {
	out := make([]Rule, 0, len(rules)+len(custom))
	replaced := make(map[string]bool, len(custom))
	for _, c := range custom {
		replaced[c.ID] = true
	}
	for _, r := range rules {
		if !replaced[r.ID] {
			out = append(out, r)
		}
	}

	return append(out, custom...)
}
