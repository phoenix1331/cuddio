package cli

import "runtime/debug"

// resolveVersion prefers the version stamped into release builds, then the
// module version Go records for go install and go build, then "dev".
func resolveVersion(stamped string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if stamped != "" {
		return stamped
	}

	info, ok := readBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}

	return info.Main.Version
}
