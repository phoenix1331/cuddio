package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/phoenix1331/cuddio/internal/replace"
)

// restoring lets the text and SQL cleaners restore instead: they hand every
// string value and comment to Redact, and every sensitive column value to
// Whole, so tokens are swapped back wherever they sit, and SQL values are
// re-escaped for their dialect.
type restoring struct {
	*replace.Restorer
}

func (r restoring) Redact(s string) string {
	return r.Restore(s)
}

func (r restoring) Whole(_, value string, _ int) string {
	return r.Restore(value)
}

func (a *app) newRestoreCmd() *cobra.Command {
	var flags ioFlags
	cmd := &cobra.Command{
		Use:   "restore [FILE]",
		Short: "Put the real values back into text that uses placeholders from a --map run",
		Long: `restore reads FILE, or stdin, such as an AI's answer, and replaces every
placeholder from the stored session mapping with the value it stands for.
Placeholders cuddio did not hand out are left as they are. SQL input is
re-escaped for its dialect, so a restored dump is still valid SQL.

With --clip, restore works on the clipboard in place. On Windows and WSL
the restored text is marked so clipboard history and cloud sync skip it.

The mapping comes from cleaning with --map, and expires (default: 1 hour).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !flags.clip && isTerminal(a.env.Stdin) {
				return cmd.Help()
			}
			return a.restore(args, flags)
		},
	}
	addIOFlags(cmd, &flags)

	return cmd
}

// restore puts the stored mapping's values back into the input.
func (a *app) restore(args []string, flags ioFlags) error {
	j, err := newJob(args, flags)
	if err != nil {
		return err
	}
	j.private = true
	j.changed, j.unchanged = "clipboard restored", "no placeholders from the stored mapping on the clipboard; left as it was"
	store, err := a.mappingStore()
	if err != nil {
		return err
	}
	pairs, err := store.Load()
	if err != nil {
		return err
	}
	if len(pairs) == 0 {
		return errors.New("no stored mapping to restore from: clean the input with --map first (mappings expire after " + a.config.Mapping.TTL.String() + ")")
	}

	return a.run(j, restoring{replace.NewRestorer(pairs)})
}

func (a *app) newForgetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "forget",
		Short: "Delete stored mappings now, instead of waiting for them to expire",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			store, err := a.mappingStore()
			if err != nil {
				return err
			}
			if err := store.Forget(); err != nil {
				return err
			}
			_, err = fmt.Fprintln(a.env.Stdout, "Stored mappings deleted.")
			return err
		},
	}
}
