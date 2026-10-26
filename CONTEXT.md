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
| `internal/cli/` | Cobra commands; `Env` carries args, stdin, stdout, stderr, build info and rules so commands run in-process in tests; root command cleans stdin or `FILE` to stdout or `-o` |
| `internal/detect/` | Rule type, registry with enable/disable selection, Luhn and entropy validators, allowlist (built-in values plus config values and patterns), `Detector` turning rules into `replace.Match` spans |
| `internal/rules/` | `Builtin()` rule set passed in by `main`: `secrets.go`, `pii.go`; vendored gitleaks rules (planned) |
| `internal/textscan/` | Streaming line scanner for plain text and logs; buffers PEM blocks, keeps line endings, caps line length |
| `internal/sqlscan/` | Streaming SQL tokeniser, dialects, column tracking (planned) |
| `internal/replace/` | Placeholder tokens consistent within a run, overlap resolution; fake values (planned) |
| `internal/mapstore/` | Local mapping store, `0600` files with TTL (planned) |
| `internal/clipboard/` | Clipboard access including WSL (planned) |
| `internal/config/` | Config schema, defaults, project file discovery, strict YAML decoding, per-file validation, merging, `CustomRules` compilation |
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
| Config: defaults, then global `cuddio/config.yaml` in `os.UserConfigDir()`, then the nearest `.cuddio.yaml` (walking up, stopping at `.git`), or `--config` instead of the project file | Brief's project-over-global rule; stopping at `.git` keeps a parent directory's file from leaking into another repository |
| Merge: scalars from the later file win, lists combine, custom rules replace by `id`, SQL table columns combine per table; a name the later file enables or disables leaves the earlier file's opposite list | Global allowlists and rules keep applying inside projects; a project can still change modes and formats and switch rules back on |
| Each file validated as it loads (mode, placeholder format, TTL, allowlist patterns, custom rules); rule names in `enable`/`disable` checked at selection | Errors name the file at fault; a toggle may name a custom rule from the other file |
| Custom rules need `id`, `category`, `label`, `pattern`; on by default; an `id` matching a built-in replaces it | Lets users retune a noisy built-in without a new mechanism; `disable` still switches custom rules off |
| `mode: fake` errors until fake values exist | Same reasoning as `--format sql` |
| `mapping.disabled` can be set by either file and unset by neither | A global "never store mappings" policy cannot be undone by a project file |
| Strict YAML decoding; type errors rewritten to drop quoted values | Typos fail loudly; yaml's own messages quote the start of the bad value, which could be an allowlisted secret |
| `go.yaml.in/yaml/v3` | Maintained successor of the archived `gopkg.in/yaml.v3`; v4 is still a release candidate |
| Rules sorted by category (secret, then pii), then ID | Output never depends on registration order; when two rules match the same value, the secret label wins |
| Enable/disable: category entries first, ID entries override, disable wins at the same level; unknown names are errors | `disable: [pii]` plus `enable: [email]` reads naturally; a typo never silently leaves a rule on |
| Rule IDs lowercase kebab case, labels uppercase, IDs never equal a category name | IDs match the gitleaks style; labels fit `<{label}_{n}>`; config names stay unambiguous |
| Built-in allowlist is exact, case-sensitive values only (reserved domains, `test@example.com`, loopback, AWS and Stripe documentation keys) | A near miss is redacted, which is the safe failure; fixtures use `alice@example.com` and still get redacted |
| Allowlist patterns match anywhere unless anchored; invalid patterns report the RE2 error code only | Matches Go's `regexp` behaviour; `regexp` errors quote the pattern, which may hold the secret being allowlisted |
| Same value, same token for the whole run, keyed by value alone; each label numbered by first appearance | The AI can tell values apart across a file; one token per value keeps restore unambiguous even when two rules match it |
| Overlaps: longest span wins, then lower rule priority, then earlier start | A whole DSN or private key beats a fragment inside it; ties stay deterministic |
| Placeholder format must contain `{label}` and `{n}` | Without both, distinct values could share a token and restore would be lossy |
| Text is redacted line by line; a PEM block (`-----BEGIN` to `-----END`) is one chunk, capped at 64 KiB | Bounded memory on any input size; private keys span lines and must reach a rule whole |
| Line ending split off before redaction and written back as read | Output keeps LF, CRLF and a missing final newline byte for byte; rules never swallow the `\r` |
| A line over 1 MiB is an error, not split | A split could cut a secret in two and let both halves through; failing is the safe outcome |
| `-o` writes a temporary file (mode `0600`) in the target directory, then renames it | No partial output on error; the input is never truncated, even through a link; `os.SameFile` refuses the input itself, including redirected stdin, even with `--force` |
| No `FILE` and stdin is a terminal: show help | Waiting for typed input looks like a hang; a pipe or redirected file is read as normal |
| Built-in rules reach the CLI through `Env.Rules`, set in `main` | Tests run the full pipeline with small rule sets they define |
| A rule's `(?P<value>...)` groups (Go allows repeated names) mark the part to redact; the first that took part wins, otherwise the whole match | Keys, schemes and quotes stay readable (`DB_PASSWORD=<SECRET_1>`); validators and the allowlist see only the value |
| Assignment rules skip empty values, `null`/booleans, `${VAR}`/`$VAR`/`{{ }}`/`%VAR%` references and `<LABEL_N>` tokens | Names of secrets are not secrets; re-cleaning cleaned output is a no-op |
| `.env` rule matches commented-out lines; key names matched case-insensitively at line start | An old key is still a key; anchoring to line start keeps prose like `echo PASSWORD=x` out |
| `high-entropy-assignment`: value of 32+ characters starting with a letter, digit or `+`, mixed case and digits, entropy 4.3 bits per byte or more | Hex (git hashes, UUIDs) peaks at 4 bits; URLs and paths never qualify; public random values such as SRI hashes are an accepted false positive |
| Private key rule matches to the end of the chunk when `-----END` is missing | A truncated key in a log is still hidden |
| Public IPs on by default; `private-ip-address` (RFC 1918, unique local, link-local, CGNAT) and `uk-postcode` optional | Brief: private ranges optional, postcodes noisy. Loopback and unspecified are never redacted; documentation ranges are, so examples behave like real data |
| IP and phone patterns find candidates; `net/netip` and digit counts confirm them | Regex alone cannot validate octets or IPv6 compression; `net/netip` is pure parsing and passes the no-network test |
| IPs not preceded by a word character, `.` or a single `/`; IPv6 candidates need a digit | Skips user agent versions (`Chrome/130.0.0.0`) and PHP static calls (`Feed::add`) while keeping `http://203.0.113.9/` |
| Emails never follow `:` (except `mailto:`); `git@` remotes and file extension TLDs (`logo@2x.png`) rejected | `user:password@host` stays a password and a host, not an email |
| Cards need a Visa, Mastercard, Amex or Discover prefix plus Luhn | Luhn alone passes one in ten random numbers, including millisecond timestamps |
| `home-directory` hides only the username; Windows names may contain spaces; `Shared`, `Public`, `Default` skipped | Paths stay readable; `C:\Users\Jane Smith\` does not leak a surname |
| Rule fixtures built at test time from a deterministic `fake(n)` helper | No live-format token in the repository for push protection or users' module caches to flag |
| `.sql` input or `--format sql` errors until the SQL scanner exists | Treating a dump as plain text would change behaviour silently once SQL support lands |

## Dependencies and constraints

| Module | Version | Why |
|---|---|---|
| Go | `go 1.26.0` floor; 1.27.3 for development and releases | Latest two supported releases |
| `github.com/spf13/cobra` | v1.10.2 | Command tree, flags, help |
| `github.com/spf13/pflag` | v1.0.9 (indirect) | Cobra's flags; the only allowed importer of `net` |
| `github.com/inconshreveable/mousetrap` | v1.1.0 (indirect) | Cobra's Windows Explorer launch detection |
| `go.yaml.in/yaml/v3` | v3.0.5 | Config file decoding (MIT and Apache-2.0) |

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
