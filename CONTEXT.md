# Context

**cuddio** (Welsh, "to hide") is a local-only Go CLI that hides secrets and PII in logs, `.env` files and MySQL, PostgreSQL and SQLite dumps before they are shared with AI tools. It replaces values with reversible placeholders or format-preserving fakes, understands SQL columns, works on the clipboard, and can fail CI when something would be redacted. It never makes a network connection.

Module `github.com/phoenix1331/cuddio`, binary `cuddio`, config `.cuddio.yaml`.

## Family

First of a planned set of Welsh-named Go tools sharing one visual style for README GIFs and GitHub Pages sites. No sibling exists yet.

## Architecture

| Path | Role |
|---|---|
| `main.go` | `main` package at the root so `go install github.com/phoenix1331/cuddio@latest` works; wiring only, holds the ldflags `version` |
| `network_test.go` | No-network checks: build imports and linked binary symbols |
| `internal/cli/` | Cobra commands; `Env` carries args, stdin, stdout, stderr and build info so commands run in-process in tests |
| `internal/detect/` | Rule engine, rule definitions, allowlist (planned) |
| `internal/rules/` | Built-in and vendored gitleaks rules (planned) |
| `internal/textscan/` | Streaming scanner for plain text and logs (planned) |
| `internal/sqlscan/` | Streaming SQL tokeniser, dialects, column tracking (planned) |
| `internal/replace/` | Placeholders and fake values, consistent within a run (planned) |
| `internal/mapstore/` | Local mapping store, `0600` files with TTL (planned) |
| `internal/clipboard/` | Clipboard access including WSL (planned) |
| `internal/config/` | Project and global config loading and merging (planned) |
| `scripts/check.sh` | The quality gate, run locally and in CI |
| `.github/scripts/` | `check-archives.sh` (release archive allow list), `check-module-size.sh` (module zip budget) |
| `.githooks/` | `commit-msg` and `pre-commit`, enabled with `git config core.hooksPath .githooks` |

## Decisions

| Decision | Why |
|---|---|
| Go, single static binary, `CGO_ENABLED=0` | No runtime for users to install; cross-compiles for six targets from one machine |
| `go` directive 1.26, CI on 1.26 and 1.27, releases built with 1.27 | Go supports the latest two releases; a 1.27 floor would make the 1.26 CI job meaningless and force a toolchain download for 1.26 users |
| Cobra for commands | Brief's choice; subcommands, help and completion for free |
| No-network test checks imports and the linked binary | `spf13/pflag` imports `net` only to parse IP flags. Imports fail on any `net/*` except `net/url` and `net/netip` and on unlisted importers of `net`; `go tool nm` fails on any dial, listen, resolver, HTTP, TLS, SMTP or RPC symbol. Stronger than an import ban, and cobra stays |
| `main` at the repo root | Shorter install command; one binary only, all logic in `internal/` |
| Module zip size budget instead of nested modules | Module zips ignore `export-ignore` (verified against `proxy.golang.org`); a CI cap of 1 MB total and 256 KB for `examples/` keeps `go install` downloads small. `export-ignore` still trims GitHub source archives |
| Large test inputs generated in code | Keeps the module zip small; benchmarks still exercise gigabyte-scale behaviour |
| Fixture secrets are vendor test keys or assembled at test time | GitHub push protection blocks realistic live-format tokens; obviously fake values also avoid false alarms in users' module caches |
| `scripts/*.sh` as the command surface | No Makefile (brief); POSIX `sh` runs in WSL, macOS and Git Bash on Windows CI |
| `.githooks/` with `core.hooksPath`, not Husky | No Node or `package.json` in a Go repo |
| Dependabot with `commit-message.prefix: chore`, no scope, grouped | Titles become `chore: bump ...`, matching the lowercase no-scope convention |
| Exit codes 0 ok, 1 findings (`--check`), 2 error | Lets CI distinguish "would redact" from "broke" |
| Help text ends with a best-effort notice | Redaction cannot be guaranteed; MIT's "as is" covers the legal side, the notice covers the practical one |
| MIT licence | Compatible with vendoring MIT-licensed gitleaks rules |
| `SECURITY.md` points to GitHub private advisories | No personal email address published |

## Dependencies and constraints

| Module | Version | Why |
|---|---|---|
| Go | `go 1.26.0` floor; 1.27.3 for development and releases | Latest two supported releases |
| `github.com/spf13/cobra` | v1.10.2 | Command tree, flags, help |
| `github.com/spf13/pflag` | v1.0.9 (indirect) | Cobra's flags; the only allowed importer of `net` |
| `github.com/inconshreveable/mousetrap` | v1.1.0 (indirect) | Cobra's Windows Explorer launch detection |

Tools (not module dependencies): golangci-lint v2.15, govulncheck, GoReleaser v2, actionlint.

## CI

| Workflow | Runs |
|---|---|
| `test.yml` | `scripts/check.sh` on Go 1.26 and 1.27 x Linux, macOS, Windows (`shell: bash`); then module size check, GoReleaser snapshot and `check-archives.sh` |
| `govulncheck.yml` | Every push, every pull request, weekly |
| `.github/dependabot.yml` | Weekly, Go modules and Actions, one grouped PR each |

GoReleaser is snapshot only (`release.disable: true`) until the release workflow, Homebrew tap and Scoop bucket exist.

## Local setup

WSL with Go in `/usr/local/go` and `$HOME/go/bin` on `PATH`. Install golangci-lint, govulncheck and GoReleaser with `go install`, run `git config core.hooksPath .githooks` once, then `sh scripts/check.sh`. No Docker, no Makefile, no database servers: SQL fixtures are plain text.
