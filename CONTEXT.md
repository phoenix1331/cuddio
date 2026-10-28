# Context

**cuddio** (Welsh, "to hide") is a local-only Go CLI that hides secrets and PII in logs, `.env` files and MySQL, PostgreSQL and SQLite dumps before they are shared with AI tools. It replaces values with reversible placeholders or format-preserving fakes, understands SQL columns, works on the clipboard, and can fail CI when something would be redacted. It never makes a network connection.

Module `github.com/phoenix1331/cuddio`, binary `cuddio`, config `.cuddio.yaml`.

## Family

First of a planned set of Welsh-named Go tools sharing one visual style for README GIFs and GitHub Pages sites. No sibling exists yet.

## Architecture

| Path | Role |
|---|---|
| `main.go` | `main` package at the root so `go install github.com/phoenix1331/cuddio@latest` works; wiring only, holds the ldflags `version` |
| `network_test.go` | No-network checks (build imports, linked binary symbols) and the test-only package guard |
| `internal/cli/` | Cobra commands; `Env` carries args, stdin, stdout, stderr, build info, rules, the cache directory, a clock and a clipboard so commands run in-process in tests; root command cleans stdin, `FILE` or the clipboard (`--clip`) as text or SQL (`--format`, `--dialect`, `--mode`, `--map`) to stdout, `-o` or the clipboard, or counts (`--check`, `--report`); `restore`, `forget` and `rules` |
| `internal/detect/` | Rule type, registry with enable/disable selection, Luhn and entropy validators, allowlist (built-in values plus config values and patterns), `Detector` turning rules into `replace.Match` spans with their rule ID, `Redactor` (pattern redaction and whole-value tokens over one `Tokens`, with `Counts`) |
| `internal/rules/` | `Builtin()` (`secrets.go`, `pii.go`), `Gitleaks()` (`gitleaks.go` over generated `gitleaks_gen.go`), and `All()`, the set `main` passes in |
| `tools/gitleaksgen/` | Nested module: turns gitleaks' `config/gitleaks.toml` into `internal/rules/gitleaks_gen.go`; run by `scripts/update-gitleaks-rules.sh`, tested by `scripts/check.sh` |
| `internal/textscan/` | Streaming line scanner for plain text and logs; buffers PEM blocks, keeps line endings, caps line length; golden fixtures in `testdata/text/`, fuzz corpus in `testdata/fuzz/` |
| `internal/sqltest/` | Test support only: Laravel-shaped fixture dumps (`testdata/sql/`, embedded), `WriteDump` for large generated dumps, `Check` for MySQL, PostgreSQL and SQLite validity, `OpenSQLite` |
| `internal/sqlscan/` | Streaming tokeniser for MySQL, PostgreSQL and SQLite (`NewMySQL`, `NewPostgres`, `NewSQLite`, `Token`, `Value`); `Clean` and per-dialect `CleanX` over one loop; statement and column tracking (`statement.go`); `Columns` (sensitive column names and labels); `Detect` and `ParseDialect` (`detect.go`); golden output in `testdata/`, fuzz regressions in `testdata/fuzz/` |
| `internal/replace/` | `Placeholders` (with `Seed`, `Reserve`, `Ambiguous`) and `Fakes`, consistent within a run; `Apply` and overlap resolution; `Restorer` |
| `internal/mapstore/` | Session mapping for restore: `mapping.json` in `os.UserCacheDir()/cuddio`, `Load`, `Save`, `Forget`, TTL on an injected clock |
| `internal/clipboard/` | `Clipboard` interface and `System` on `os/exec`: tools chosen from an injected `Host` (GOOS, `/proc/version`, environment, `LookPath`), run by an injected `Runner`; design in `.claude/docs/clipboard-spike.md` |
| `internal/names/` | Nearest known name within two edits, for "did you mean" hints that never echo anything further away |
| `internal/config/` | Config schema, defaults, project file discovery, strict YAML decoding, per-file validation, merging, `CustomRules` compilation |
| `scripts/check.sh` | The quality gate, run locally and in CI |
| `scripts/e2e.sh` | End-to-end run of the built binary, locally and in CI's `e2e` job |
| `scripts/test-windows.sh` | In WSL: every package's tests cross-compiled for Windows and run natively through interop |
| `examples/` | Fake files for users to try, one command each in `examples/README.md`; `examples_test.go` at the root cleans every one with the real rules |
| `tapes/` | VHS tapes: `settings.tape` (shared look, `Set` only), `setup.tape` (hidden `cd` and prompt), and `hero`, `sql`, `restore`, `clipboard`, `check`, `config` |
| `docs/` | GitHub Pages site (`index.html`, one self-contained page) and `docs/gifs/`; a separate module (`docs/go.mod`) so it stays out of cuddio's module zip |
| `scripts/render-gifs.sh` | Builds cuddio, records each tape in a fresh copy of `examples/` with temp config and cache, into `docs/gifs/` |
| `.github/scripts/` | `check-archives.sh` (release archive allow list), `check-module-size.sh` (module zip budget), `e2e.ps1` (Windows PowerShell 5.1 checks) |
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
| `--mode` flag over config `mode`; fake mode ignores the placeholder format | One switch per run; the format only shapes placeholders |
| `detect.Tokens` (`Token`, `IsToken`, `Fit`) lets one `Redactor` hand out placeholders or fakes | Pattern and column redaction share one value-keyed store whichever mode runs |
| Fake generator chosen by label kind, matching `KIND` or `*_KIND` (`BILLING_NAME` is a name); anything else is scrambled: same length, case and digits kept, other characters kept, letters from an xorshift seeded by label and counter | A fake never derives from the value it hides; unknown labels (keys, IDs, payloads) still fit their column and unique keys |
| Fakes: `userN@example.invalid`; Ofcom drama ranges (07700 900xxx, 01632 960xxx, 020 7946 0xxx, 0xxx 496 0xxx) in the original's layout and `+44`/`0044` form; RFC 5737 and 3849 IPs; published test cards matched on digit count; `QQ` NI numbers; `ZZ99` postcodes; dates 1950 to 2004 in `YYYY-MM-DD` or `DD/MM/YYYY`; 20 first names x 20 surnames; `REDACTED` passwords and private keys | Brief's examples; every range is reserved for fiction or testing, so a fake is never someone's real data |
| Generators wrap after their range (15,000 phones, 762 IPv4s, 400 names, a handful of cards); a fake equal to its value is skipped for the next | Bounded tables, deterministic output; past the range, distinct values may share a fake, which only weakens joins. Found by the golden test: the fixture's card is the first test card |
| `CREATE TABLE` `char`/`varchar`/`character varying` lengths are recorded; `Fakes.Fit` cuts a whole-value fake to that many characters; placeholders are never cut | Strict-mode MySQL and PostgreSQL reject over-long values; restore needs placeholders whole |
| Fake mode cannot be restored | One fake (`REDACTED`) can stand for many values; `restore` works on placeholders |
| One session file, `mapping.json`, replaced whole on each save | A later `--map` run can carry on the same numbering, so a log and a dump pasted into one chat never share a token |
| TTL counts from the last save, stored in the file (`saved_at`), not the file's mtime; a mapping saved "in the future" stays live until its TTL passes | Tests drive expiry with an injected clock; a file's mtime can be changed by copying or backup tools |
| Every `Load` and `Save` purges first: expired or corrupt `mapping.json`, and `.mapping-*` temp files over a minute old | Temp files from a crashed write hold real values; a minute leaves a concurrent run's write alone |
| Corrupt file: deleted, `ErrCorrupt` with a fixed message, never the JSON error | `encoding/json` errors quote bytes of the file, which are real values |
| Directory `0700` (narrowed if it already exists), file `0600` via `os.CreateTemp` then rename; Windows relies on the per-user ACL of `%LocalAppData%` | Go sets only the read-only bit on Windows; the tests assert modes on Unix and skip with that reason on Windows |
| `mapping.disabled` makes `Save` return `ErrDisabled`; `Load` and `Forget` still work | A mapping saved before the policy changed can still be restored or deleted |
| Not encrypted | Brief puts encryption out of scope for v0.1; owner-only permissions in the user's own cache directory, and a 1 hour default TTL |
| Clipboard: own `internal/clipboard` on `os/exec`, no library | `atotto/clipboard` writes raw UTF-8 to `clip.exe`, leaving mojibake on the Windows clipboard; `golang.design/x/clipboard` imports `net` on Linux. Detail in `.claude/docs/clipboard-spike.md` |
| WSL and Windows use PowerShell `Set-Clipboard` with UTF-8 stdin and `Get-Clipboard -Raw` with UTF-8 output; macOS `pbcopy`/`pbpaste` with `LANG=en_US.UTF-8`; Linux `wl-copy`, `xclip`, `xsel` | The only WSL path that round-trips bytes exactly (verified to 8 MiB); clipboard text is always on stdin, never in arguments |
| Tool choice: Windows, or WSL (`/proc/version` contains `microsoft`) with `powershell.exe` on `PATH`; macOS; Wayland with both `wl-copy` and `wl-paste`; `xclip`; `xsel`; else an error on first use naming what to install (and Windows interop in WSL) | WSL goes straight to the Windows clipboard rather than WSLg's bridge; commands that never touch the clipboard never see the error |
| `restore --clip` writes a `DataObject` marked `ExcludeClipboardContentFromMonitorProcessing`, `CanIncludeInClipboardHistory = 0` and `CanUploadToCloudClipboard = 0` on Windows and WSL; plain write elsewhere | Restored text holds real values; Windows clipboard history and cloud sync then skip it (verified formats present in WSL) |
| Writes discard tool output; reads capture it; a tool's stderr is never shown, only its name and exit status; known "clipboard empty" messages from `xclip` and `wl-paste` read as empty; 10 s timeout | `xclip` and `wl-copy` linger to serve the selection; stderr can echo clipboard text |
| `--clip`: empty clipboard is an error; unchanged text is not written back; a one-line note on stderr says what happened; with `-o` the result goes to the file and the clipboard is left alone; `FILE` with `--clip` is an error | Nothing else is printed, so the note is the only feedback; skipping no-op writes keeps clipboard history clean |
| Real-clipboard test opt-in with `CUDDIO_CLIPBOARD_TEST=1`, saving and restoring the clipboard; `Exec` tested by re-running the test binary as a stand-in tool | Tests must not clobber a developer's clipboard; CI runners' clipboards are unreliable |
| `--check` and `--report` count replacements that reach the output (after overlap resolution), by rule ID, with whole sensitive-column values as `column:<label>`; occurrences, not distinct values | Matches what cleaning would change; a column label is a column name, never a value |
| Counts go to stdout (`--report` as a sorted `tabwriter` table with a `total` row); `--check` exits 1 through a sentinel error that `Execute` maps without printing; read and setup errors stay 2 | CI can tell "would redact" from "broke"; sorted IDs keep output deterministic |
| `--check`/`--report` with `-o` or `--map` is an error; with `--clip` the clipboard is read, never written | Counting writes nothing, so those flags would silently do nothing |
| `cuddio rules` columns: ID, CATEGORY, LABEL, DEFAULT (no config), ENABLED (after the config files found), SOURCE (built-in, gitleaks, custom); `--category` filters | One command answers both "what exists" and "what is on here" |
| Text memory test samples live heap after forced GCs, allowing the 1 MiB line buffer plus 1 MiB | `HeapSys` flaked (one run in three under load, also on the earlier code); same method as the SQL memory tests |
| Input starting with a UTF-16 (or UTF-32) byte order mark is refused with conversion instructions, before any output is created; a UTF-8 BOM is fine | Every character is separated by NUL bytes, so no rule matches and secrets pass through: found by testing, exit 0 with the email intact. PowerShell 5.1 writes UTF-16 with `>` and `Out-File` |
| Windows PowerShell 5.1 guidance in `--help`: name the file and use `-o`, not `|` or `>` | 5.1 re-encodes piped text with `$OutputEncoding` (ASCII by default) and writes `>` as UTF-16 |
| `.gitattributes` marks `**/testdata/** -text` | `testdata/**` only matched a root `testdata/`, so `internal/*/testdata` fixtures were `text=auto` and a CR byte in one would have been normalised away |
| `scripts/e2e.sh` (POSIX, Git Bash on Windows) builds the binary and runs every command with exit codes and output checked, with `HOME`, XDG, `APPDATA` and `LOCALAPPDATA` in a temp dir; `.github/scripts/e2e.ps1` adds Windows PowerShell 5.1 checks; CI job `e2e` on all three OSes | Proves the shipped binary, not just packages, on each platform; never touches a developer's own config or mappings |
| Both workflows have `workflow_dispatch`; `test` and `e2e` run Linux only on push and pull request, all three OSes only when started by hand | The repository is private until release, where macOS minutes cost 10x and Windows 2x; one month's allowance went in a day. Revisit once public, when standard runners are free |
| Windows checked locally with `scripts/test-windows.sh` (no `-race`: cross-compiling it needs cgo; the three no-network tests skipped: they need the `go` command); macOS on a family Mac | Real Windows APIs and file system without CI minutes or changing PowerShell's execution policy |
| `Execute` always passes cobra a non-nil argument list | Given nil, cobra reads `os.Args`; found when a Windows test binary's `-test.skip` pattern became cuddio's input file |
| `docs/` is a nested module (`docs/go.mod`, no Go code) | The six GIFs (about 700 KB) put the module zip at 1.4 MB, over the 1 MB budget; Go leaves nested modules out of the parent's zip (`.claude/docs/module-zip-spike.md`), so `go install` never downloads images |
| GIFs: Catppuccin Mocha, DejaVu Sans Mono 16, 1280 wide, height per tape; recorded with VHS in WSL, never the Docker image | One look for the family of Welsh-named tools; DejaVu is widely installed and covers Welsh characters such as ŷ |
| `settings.tape` holds only `Set`; each tape sources it, sets its height, then sources `setup.tape` | VHS ignores `Set` after the first action, so per-tape heights were silently dropped when the shared tape also held the hidden setup |
| The clipboard tape uses hidden `copy` and `paste` helpers on the Windows clipboard and renders only in WSL; `render-gifs.sh` saves and restores the clipboard | Shows the real `--clip` flow without leaving the demo text on the developer's clipboard |
| The postcode field was taken out of `examples/laravel.log` | `uk-postcode` is off by default, so a visible postcode in the hero GIF read as a leak |
| Issue forms: bug, false positive, false negative, new rule; blank issues off; security reports linked to private advisories | Every form warns against real values; the false-negative form asks for a made-up value in the same shape and a confirmation checkbox, since a real key in a public issue is a leak |
| `SECURITY.md` lists what cuddio touches, detects and does not detect, and what counts as a vulnerability (a value printed, stored or sent) versus a false negative (public issue, fake data) | One place answers "is this safe to run" and "where do I report this" |
| Site: one `docs/index.html`, inline CSS and JS, Google Fonts (Inter, JetBrains Mono) only; colours as CSS custom properties with a `prefers-color-scheme` dark theme; slate, Welsh green and dragon red | The family style for future Welsh-named tools: change the tokens, keep the components |
| The site's rule tables are generated from `cuddio rules` output when the page is built, and its config example was loaded by cuddio to check it | The page cannot list a rule the binary lacks or show config that does not load; regenerate after rule changes |
| Logo: `docs/cuddio-logo-dark.png` and `-white.png` are the originals; the site header uses `-header.png` copies (108 px tall, 25 to 40 KB) through `<picture>`, white for `prefers-color-scheme: dark`; regenerate them when the originals change | The originals are large (1510x440, up to 283 KB), with faint background specks (alpha under 32) that showed on dark backgrounds; the header copies drop those and are sized for 36 px at 3x |
| The product is **Cuddio** in prose (docs, site, titles, image descriptions); `cuddio` wherever it is typed or a name in code: commands, the binary, module path, URLs, `.cuddio.yaml`, `cuddio/config.yaml`, the `cuddio:` error prefix | Matches the logo, as Git is to `git`; what people type stays exact |
| The clipboard helper process runs `os.Executable()`, not `os.Args[0]` | `os.Args[0]` can be a bare name, which Windows `exec` will not look up in the current directory |
| Dependabot ignores `github.com/pingcap/tidb/pkg/parser`; bump its pseudo-version by hand (`/1331-compat`) | It has no semver tags, so Dependabot picks the parent repo's `v2.1.0-rc.5+incompatible`, which lacks the package, and the grouped Go update failed every week |
| Mapping store: a file where its folder should be is an error ("the cache path is a file, not a folder") on every platform | Found by Windows CI: listing a file there reports "not found", so the store read as empty and `--map` wrote output before failing to save |
| Example secrets are generic shapes (bearer tokens, cookies, DSN and `.env` passwords, `APP_KEY`, secret JSON fields, token query parameters) plus AWS's documentation keys, never vendor live formats; SQL examples are the fixtures expanded once, with `sk_live_` swapped for a generic value and emails moved to `example.com` | GitHub push protection rejects live-format provider keys in static files; the AWS keys show the allowlist leaving documented examples alone |
| `examples_test.go` lists each example's fake values; the test fails if one is missing from the file or survives cleaning, if a SQL example is no longer valid, or if `ai-answer.md` uses a placeholder `laravel.log` does not produce (other than `<EMAIL_9>`) | Examples cannot drift from what cuddio does; runs with an empty working directory and its own cache so local config cannot change the result |
| `e2e.ps1` keeps to ASCII, building non-ASCII text from code points, and uses `ErrorActionPreference = 'Continue'` with `$LASTEXITCODE` checks | PowerShell 5.1 reads a BOM-less script as Windows-1252, and turns redirected native stderr into errors that `Stop` would throw |
| `--map` seeds placeholders from the live mapping and saves the merged mapping after the output is written; a failed run stores nothing | Numbering carries on across runs within the TTL, so files cleaned for one chat never share a token |
| Placeholder-shaped text in the input is reserved as it streams past (`Reserve` on every value and line), and new tokens skip it; if a token cuddio already handed out (this run or seeded) shows up in the input, the run is ambiguous: output is written, the mapping is not stored, exit 2 | Streaming cannot look ahead, so reserving covers the common case (a template mentioning `<EMAIL_1>` first) and refusing covers the rest; restore must never swap the input's own text for a real value |
| A sensitive column value goes only to `Whole`, never also to `Redact` | Redact on a value about to be replaced whole would hand out tokens that never reach the output |
| `restore` reuses the text and SQL cleaners with a redactor that swaps tokens back (`strings.Replacer`, longest token first); SQL is re-escaped per dialect | Raw substitution would break quoting (`'<NAME_1>'` to `'O'Brien'`); round trips of the MySQL, PostgreSQL and SQLite fixtures come back byte for byte, `pg_dump --inserts` value for value (`E''` and dollar-quoted strings return as standard strings) |
| `restore` with no live mapping is an error naming the TTL; unknown tokens are left; `forget` deletes everything | The usual cause is expiry; an AI inventing `<EMAIL_9>` should not break the rest of its answer |
| `Env.CacheDir` from `mapstore.DefaultDir()` in `main`; empty makes `--map`, `restore` and `forget` fail with one message | Tests point it at a temp dir; no command writes outside it |
| `mapping.disabled` can be set by either file and unset by neither | A global "never store mappings" policy cannot be undone by a project file |
| Strict YAML decoding; type errors rewritten to drop quoted values | Typos fail loudly; yaml's own messages quote the start of the bad value, which could be an allowlisted secret |
| Errors repeat a name from config only as a near miss (two edits) of a real setting, rule or category; otherwise they give a line number or position. Quoted keys, anchors and non-core tags are dropped | A value typed in the wrong place (a secret as a key, in `enable`, as a rule ID) is never echoed; a shape check alone would pass lowercase hex keys |
| Security tests in `internal/cli/security_test.go` run every clean path with known values, assert none appear in stdout, stderr or output, and compare the input's SHA-256, mode and mtime before and after | One place proves "never print a secret" and "never touch the input"; `assertNoLeak` is reused by `--check` and `--report` |
| `go.yaml.in/yaml/v3` | Maintained successor of the archived `gopkg.in/yaml.v3`; v4 is still a release candidate |
| Rules sorted by category (secret, then pii), then source (vendored last), then ID | Output never depends on registration order; when two rules match the same span, the secret label wins, then cuddio's own rule over a vendored one |
| gitleaks rules vendored as generated Go, not embedded TOML | No TOML parser in the binary; patterns checked at generation; reviewable diffs. Detail in `.claude/docs/gitleaks-spike.md` |
| Generator rewrites gitleaks' secret capture groups to `(?P<value>...)` in the pattern text, then proves with `regexp/syntax` that only names changed | Reuses the existing value-group mechanism; `syntax.Regexp.String()` would make patterns unreadable |
| Left out of gitleaks: `generic-api-key`, `gcp-api-key`, path-scoped rules; upstream rules sharing a built-in ID dropped by `All()` | `generic-api-key` costs more than all other upstream rules together and is noisy; `gcp-api-key`'s allowlist holds live-format keys; stdin has no path; built-ins are tuned for cuddio |
| Vendored rules keep gitleaks semantics: secret entropy must exceed the threshold, global and rule allowlists (secret regexes, stopwords) apply | Same findings as gitleaks; allowlists needing a path or commit can never apply to cuddio input, so the generator drops them and fails on any other unsupported allowlist |
| Vendored labels are the uppercased ID (`GITLAB_PAT`); labels may start with a digit | gitleaks has no labels; `1password-*` IDs start with one |
| `gitleaks` is a toggle name like a category: categories apply first, then rule sets, then IDs | `disable: [gitleaks]` turns the vendored set off while `enable: [gitlab-pat]` keeps one |
| Enable/disable: category entries first, then rule sets, then IDs override, disable wins at the same level; unknown names are errors | `disable: [pii]` plus `enable: [email]` reads naturally; a typo never silently leaves a rule on |
| Rule IDs lowercase kebab case, labels uppercase, IDs never equal a category or rule set name | IDs match the gitleaks style; labels fit `<{label}_{n}>`; config names stay unambiguous |
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
| Rules carry lowercase `Keywords`; a rule's regexp runs only when one appears in the chunk (lowercased once) | Prefilter like gitleaks' keywords; 2.4x faster on logs; each keyword must be a necessary condition of the pattern, which the "fires on its own" test checks against every positive case |
| IP pattern consumes no trailing context (IPv4 ends at `\b`) | Found by fuzzing: a consumed trailing separator hid the next address in `a.b.c.d e.f.g.h`. Cost: `1.2.3.4.5` becomes `<IP_1>.5`, an accepted over-redaction since RE2 has no lookahead |
| Golden inputs are templates (`{{fake N}}`, `{{private-key}}`) expanded at test time; CRLF variants generated from LF files | No live-format token committed; outputs contain only placeholders; one fixture per format |
| Fuzz property: identity clean round-trips; a rule match in cleaned output fails unless it touches a placeholder | Strict idempotence is too strong: a token changes its neighbours' context (`0::2` becomes `<IP_1>::2`) without anything surviving |
| Memory test: warm-up, then 3 MiB streamed with at most 1 MiB heap growth | Heap is flat after warm-up (about 200 KiB at 4 and 16 MiB); small enough for `-race` in CI |
| Placeholder state grows with distinct values | Same value, same token for the whole run needs a map; bounded memory holds for bounded distinct values |
| Rule fixtures built at test time from a deterministic `fake(n)` helper | No live-format token in the repository for push protection or users' module caches to flag |
| SQL validity: TiDB parser (MySQL), go-pgquery (PostgreSQL, libpg_query as Wasm), modernc.org/sqlite (import); test-only, in the main module | All pure Go, so tests run on Windows CI with `CGO_ENABLED=0`; detail in `.claude/docs/sql-validity-spike.md` |
| `internal/sqltest` is a normal package imported only by tests; `TestBuildExcludesTestOnlyPackages` fails if it or its libraries appear in `go list -deps .` | Shared by `sqlscan` and CLI tests; the guard catches a stray import before it bloats the binary |
| `Check` also compares each row's value count with its table's columns (from `CREATE TABLE`, `INSERT` and `COPY` column lists) | Parsers check syntax only; a dropped or doubled value would otherwise pass |
| `Check` skips mysql `DELIMITER ;;` blocks and psql meta-commands (`\restrict`, `\unrestrict`); COPY data rows are checked in Go (field count on unescaped tabs, no trailing lone backslash) | None of these are SQL; TiDB cannot parse triggers or routines; blocks hold code, not row data |
| SQL fixtures use `{{fake N S}}` and `{{hex N S}}` with a seed; a test runs every secret rule over the committed templates | Distinct deterministic tokens per row; nothing committed looks live to push protection |
| `golang.org/x/text` pinned at v0.41.0 | TiDB's parser pulls in v0.39.0, affected by GO-2026-6629 (not called, but the module scan flags it) |
| SQL tokens keep their exact source `Text`; joining them gives the input back | Everything cuddio does not change passes through byte for byte; the fuzz test checks it |
| SQL token cap of 16 MiB (`DefaultMaxToken`); a longer value is an error, never split | Same reasoning as the text line cap; LONGTEXT session payloads fit |
| MySQL executable comments (`/*!40101 ... */`) are tokenised inside: `DirectiveStart`, normal tokens, `DirectiveEnd` | MySQL runs their contents; strings inside (`/*!80000 '+'*/`) must reach the same rules as any other |
| Charset introducers (`_binary`, `_utf8mb4`) are recorded on the next literal (`Token.Charset`); `N'...'` gives `N` | Lets column handling tell binary values from text without a second pass |
| Unterminated strings, identifiers and comments at end of input are tokens marked `Unterminated`, not errors | A truncated dump still streams; the caller decides, and redacting the open value is the safe choice |
| SQL memory test forces a GC every 512 KiB and checks peak live heap (54 KiB growth over 10 MiB) | Total heap size mostly measures garbage between collections, which flaked at Go's 4 MiB minimum GC target |
| Dialect detected from the first 64 KiB (`DetectSize`, buffered, not consumed): the dialect whose dump-tool marker (`-- MySQL dump`, `/*!40`, backticks; `-- PostgreSQL database dump`, `\restrict`, ` FROM stdin;`; `PRAGMA foreign_keys=OFF;`, `sqlite_sequence`) comes first | Header markers sit at the top, so a value further down holding another dialect's marker cannot change the answer |
| No marker: an error asking for `--dialect`, never a guess; empty input is fine | A wrong dialect misreads backslashes and `COPY` rows and could leak; failing is the safe outcome |
| `--dialect` implies `--format sql`, errors with `--format text`, accepts `mariadb`, `postgresql`, `pg`, `sqlite3`; an invalid value is not echoed | No flag combination silently does nothing; nothing typed is repeated in an error |
| SQL mode redacts string values, sensitive-column literals, comments and psql meta-command arguments; unquoted words are passed through | In a dump, data is always quoted or numeric; free text outside strings is not valid SQL, so text mode is the tool for it |
| SQLite tokeniser: backslashes literal, `"name"` and `[name]` identifiers, comments do not nest | Matches SQLite's own lexer and `sqlite3 .dump` output |
| The first argument of `replace(...)` or `unistr(...)` in a row counts as the column's value | `sqlite3 .dump` wraps strings holding line breaks this way; without it, a multi-line address in a sensitive column would only get pattern rules |
| Tokeniser read errors are wrapped `read: ...`, as in text mode | One error shape for both scanners; the security tests assert it |
| SQL statement tracking is a small state machine over tokens, not a parser: `CREATE TABLE` definitions and `INSERT`/`REPLACE` column lists and `VALUES` rows; anything else gets pattern rules only | Bounded memory (only column names per table are kept); a statement cuddio does not understand still has every string value checked |
| Whole-value redaction applies to literals at row depth 1 (strings, numbers, hex, bit); `NULL`, function arguments and `INSERT ... SET`/`SELECT` values get pattern rules only | A nested value is an expression, not the column's value; `NULL` and blanks reveal nothing |
| Default sensitive columns: the brief's list plus `token`, `secret`, `two_factor_recovery_codes`, the person-name columns `full_name`, `billing_name`, `shipping_name`, `customer_name`, `contact_name`, `display_name`, and `sessions.id`/`payload`; every default except `name` also matches as an `_` suffix (`billing_email`, `customer_first_name`); configured names match exactly | Laravel stores live credentials in sessions and token tables; `_name` would catch `product_name` and `file_name`, so common person-name columns are listed instead. `billing_name` was found leaking by the examples test |
| A default column's label is its kind (`billing_email` gives `EMAIL`, longest suffix wins); a configured column's label is its name in upper case | Matches pattern rule labels, so the AI sees one kind of token per kind of value |
| `Whole` skips blank, allowlisted and already-tokenised values; tokens keyed by value across pattern and column redaction | Cleaning cleaned output is a no-op; a value gets one token wherever it appears, so the first table to show it picks its label |
| Changed values are written back with mysqldump's escaping, keeping the quote character and `N` or `_charset` prefix; changed numbers, hex and bit literals become quoted strings | Output stays valid SQL (TiDB-checked in golden and fuzz tests); placeholder mode does not promise column types, fake mode will |
| Comments: only the body goes through pattern rules, never the `--`, `#` or `/* */` markers (nor the space MySQL needs after `--`); a comment a placeholder would break (line break, `/*` or `*/`, a leading `!`) becomes `/**/` | Found by fuzzing: an email match swallowed `--` and turned a comment into SQL. Nothing in a comment leaks, and no placeholder format can turn comment text into SQL |
| PostgreSQL tokeniser assumes `standard_conforming_strings = on` (pg_dump sets it): backslashes literal except in `E''`; `"` delimits names; dollar quotes; nested `/* */`; `--` needs no space; no `#` comments or `/*!` directives | Matches pg_dump since 9.1; older dumps with the setting off would decode `\` differently |
| The tokeniser itself spots `COPY ... FROM STDIN` and returns each following line as `CopyData` up to the `\.` line (`CopyEnd`); the statement tracker separately reads the table and column list | Data lines are not SQL, so tokenising them as SQL would be wrong; the tokeniser has to switch modes, the cleaner has to know columns |
| `COPY` text rows: fields split on unescaped tabs and decoded; `\N` kept; sensitive columns replaced whole; changed fields escaped as pg_dump does; unchanged fields keep their bytes. A `COPY` with options (`WITH (FORMAT csv)`) or no known columns has each line pattern-checked whole, with any line break a placeholder brings turned into a space | Field-exact cleaning for what pg_dump writes; anything else still gets every pattern rule without risking the block's line structure |
| Changed PostgreSQL strings become standard strings (`'...'`, quotes doubled), or `E'...'` when they were `E''`; dollar-quoted values become standard strings; `N` prefixes kept | Always valid with `standard_conforming_strings` on, without choosing a dollar tag that cannot clash |
| psql meta-commands (lines starting `\`): arguments pattern-checked, command kept; `\restrict` and `\unrestrict` untouched | Their key is random per dump and psql rejects anything but letters and digits |
| `INSERT ... OVERRIDING SYSTEM VALUE VALUES`, `ON CONFLICT`, `CREATE UNLOGGED TABLE` handled; `PARTITION OF` and `LIKE` tables have no column list | pg_dump `--inserts` output for identity columns; partitions get pattern rules only |

## Dependencies and constraints

| Module | Version | Why |
|---|---|---|
| Go | `go 1.26.0` floor; 1.27.3 for development and releases | Latest two supported releases |
| `github.com/spf13/cobra` | v1.10.2 | Command tree, flags, help |
| `github.com/spf13/pflag` | v1.0.9 (indirect) | Cobra's flags; the only allowed importer of `net` |
| `github.com/inconshreveable/mousetrap` | v1.1.0 (indirect) | Cobra's Windows Explorer launch detection |
| `go.yaml.in/yaml/v3` | v3.0.5 | Config file decoding (MIT and Apache-2.0) |
| gitleaks rules (vendored, not a module) | v8.30.1, `config/gitleaks.toml` SHA-256 `e163e53b...a329caf` | Upstream rules as generated Go; MIT notice in `LICENSE` and the generated header; pinned in `scripts/update-gitleaks-rules.sh` |
| `github.com/pingcap/tidb/pkg/parser` | `v0.0.0-20261010074516-03148cd4adf3`, tests only | MySQL syntax and column-count checks (Apache-2.0); no semver tags |
| `github.com/wasilibs/go-pgquery` | `v0.0.0-20260915022521-81f99195012b`, tests only | PostgreSQL syntax and column-count checks (MIT); returns `pg_query_go/v6` trees |
| `modernc.org/sqlite` | v1.60.1, tests only | In-memory SQLite import (BSD-3-Clause) |
| `github.com/BurntSushi/toml` | v1.5.0, `tools/gitleaksgen` only | Reads gitleaks' TOML in the generator; never in `go list -deps .` |

Tools (not module dependencies): golangci-lint v2.15, govulncheck, GoReleaser v2, actionlint.

## CI

| Workflow | Runs |
|---|---|
| `test.yml` | `scripts/check.sh` on Go 1.26 and 1.27 and `scripts/e2e.sh`: Linux on push and pull request, plus macOS and Windows (and `e2e.ps1`) when started by hand; module size check, GoReleaser snapshot and `check-archives.sh` |
| `govulncheck.yml` | Every push, every pull request, weekly |
| `.github/dependabot.yml` | Weekly, Go modules and Actions, one grouped PR each |

GoReleaser is snapshot only (`release.disable: true`) until the release workflow, Homebrew tap and Scoop bucket exist.

## Local setup

WSL with Go in `/usr/local/go` and `$HOME/go/bin` on `PATH`. Install golangci-lint, govulncheck and GoReleaser with `go install`, run `git config core.hooksPath .githooks` once, then `sh scripts/check.sh`. No Docker, no Makefile, no database servers: SQL fixtures are plain text.
