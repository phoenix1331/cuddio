// Package config loads cuddio's settings from a global file and a project
// .cuddio.yaml, with project settings layered over global ones.
package config

import (
	"slices"
	"time"
)

// Output modes.
const (
	ModePlaceholder = "placeholder"
	ModeFake        = "fake"
)

// Config is the merged result of the default, global and project settings.
type Config struct {
	// Mode is "placeholder" or "fake".
	Mode        string      `yaml:"mode"`
	Rules       Rules       `yaml:"rules"`
	Allowlist   Allowlist   `yaml:"allowlist"`
	SQL         SQL         `yaml:"sql"`
	Placeholder Placeholder `yaml:"placeholder"`
	Mapping     Mapping     `yaml:"mapping"`
}

// Rules turns built-in rules on or off by ID or category, and adds custom ones.
type Rules struct {
	Enable  []string     `yaml:"enable"`
	Disable []string     `yaml:"disable"`
	Custom  []CustomRule `yaml:"custom"`
}

// CustomRule is a user-defined pattern rule.
type CustomRule struct {
	ID       string `yaml:"id"`
	Pattern  string `yaml:"pattern"`
	Label    string `yaml:"label"`
	Category string `yaml:"category"`
}

// Allowlist holds values that are never redacted, matched exactly or by pattern.
type Allowlist struct {
	Values   []string `yaml:"values"`
	Patterns []string `yaml:"patterns"`
}

// SQL lists extra sensitive columns, for every table or per table.
type SQL struct {
	Columns []string            `yaml:"columns"`
	Tables  map[string][]string `yaml:"tables"`
}

// Placeholder controls how redacted values are written, e.g. "<{label}_{n}>".
type Placeholder struct {
	Format string `yaml:"format"`
}

// Mapping controls the local store that restore reads from.
type Mapping struct {
	TTL time.Duration `yaml:"ttl"`
	// Disabled stops --map from storing anything. Either file can set it and
	// neither can unset it, so a global policy cannot be overridden per project.
	Disabled bool `yaml:"disabled"`
}

// Default returns the settings used when no file sets them.
func Default() Config {
	return Config{
		Mode:        ModePlaceholder,
		Placeholder: Placeholder{Format: "<{label}_{n}>"},
		Mapping:     Mapping{TTL: time.Hour},
	}
}

// merge layers over on top of base: scalars set in over win, lists are
// combined in order (rule toggles in over take precedence), custom rules with the same ID are replaced, and SQL
// table columns are combined per table.
func merge(base, over Config) Config {
	out := base

	if over.Mode != "" {
		out.Mode = over.Mode
	}
	if over.Placeholder.Format != "" {
		out.Placeholder.Format = over.Placeholder.Format
	}
	if over.Mapping.TTL != 0 {
		out.Mapping.TTL = over.Mapping.TTL
	}
	out.Mapping.Disabled = base.Mapping.Disabled || over.Mapping.Disabled

	// A name the later file enables or disables drops out of the other list,
	// so a project can switch back on what the global file switched off
	out.Rules.Enable = slices.Concat(without(base.Rules.Enable, over.Rules.Disable), over.Rules.Enable)
	out.Rules.Disable = slices.Concat(without(base.Rules.Disable, over.Rules.Enable), over.Rules.Disable)
	out.Rules.Custom = mergeCustom(base.Rules.Custom, over.Rules.Custom)
	out.Allowlist.Values = slices.Concat(base.Allowlist.Values, over.Allowlist.Values)
	out.Allowlist.Patterns = slices.Concat(base.Allowlist.Patterns, over.Allowlist.Patterns)
	out.SQL.Columns = slices.Concat(base.SQL.Columns, over.SQL.Columns)
	out.SQL.Tables = mergeTables(base.SQL.Tables, over.SQL.Tables)

	return out
}

func mergeCustom(base, over []CustomRule) []CustomRule {
	out := slices.Clone(base)

	for _, rule := range over {
		i := slices.IndexFunc(out, func(r CustomRule) bool { return r.ID == rule.ID })
		if i >= 0 {
			out[i] = rule
			continue
		}
		out = append(out, rule)
	}

	return out
}

func mergeTables(base, over map[string][]string) map[string][]string {
	if len(base) == 0 && len(over) == 0 {
		return nil
	}

	out := make(map[string][]string, len(base)+len(over))
	for table, columns := range base {
		out[table] = slices.Clone(columns)
	}
	for table, columns := range over {
		out[table] = slices.Concat(out[table], columns)
	}

	return out
}

// without returns names minus any in drop.
func without(names, drop []string) []string {
	return slices.DeleteFunc(slices.Clone(names), func(n string) bool { return slices.Contains(drop, n) })
}
