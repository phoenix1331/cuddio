// Package notices holds the licence notices for cuddio and everything linked
// into its binary, so release archives can carry only the binary, LICENSE and
// README.md. scripts/update-notices.sh regenerates notices.txt.
package notices

import (
	_ "embed"
	"strings"
)

//go:embed notices.txt
var text string

// Text returns every notice: cuddio's licence (with the gitleaks notice), Go's,
// then each module's under a "== module/path ==" heading.
func Text() string {
	return text
}

// Modules returns the module paths Text has a notice for, in order.
func Modules() []string {
	var modules []string
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		path, ok := strings.CutPrefix(line, "== ")
		if !ok || !strings.HasSuffix(path, " ==") || strings.HasPrefix(path, "Go ") {
			continue
		}
		modules = append(modules, strings.TrimSuffix(path, " =="))
	}

	return modules
}
