package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phoenix1331/cuddio/internal/notices"
)

func TestLicencesCommand(t *testing.T) {
	t.Parallel()

	// A config file cuddio cannot read must not stop the licences printing
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".cuddio.yaml"), []byte("mode: [broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"licences", "licenses"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, Env{WorkDir: dir, Args: []string{name}})

			if got.code != exitOK {
				t.Fatalf("exit code = %d, want %d (stderr %q)", got.code, exitOK, got.stderr)
			}
			if got.stdout != notices.Text() {
				t.Errorf("stdout is not the notices text (%d bytes, want %d)", len(got.stdout), len(notices.Text()))
			}
		})
	}
}
