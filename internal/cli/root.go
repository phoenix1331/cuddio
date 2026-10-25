// Package cli builds the cuddio command tree and maps its result to an exit code.
package cli

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/spf13/cobra"
)

const (
	exitOK    = 0
	exitError = 2
)

const longHelp = `cuddio hides secrets and PII (personally identifiable information) in logs,
.env files and SQL dumps before you share them with AI tools. Everything runs
locally: cuddio never makes a network connection.

Redaction is best effort. Review the output before you share it.`

// Env is everything a command needs from the outside world, so tests can run
// commands in-process with their own input, output and build information.
type Env struct {
	Version       string
	Args          []string
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	ReadBuildInfo func() (*debug.BuildInfo, bool)
}

// Execute runs the command line in env and returns the process exit code.
func Execute(env Env) int {
	cmd := newRootCmd(env)
	cmd.SetArgs(env.Args)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(env.Stderr, "cuddio: %v\n", err)
		return exitError
	}

	return exitOK
}

func newRootCmd(env Env) *cobra.Command {
	readBuildInfo := env.ReadBuildInfo
	if readBuildInfo == nil {
		readBuildInfo = debug.ReadBuildInfo
	}

	cmd := &cobra.Command{
		Use:           "cuddio",
		Short:         "Hide secrets and PII in logs, SQL dumps and text before you paste them into AI tools",
		Long:          longHelp,
		Version:       resolveVersion(env.Version, readBuildInfo),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.SetVersionTemplate("cuddio {{.Version}}\n")
	cmd.SetIn(env.Stdin)
	cmd.SetOut(env.Stdout)
	cmd.SetErr(env.Stderr)

	return cmd
}
