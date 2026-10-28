# Contributing to cuddio

Thanks for helping. Bug reports, fixes and new rules are all welcome.

Never paste real secrets or personal data into an issue or pull request, even redacted. Use obviously fake values (`example.com`, `example.invalid`, documented vendor test keys).

## Setup

cuddio is developed in WSL (Ubuntu) on Windows, and works the same on Linux and macOS. No Docker, database servers or Makefile are needed: SQL fixtures are plain text, and `scripts/*.sh` are the only commands.

1. Install Go 1.26 or later from [go.dev/dl](https://go.dev/dl/). In WSL, unpack it to `/usr/local/go` and add `/usr/local/go/bin` and `$HOME/go/bin` to your `PATH` in `~/.profile`.
2. Install the tools the gate uses:

   ```bash
   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
   go install golang.org/x/vuln/cmd/govulncheck@latest
   go install github.com/goreleaser/goreleaser/v2@latest   # release snapshots
   go install github.com/rhysd/actionlint/cmd/actionlint@latest   # workflow changes
   ```

3. Clone and activate the git hooks:

   ```bash
   git clone https://github.com/phoenix1331/cuddio
   cd cuddio
   git config core.hooksPath .githooks
   sh scripts/check.sh
   ```

Re-rendering the GIFs also needs [VHS](https://github.com/charmbracelet/vhs), `ttyd` and `ffmpeg` (see below).

## Running checks

`sh scripts/check.sh` runs the full gate, in this order:

| Command | What it does |
|---|---|
| `gofmt -l .` | Lists unformatted files; must print nothing |
| `go vet ./...` | Go's built-in correctness checks |
| `golangci-lint run` | Extra linters, configured in `.golangci.yml` |
| `go test -race -coverprofile=coverage.out ./...` | Tests with the race detector |
| `go tool cover -func=coverage.out` | Coverage; the total must be 90% or higher |

The tests include a no-network check: they fail if the build gains a network package or if any dial, listen or HTTP code is linked into the binary.

`sh scripts/e2e.sh` builds the binary and runs every command on generated input, checking output and exit codes. It points the config and cache folders at a temporary directory, so your own settings and stored mappings are never touched. CI runs it on Linux, plus macOS and Windows (Git Bash, and `.github/scripts/e2e.ps1` under Windows PowerShell 5.1) when the workflow is started by hand.

`sh scripts/render-gifs.sh` re-records the GIFs in `docs/gifs/` from the VHS tapes in `tapes/` (all six, or name some: `sh scripts/render-gifs.sh hero sql`). It needs [VHS](https://github.com/charmbracelet/vhs), `ttyd` and `ffmpeg`, runs in WSL or Linux, and records each tape in a fresh copy of `examples/` with its own config and cache. The clipboard tape renders only in WSL, and puts your clipboard back afterwards. Re-render after any change to cuddio's output or the examples.

In WSL, `sh scripts/test-windows.sh` runs every package's tests as native Windows programs: each test binary is cross-compiled for Windows and started through WSL's Windows interop, so Windows behaviour can be checked without CI.

## Adding or changing a rule

Built-in rules live in `internal/rules/`: secrets in `secrets.go`, PII in `pii.go`. A rule has an ID (lowercase kebab case), a category (`secret` or `pii`), a label for its placeholders (upper case, so `<LABEL_1>`), an RE2 pattern, and optionally:

- `Keywords`: lowercase text that must appear before the pattern runs. Each must be part of every possible match, or the rule silently misses some
- a `(?P<value>...)` group, to hide only part of the match (`DB_PASSWORD=<SECRET_1>` keeps the key readable)
- `Validate`, such as the Luhn check for cards
- `Optional: true`, for noisy rules that are off unless enabled

Steps:

1. Add the rule to `secrets.go` or `pii.go`, with a comment on what it matches and why its edges are where they are.
2. Add cases to `secretCases` or `piiCases` in the matching `_test.go`: at least one match and one near miss that must pass unchanged. `TestEveryRuleHasCases` enforces both, and `TestRulesFireOnTheirOwn` checks each match is caught by this rule, not a neighbour.
3. Build token-shaped values with the `fake(n)` helper rather than writing them out, so nothing committed looks like a live key to GitHub push protection or a user's secret scanner.
4. Run `go test ./internal/textscan -run TestGolden -update` and `go test ./internal/sqlscan -run TestGolden -update`, then review the golden file diff: it shows exactly what the rule changed in realistic logs and dumps.
5. Add a row to the README's rule table, and a line under `## [Unreleased]` in `CHANGELOG.md`. `cuddio rules` lists the new rule on its own.

The vendored gitleaks rules are generated: never edit `internal/rules/gitleaks_gen.go`. `sh scripts/update-gitleaks-rules.sh` regenerates it from the pinned upstream release. Default sensitive SQL columns are in `internal/sqlscan/columns.go`.

## Test data

Every value in fixtures, examples and issues must be obviously fake: `example.com` or `example.invalid` addresses, Ofcom drama-range phone numbers (07700 900xxx), documentation IP ranges (192.0.2.x, 198.51.100.x, 203.0.113.x, 2001:db8::), published test card numbers and vendor documentation keys. Fixtures write anything shaped like a vendor key as a template (`{{fake 24}}`) that the test expands. Large inputs are generated in code, never committed: the module zip has a 1 MB budget.

## Git hooks

- `commit-msg` - messages must be entirely lowercase [Conventional Commits](https://www.conventionalcommits.org/) without a scope, e.g. `feat: add stripe key rule`. `chore(deps): ...` is rejected
- `pre-commit` - checks formatting and runs `govulncheck`

## Changelog

Every user-facing change adds a line under `## [Unreleased]` in `CHANGELOG.md`, in the same pull request, using the [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) headings (`Added`, `Changed`, `Fixed`, `Removed`, `Deprecated`, `Security`). Internal refactors and test-only changes do not need an entry.

## Pull requests

- Keep each pull request to one change
- Include tests; coverage must stay at or above 90%
- `sh scripts/check.sh` must pass
