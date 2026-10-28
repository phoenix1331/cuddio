package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/phoenix1331/cuddio/internal/notices"
)

func (a *app) newLicencesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "licences",
		Aliases: []string{"licenses"},
		Short:   "Print the licences of cuddio and the code built into it",
		Args:    cobra.NoArgs,
		// Licences need no config, so a broken config file must not hide them
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(*cobra.Command, []string) error {
			_, err := io.WriteString(a.env.Stdout, notices.Text())
			return err
		},
	}
}
