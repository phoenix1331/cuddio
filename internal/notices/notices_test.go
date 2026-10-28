package notices

import (
	"slices"
	"strings"
	"testing"
)

func TestText(t *testing.T) {
	t.Parallel()

	text := Text()
	for _, want := range []string{"MIT License", "gitleaks", "== Go (standard library and runtime) ==", "Apache License"} {
		if !strings.Contains(text, want) {
			t.Errorf("notices missing %q", want)
		}
	}
}

func TestModules(t *testing.T) {
	t.Parallel()

	got := Modules()
	if !slices.Contains(got, "github.com/spf13/cobra") {
		t.Errorf("Modules() = %v, want github.com/spf13/cobra among them", got)
	}
	if slices.ContainsFunc(got, func(m string) bool { return strings.HasPrefix(m, "Go ") }) {
		t.Errorf("Modules() = %v, want Go's own notice left out", got)
	}
	if !slices.IsSorted(got) {
		t.Errorf("Modules() = %v, want sorted, as the script writes them", got)
	}
}
