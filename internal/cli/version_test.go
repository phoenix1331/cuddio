package cli

import (
	"runtime/debug"
	"testing"
)

func buildInfo(version string, ok bool) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		if !ok {
			return nil, false
		}
		return &debug.BuildInfo{Main: debug.Module{Version: version}}, true
	}
}

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		stamped   string
		buildInfo func() (*debug.BuildInfo, bool)
		want      string
	}{
		{"stamped by release build", "v0.1.0", buildInfo("v0.0.9", true), "v0.1.0"},
		{"go install of a tag", "", buildInfo("v0.1.0", true), "v0.1.0"},
		{"local build from a checkout", "", buildInfo("v0.0.0-20261025120000-abcdef123456+dirty", true), "v0.0.0-20261025120000-abcdef123456+dirty"},
		{"build without version control", "", buildInfo("(devel)", true), "dev"},
		{"empty module version", "", buildInfo("", true), "dev"},
		{"no build information", "", buildInfo("", false), "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveVersion(tt.stamped, tt.buildInfo); got != tt.want {
				t.Errorf("resolveVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
