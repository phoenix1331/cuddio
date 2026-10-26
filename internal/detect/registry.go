package detect

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/phoenix1331/cuddio/internal/names"
)

// Registry holds every known rule in a fixed order: by category (secrets
// first), then source (vendored rules last), then ID. The order never
// depends on how rules were registered.
type Registry struct {
	rules []Rule
	byID  map[string]int
}

// NewRegistry checks each rule and returns them as a registry. IDs must be
// unique and must not clash with a category name.
func NewRegistry(rules []Rule) (*Registry, error) {
	sorted := slices.Clone(rules)
	slices.SortFunc(sorted, func(a, b Rule) int {
		return cmp.Or(
			cmp.Compare(categoryRank(a.Category), categoryRank(b.Category)),
			cmp.Compare(sourceRank(a.Source), sourceRank(b.Source)),
			cmp.Compare(a.ID, b.ID),
		)
	})

	byID := make(map[string]int, len(sorted))
	for i, rule := range sorted {
		if err := rule.check(); err != nil {
			return nil, err
		}
		if isGroupName(rule.ID) {
			return nil, fmt.Errorf("rule %s: id is a category or rule set name", rule.ID)
		}
		if _, ok := byID[rule.ID]; ok {
			return nil, fmt.Errorf("rule %s: defined more than once", rule.ID)
		}
		byID[rule.ID] = i
	}

	return &Registry{rules: sorted, byID: byID}, nil
}

// Rules returns every rule, enabled or not, in registry order.
func (r *Registry) Rules() []Rule {
	return slices.Clone(r.rules)
}

// Select returns the rules that are on after applying enable and disable,
// each a list of rule IDs, category names and rule set names ("gitleaks"),
// in registry order.
//
// A rule starts on unless it is optional. Category entries apply first, then
// rule sets, then IDs, each overriding the last, so "disable: [pii]" with
// "enable: [email]" leaves only the email rule on among PII rules. At the
// same level, disable wins. An unknown name is an error, so a typo never
// silently leaves a rule on.
func (r *Registry) Select(enable, disable []string) ([]Rule, error) {
	on := make([]bool, len(r.rules))
	for i, rule := range r.rules {
		on[i] = !rule.Optional
	}

	ids, groups, err := r.split(enable, disable)
	if err != nil {
		return nil, err
	}
	for i, rule := range r.rules {
		if state, ok := groups[string(rule.Category)]; ok {
			on[i] = state
		}
		if state, ok := groups[string(rule.Source)]; ok {
			on[i] = state
		}
		if state, ok := ids[rule.ID]; ok {
			on[i] = state
		}
	}

	var selected []Rule
	for i, rule := range r.rules {
		if on[i] {
			selected = append(selected, rule)
		}
	}

	return selected, nil
}

// split sorts enable and disable entries into rule IDs and group names
// (categories and rule sets), with disable applied last so it wins.
func (r *Registry) split(enable, disable []string) (ids, groups map[string]bool, err error) {
	ids = map[string]bool{}
	groups = map[string]bool{}

	for _, list := range []struct {
		names []string
		state bool
	}{{enable, true}, {disable, false}} {
		for _, name := range list.names {
			if isGroupName(name) {
				groups[name] = list.state
				continue
			}
			if _, ok := r.byID[name]; !ok {
				return nil, nil, r.unknownName(name)
			}
			ids[name] = list.state
		}
	}

	return ids, groups, nil
}

// unknownName reports a name in enable or disable that matches no rule. The
// name is repeated only as a near miss of a real one, so a value pasted into
// the wrong list is never echoed.
func (r *Registry) unknownName(name string) error {
	known := make([]string, 0, len(categories)+1+len(r.rules))
	for _, c := range categories {
		known = append(known, string(c))
	}
	known = append(known, string(SourceGitleaks))
	for _, rule := range r.rules {
		known = append(known, rule.ID)
	}

	if near, ok := names.Nearest(name, known); ok {
		return fmt.Errorf("unknown rule or category %q in enable or disable; did you mean %q?", name, near)
	}

	return errors.New("unknown rule or category in enable or disable")
}
