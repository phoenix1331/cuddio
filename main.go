// Command cuddio hides secrets and PII in logs, .env files and SQL dumps
// before they are shared with AI tools.
package main

import (
	"io"
	"os"

	"github.com/phoenix1331/cuddio/internal/cli"
)

// version is stamped by release builds with -ldflags "-X main.version=v1.2.3".
var version = ""

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// Without a working or config directory, cuddio skips that config file
	workDir, _ := os.Getwd()
	configDir, _ := os.UserConfigDir()

	return cli.Execute(cli.Env{
		Version:   version,
		Args:      args,
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
		WorkDir:   workDir,
		ConfigDir: configDir,
	})
}
