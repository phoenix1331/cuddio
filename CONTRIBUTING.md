# Contributing to cuddio

Thanks for helping. Bug reports, fixes and new rules are all welcome.

Never paste real secrets or personal data into an issue or pull request, even redacted. Use obviously fake values (`example.com`, `example.invalid`, documented vendor test keys).

## Setup

Prerequisites: Go 1.26 or later, plus these tools:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
```

```bash
git clone https://github.com/phoenix1331/cuddio
cd cuddio
git config core.hooksPath .githooks   # activates the git hooks
```

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

## Git hooks

- `commit-msg` - messages must be entirely lowercase [Conventional Commits](https://www.conventionalcommits.org/) without a scope, e.g. `feat: add stripe key rule`. `chore(deps): ...` is rejected
- `pre-commit` - checks formatting and runs `govulncheck`

## Changelog

Every user-facing change adds a line under `## [Unreleased]` in `CHANGELOG.md`, in the same pull request, using the [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) headings (`Added`, `Changed`, `Fixed`, `Removed`, `Deprecated`, `Security`). Internal refactors and test-only changes do not need an entry.

## Pull requests

- Keep each pull request to one change
- Include tests; coverage must stay at or above 90%
- `sh scripts/check.sh` must pass
