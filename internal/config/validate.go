package config

import (
	"errors"
	"fmt"

	"github.com/phoenix1331/cuddio/internal/detect"
	"github.com/phoenix1331/cuddio/internal/replace"
)

// validate checks one file's settings, so an error names the file it is in.
// Rule names in enable and disable are checked when the rules are selected,
// since a custom rule may come from the other file.
func validate(cfg Config) error {
	if cfg.Mode != "" && cfg.Mode != ModePlaceholder && cfg.Mode != ModeFake {
		return errors.New("mode must be placeholder or fake")
	}
	if cfg.Placeholder.Format != "" {
		if _, err := replace.NewPlaceholders(cfg.Placeholder.Format); err != nil {
			return err
		}
	}
	if cfg.Mapping.TTL < 0 {
		return errors.New("mapping ttl must not be negative")
	}
	if _, err := detect.NewAllowlist(nil, cfg.Allowlist.Patterns); err != nil {
		return err
	}
	if _, err := CustomRules(cfg.Rules.Custom); err != nil {
		return err
	}

	return nil
}

// CustomRules compiles config-defined rules. IDs must be unique.
func CustomRules(custom []CustomRule) ([]detect.Rule, error) {
	rules := make([]detect.Rule, 0, len(custom))
	seen := make(map[string]bool, len(custom))
	for _, c := range custom {
		if seen[c.ID] {
			return nil, fmt.Errorf("rule %s: defined more than once", c.ID)
		}
		seen[c.ID] = true

		rule, err := detect.CustomRule(c.ID, c.Category, c.Label, c.Pattern)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}
