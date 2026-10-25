# Learning log

A running record of the Go concepts behind each piece of cuddio.

## gofmt

**What:** a hook that runs `gofmt -w` on every Go file as it is edited, and a CI step that fails if `gofmt -l .` lists anything.

**Why:** Go has one official layout for code (tabs, spacing, brace placement), applied by a tool rather than argued about. Diffs then only show real changes.

**Concept:** Go's toolchain ships its own formatter, test runner (`go test`), checker (`go vet`) and module manager. Third-party tools are the exception, not the rule; `golangci-lint` is the main one, bundling dozens of extra linters.

**Read more:** <https://go.dev/blog/gofmt>

## Modules, main packages and the linker

**What:** the `github.com/phoenix1331/cuddio` module, a tiny `main` package in `cmd/cuddio` that only wires things together, the real logic in `internal/cli`, and tests that prove no network code is in the binary.

**Why:** `internal/` packages cannot be imported by other modules, so cuddio has no accidental public Go API to keep stable. Keeping `main` thin means almost everything is testable in-process: commands receive an `Env` holding their input, output and build information instead of reaching for `os.Stdout` directly.

**Concept:** a Go binary contains only the code that is reachable. Cobra's flag library imports `net` to parse IP address flags, but the linker drops every function cuddio never calls. `go tool nm` lists the symbols that survived, which is a stronger check than looking at imports. `-ldflags "-X main.version=..."` sets a string variable at link time, and `runtime/debug.ReadBuildInfo` reads the module version Go embeds for `go install` builds.

**Read more:** <https://go.dev/doc/modules/layout>
