package cli

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/phoenix1331/cuddio/internal/detect"
)

func (a *app) newRulesCmd() *cobra.Command {
	var category string
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List the rules cuddio applies, and which are on",
		Long: `rules lists every rule: built-in, vendored from gitleaks, and custom rules
from config. DEFAULT is the rule's state with no config; ENABLED is its
state after the config files cuddio found switch rules on and off.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return a.listRules(category)
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "only list rules in this category: secret or pii")

	return cmd
}

// listRules writes the rule table, in registry order: secrets, then PII,
// cuddio's own rules before vendored ones.
func (a *app) listRules(category string) error {
	if category != "" && category != string(detect.Secret) && category != string(detect.PII) {
		return errors.New("invalid --category: want secret or pii")
	}
	all, selected, err := a.rules()
	if err != nil {
		return err
	}
	on := map[string]bool{}
	for _, r := range selected {
		on[r.ID] = true
	}

	tw := tabwriter.NewWriter(a.env.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCATEGORY\tLABEL\tDEFAULT\tENABLED\tSOURCE")
	for _, r := range all {
		if category != "" && string(r.Category) != category {
			continue
		}
		source := r.Source
		if source == "" {
			source = detect.SourceBuiltin
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Category, r.Label, onOff(!r.Optional), onOff(on[r.ID]), source)
	}

	return tw.Flush()
}

func onOff(b bool) string {
	if b {
		return "on"
	}

	return "off"
}
