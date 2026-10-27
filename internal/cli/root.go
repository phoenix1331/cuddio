// Package cli builds the cuddio command tree and maps its result to an exit code.
package cli

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/phoenix1331/cuddio/internal/config"
	"github.com/phoenix1331/cuddio/internal/detect"
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
	// Rules are the built-in rules before config switches any on or off.
	Rules []detect.Rule
}

// app holds the command tree and the state its commands share.
type app struct {
	env    Env
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

	a := &app{env: env}
	var configPath string
	var flags cleanFlags

	cmd := &cobra.Command{
		Use:           "cuddio [FILE]",
		Short:         "Hide secrets and PII in logs, SQL dumps and text before you paste them into AI tools",
		Long:          longHelp,
		Version:       resolveVersion(env.Version, readBuildInfo),
		Args:          cobra.MaximumNArgs(1),
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
		RunE: func(cmd *cobra.Command, args []string) error {
			// Waiting for typed input would look like a hang, so show help instead
			if len(args) == 0 && isTerminal(env.Stdin) {
				return cmd.Help()
			}
			return a.clean(args, flags)
		},
	}

	cmd.PersistentFlags().StringVar(&configPath, "config", "", "use this config file instead of the nearest "+config.ProjectFile)
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "write to this file instead of stdout; never the input file")
	cmd.Flags().BoolVar(&flags.force, "force", false, "replace the --output file if it exists")
	cmd.Flags().StringVar(&flags.format, "format", "", "input format, text or sql (default: sql for .sql files, otherwise text)")
	cmd.Flags().StringVar(&flags.mode, "mode", "", "placeholder (<EMAIL_1>, reversible) or fake (realistic fake values, so a dump still imports); default from config, else placeholder")
	cmd.Flags().StringVar(&flags.dialect, "dialect", "", "SQL dialect, mysql, postgres or sqlite (default: detected from the start of the dump); implies --format sql")
	cmd.SetVersionTemplate("cuddio {{.Version}}\n")
	cmd.SetIn(env.Stdin)
	cmd.SetOut(env.Stdout)
	cmd.SetErr(env.Stderr)

	a.root = cmd
	return a
}
