package main

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/phoenix1331/cuddio/internal/notices"
)

// Release archives ship only the binary, LICENSE and README.md, so the
// licence notices for everything linked in travel inside the binary. These
// tests fail when a dependency or LICENSE changes until
// scripts/update-notices.sh has been run.

func TestNoticesCoverEveryModule(t *testing.T) {
	t.Parallel()

	var linked []string
	for _, goos := range []string{"linux", "darwin", "windows"} {
		cmd := exec.Command(goTool(t), "list", "-deps", "-f", "{{with .Module}}{{.Path}}{{end}}", ".")
		cmd.Env = append(os.Environ(), "GOOS="+goos)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list for %s: %v", goos, err)
		}
		linked = append(linked, strings.Fields(string(out))...)
	}
	slices.Sort(linked)
	linked = slices.DeleteFunc(slices.Compact(linked), func(path string) bool {
		return path == "github.com/phoenix1331/cuddio"
	})

	if got := notices.Modules(); !slices.Equal(got, linked) {
		t.Errorf("notices cover %v, but the build links %v; run scripts/update-notices.sh", got, linked)
	}
}

func TestNoticesIncludeLicenses(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"LICENSE", "internal/rules/LICENSE-gitleaks"} {
		license, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(notices.Text(), string(license)) {
			t.Errorf("notices do not include the current %s; run scripts/update-notices.sh", path)
		}
	}
}
