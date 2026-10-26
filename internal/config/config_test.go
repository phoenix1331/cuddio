package config

import (
	"reflect"
	"testing"
	"time"
)

func TestMerge(t *testing.T) {
	t.Parallel()

	global := Config{
		Mode:        "fake",
		Rules:       Rules{Disable: []string{"ipv4"}, Custom: []CustomRule{{ID: "acme", Pattern: "acme_[0-9]+", Label: "ACME", Category: "secret"}, {ID: "corp", Pattern: "corp-[a-z]+"}}},
		Allowlist:   Allowlist{Values: []string{"test@example.com"}, Patterns: []string{"^dev-"}},
		SQL:         SQL{Columns: []string{"notes"}, Tables: map[string][]string{"users": {"nickname"}}},
		Placeholder: Placeholder{Format: "[{label}-{n}]"},
		Mapping:     Mapping{TTL: 30 * time.Minute, Disabled: true},
	}
	project := Config{
		Mode:      "placeholder",
		Rules:     Rules{Enable: []string{"uk-postcode"}, Disable: []string{"ipv6"}, Custom: []CustomRule{{ID: "acme", Pattern: "acme_[a-z0-9]+", Label: "ACME_TOKEN", Category: "secret"}, {ID: "team", Pattern: "team-[0-9]+"}}},
		Allowlist: Allowlist{Values: []string{"ops@example.com"}},
		SQL:       SQL{Columns: []string{"comment"}, Tables: map[string][]string{"users": {"bio"}, "orders": {"delivery_note"}}},
		Mapping:   Mapping{TTL: 2 * time.Hour},
	}

	want := Config{
		Mode: "placeholder",
		Rules: Rules{
			Enable:  []string{"uk-postcode"},
			Disable: []string{"ipv4", "ipv6"},
			Custom: []CustomRule{
				{ID: "acme", Pattern: "acme_[a-z0-9]+", Label: "ACME_TOKEN", Category: "secret"},
				{ID: "corp", Pattern: "corp-[a-z]+"},
				{ID: "team", Pattern: "team-[0-9]+"},
			},
		},
		Allowlist:   Allowlist{Values: []string{"test@example.com", "ops@example.com"}, Patterns: []string{"^dev-"}},
		SQL:         SQL{Columns: []string{"notes", "comment"}, Tables: map[string][]string{"users": {"nickname", "bio"}, "orders": {"delivery_note"}}},
		Placeholder: Placeholder{Format: "[{label}-{n}]"},
		Mapping:     Mapping{TTL: 2 * time.Hour, Disabled: true},
	}

	if got := merge(global, project); !reflect.DeepEqual(got, want) {
		t.Errorf("merge() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestMergeLeavesInputsUntouched(t *testing.T) {
	t.Parallel()

	base := Config{Rules: Rules{Disable: make([]string, 1, 4)}, SQL: SQL{Tables: map[string][]string{"users": make([]string, 1, 4)}}}
	base.Rules.Disable[0] = "ipv4"
	base.SQL.Tables["users"][0] = "nickname"

	merged := merge(base, Config{Rules: Rules{Disable: []string{"ipv6"}}, SQL: SQL{Tables: map[string][]string{"users": {"bio"}}}})
	merged.Rules.Disable[0] = "changed"
	merged.SQL.Tables["users"][0] = "changed"

	if base.Rules.Disable[0] != "ipv4" || base.SQL.Tables["users"][0] != "nickname" {
		t.Errorf("merge() shares storage with its input: %+v", base)
	}
}

func TestMergeWithoutTablesStaysNil(t *testing.T) {
	t.Parallel()

	if got := merge(Default(), Config{}); got.SQL.Tables != nil {
		t.Errorf("SQL.Tables = %v, want nil", got.SQL.Tables)
	}
}

func TestMergeKeepsBaseScalarsWhenUnset(t *testing.T) {
	t.Parallel()

	if got := merge(Default(), Config{}); !reflect.DeepEqual(got, Default()) {
		t.Errorf("merge(Default(), empty) = %+v, want %+v", got, Default())
	}
}
