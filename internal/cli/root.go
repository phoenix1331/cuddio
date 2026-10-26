// Package cli builds the cuddio command tree and maps its result to an exit code.
package cli

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/phoenix1331/cuddio/internal/config"
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
// commands in-process with their own input, output, directories and build
// information.
type Env struct {
	Version string
	Args    []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	// WorkDir is where the search for .cuddio.yaml starts; empty skips it.
	WorkDir string
	// ConfigDir is the user config directory; empty skips the global file.
	ConfigDir     string
	ReadBuildInfo func() (*debug.BuildInfo, bool)
}

// app holds the command tree and the state its commands share.
type app struct {
	root   *cobra.Command
	config config.Config
}

// Execute runs the command line in env and returns the process exit code.
func Execute(env Env) int {
	a := newApp(env)
	a.root.SetArgs(env.Args)

	if err := a.root.Execute(); err != nil {
		fmt.Fprintf(env.Stderr, "cuddio: %v\n", err)
		return exitError
	}

	return exitOK
}

func newApp(env Env) *app {
	readBuildInfo := env.ReadBuildInfo
	if readBuildInfo == nil {
		readBuildInfo = debug.ReadBuildInfo
	}

	a := &app{}
	var configPath string

	cmd := &cobra.Command{
		Use:           "cuddio",
		Short:         "Hide secrets and PII in logs, SQL dumps and text before you paste them into AI tools",
		Long:          longHelp,
		Version:       resolveVersion(env.Version, readBuildInfo),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			cfg, err := config.Load(config.Sources{
				WorkDir:   env.WorkDir,
				GlobalDir: env.ConfigDir,
				Explicit:  configPath,
			})
			a.config = cfg
			return err
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&configPath, "config", "", "use this config file instead of the nearest "+config.ProjectFile)
	cmd.SetVersionTemplate("cuddio {{.Version}}\n")
	cmd.SetIn(env.Stdin)
	cmd.SetOut(env.Stdout)
	cmd.SetErr(env.Stderr)

	a.root = cmd
	return a
}
