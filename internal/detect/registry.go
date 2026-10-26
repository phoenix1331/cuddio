package detect

import (
	"cmp"
	"fmt"
	"slices"
)

// Registry holds every known rule in a fixed order: by category (secrets
// first), then by ID. The order never depends on how rules were registered.
type Registry struct {
	rules []Rule
	byID  map[string]int
}

// NewRegistry checks each rule and returns them as a registry. IDs must be
// unique and must not clash with a category name.
func NewRegistry(rules []Rule) (*Registry, error) {
	sorted := slices.Clone(rules)
	slices.SortFunc(sorted, func(a, b Rule) int {
		return cmp.Or(cmp.Compare(categoryRank(a.Category), categoryRank(b.Category)), cmp.Compare(a.ID, b.ID))
	})

	byID := make(map[string]int, len(sorted))
	for i, rule := range sorted {
		if err := rule.check(); err != nil {
			return nil, err
		}
		if categoryRank(Category(rule.ID)) >= 0 {
			return nil, fmt.Errorf("rule %s: id is a category name", rule.ID)
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
// each a list of rule IDs and category names, in registry order.
//
// A rule starts on unless it is optional. Category entries apply first and
// ID entries override them, so "disable: [pii]" with "enable: [email]"
// leaves only the email rule on among PII rules. At the same level, disable
// wins. An unknown name is an error, so a typo never silently leaves a rule on.
func (r *Registry) Select(enable, disable []string) ([]Rule, error) {
	on := make([]bool, len(r.rules))
	for i, rule := range r.rules {
		on[i] = !rule.Optional
	}

	ids, cats, err := r.split(enable, disable)
	if err != nil {
		return nil, err
	}
	for i, rule := range r.rules {
		if state, ok := cats[rule.Category]; ok {
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

// split sorts enable and disable entries into rule IDs and categories, with
// disable applied last so it wins.
func (r *Registry) split(enable, disable []string) (map[string]bool, map[Category]bool, error) {
	ids := map[string]bool{}
	cats := map[Category]bool{}

	for _, list := range []struct {
		names []string
		state bool
	}{{enable, true}, {disable, false}} {
		for _, name := range list.names {
			if categoryRank(Category(name)) >= 0 {
				cats[Category(name)] = list.state
				continue
			}
			if _, ok := r.byID[name]; !ok {
				return nil, nil, fmt.Errorf("unknown rule or category %q", name)
			}
			ids[name] = list.state
		}
	}

	return ids, cats, nil
}
