<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/cuddio-logo-white-header.png">
    <img src="docs/cuddio-logo-dark-header.png" alt="Cuddio" height="72">
  </picture>
</p>

<p align="center"><strong>Hide what matters before you paste.</strong></p>

<p align="center">
  <a href="https://github.com/phoenix1331/cuddio/actions/workflows/test.yml"><img src="https://github.com/phoenix1331/cuddio/actions/workflows/test.yml/badge.svg" alt="tests"></a>
  <a href="https://pkg.go.dev/github.com/phoenix1331/cuddio"><img src="https://pkg.go.dev/badge/github.com/phoenix1331/cuddio.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/phoenix1331/cuddio"><img src="https://goreportcard.com/badge/github.com/phoenix1331/cuddio" alt="Go Report Card"></a>
  <a href="https://github.com/phoenix1331/cuddio/releases"><img src="https://img.shields.io/github/v/release/phoenix1331/cuddio" alt="release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/phoenix1331/cuddio" alt="licence: MIT"></a>
</p>

Cuddio hides **secrets** and **PII** (personally identifiable information) in logs, `.env` files and MySQL, PostgreSQL and SQLite dumps before you share them with AI tools. API keys, passwords, emails, phone numbers, card numbers and IP addresses become placeholders like `<EMAIL_1>`, so the AI can still tell one customer from another, and `cuddio restore` puts the real values back into its answer. It runs locally as a single binary and never makes a network connection.

<p align="center"><img src="docs/gifs/hero.gif" alt="Two Laravel log lines with an email, phone number, card number, IP address and tokens, then the same lines through Cuddio with each value replaced by a placeholder such as EMAIL_1 and CARD_1." width="900"></p>

*Cuddio* is Welsh for *to hide*. Say it roughly "KITH-yo": the Welsh *dd* is the *th* in *this*.

> **Use at your own risk.** Redaction is best effort. Cuddio cannot guarantee that every secret or piece of personal data is caught, so review the output before you share it. The software is provided "as is", without warranty of any kind, under the [MIT licence](LICENSE).

## Why Cuddio

Developers paste logs, stack traces, config and database dumps into AI chats and agents every day. Those often hold API keys and passwords, and customers' emails, phone numbers and names. Sending personal data to a third-party AI service can breach UK GDPR and company policy, even when nobody meant any harm.

Replacing values with `***` hides them, but it also hides the structure the AI needs: whether two log lines mention the same customer, or whether the failing query and the error refer to the same address. Cuddio's placeholders are consistent instead. The same value always becomes the same token, so `<EMAIL_1>` in line 3 and line 40 is visibly the same person, and you can map the answer back.

What sets it apart:

- **Reversible placeholders.** `--map` keeps a short-lived local mapping, and `cuddio restore` swaps the real values back into the AI's reply
- **SQL-aware.** It reads `CREATE TABLE` and `INSERT` columns, hides every value in sensitive columns (any `email` or `password`) even when no pattern matches, and keeps the dump valid SQL. Fake mode writes realistic values so the dump still imports
- **Clipboard mode.** `cuddio --clip` cleans what you just copied, with UTF-8 kept intact on Windows, WSL, macOS and Linux
- **Laravel and PHP aware.** `APP_KEY`, `.env` files, PDO DSN passwords, log context payloads and session tables, without being PHP-only
- **Single binary.** No Python or Node runtime, and the test suite fails if any network code is linked in

Other tools cover parts of this, and one of them may suit you better:

| Tool | Language | Focus |
|---|---|---|
| `redacted` | Go | Claude Code hook that scrubs secrets from tool output |
| `redactcli` | Python | Redacts secrets from agent output, logs, diffs and CI |
| ShareClean | Python | Cleans logs, stack traces and config before sharing |
| [gitleaks](https://github.com/gitleaks/gitleaks) | Go | Scans repositories for secrets; Cuddio's vendored rules come from it |
| **Cuddio** | Go | Secrets and PII in text, logs and SQL dumps, with reversible placeholders, column-aware SQL and the clipboard |

## Installation

Cuddio is a single program with nothing else to install. Homebrew, Scoop and the release download need no other tools; only `go install` needs Go.

**Homebrew** (macOS and Linux):

```bash
brew install phoenix1331/tap/cuddio
```

**Scoop** (Windows):

```powershell
scoop bucket add phoenix1331 https://github.com/phoenix1331/scoop-bucket
scoop install cuddio
```

**Release binary:** download the archive for your platform (Linux, macOS and Windows, amd64 and arm64) from [GitHub Releases](https://github.com/phoenix1331/cuddio/releases) and unpack it. On Linux and macOS, move `cuddio` somewhere on your `PATH`, such as `sudo mv cuddio /usr/local/bin/`. On Windows, put `cuddio.exe` in a folder that is on your `Path`, or add its folder under *Settings, System, About, Advanced system settings, Environment Variables*.

**Go** 1.26 or later, if you have Go or want to build from source:

```bash
go install github.com/phoenix1331/cuddio@latest
```

<details>
<summary>Don't have Go?</summary>

1. Install Go 1.26 or later:
   - **macOS:** `brew install go`, or the installer from [go.dev/dl](https://go.dev/dl/)
   - **Windows:** `winget install GoLang.Go`, or the installer from [go.dev/dl](https://go.dev/dl/)
   - **Linux and WSL:** the archive from [go.dev/dl](https://go.dev/dl/), following [its install steps](https://go.dev/doc/install). Distribution packages such as Ubuntu's `golang-go` are often older than 1.26
2. Check it works in a new terminal: `go version`
3. `go install` puts `cuddio` in Go's own `bin` folder, which may not be on your `PATH` yet:
   - **macOS, Linux and WSL:** add `export PATH="$PATH:$(go env GOPATH)/bin"` to `~/.profile` or `~/.zshrc`, then open a new terminal
   - **Windows:** the Go installer adds `%USERPROFILE%\go\bin` for you; open a new terminal
4. Run `go install github.com/phoenix1331/cuddio@latest`, then `cuddio --version`

</details>

## Usage

```bash
cat storage/logs/laravel.log | cuddio      # clean stdin to stdout
cuddio app.log                             # clean a file to stdout
cuddio app.log -o app.clean.log            # write a new file (never the input)
cuddio --clip                              # clean the clipboard in place
cuddio dump.sql -o dump.clean.sql          # SQL-aware: by extension, --format sql or --dialect
```

The [`examples/`](examples/) folder has fake logs, a leaky `.env`, a stack trace and three SQL dumps to try, with a command for each.

| Flag | What it does |
|---|---|
| `-o FILE` | Write to a new file through a temporary file; refuses the input itself, and an existing file unless `--force` |
| `--mode fake` | Realistic fake values instead of placeholders (`user1@example.invalid`, `07700 900001`, `REDACTED`) |
| `--format sql`, `--dialect mysql\|postgres\|sqlite` | Read SQL; the dialect is otherwise detected from the dump's header |
| `--map` | Keep the placeholder mapping for `restore` |
| `--clip` | Read the clipboard and write the result back to it |
| `--check`, `--report` | Print counts only, for CI (see below) |
| `--config FILE` | Use this config file instead of the project one |

On Windows PowerShell 5.1, give Cuddio the file name and use `-o`: piping with `|` and redirecting with `>` re-encode text.

### SQL dumps

MySQL and MariaDB (`mysqldump`, `mariadb-dump`), PostgreSQL (`pg_dump`, including `COPY ... FROM stdin` blocks) and SQLite (`sqlite3 .dump`) are streamed in bounded memory, so gigabyte dumps are fine. Every value in a sensitive column is hidden whole, pattern rules still run on every string (an email in a `notes` column is caught too), and the output stays valid SQL: quotes, escapes, `NULL`, hex and binary literals and multi-line strings are all handled.

```bash
cuddio dump.sql -o dump.clean.sql           # placeholders
cuddio dump.sql --mode fake -o dump.fake.sql  # realistic values; joins on email still line up
cuddio dump.sql --check                     # exit 1 if anything would be hidden
```

<p align="center"><img src="docs/gifs/sql.gif" alt="Cuddio cleaning a MySQL users row: names, emails and password hashes become placeholders, then realistic fake values with --mode fake, then --check reports 38 values would be hidden." width="900"></p>

Fake values are consistent too: the same email always becomes the same `userN@example.invalid`, and fakes are cut down to fit `char` and `varchar` columns, so the dump imports into a development database.

### Put the real values back

```bash
cuddio app.log --map                 # clean, and keep the mapping for this session
cuddio restore < ai-answer.md        # put the real values back into the AI's reply
cuddio restore --clip                # same, on the clipboard
cuddio forget                        # delete stored mappings now
```

Mappings are stored only when you pass `--map`, in your cache directory with owner-only permissions (`0600`), and expire after an hour by default. Placeholders Cuddio never handed out, such as one the AI made up, are left as they are. Later `--map` runs carry on the numbering, so a log and a dump cleaned for the same chat never share a placeholder.

<p align="center"><img src="docs/gifs/restore.gif" alt="Cuddio cleaning a log with --map, an AI answer that refers to EMAIL_1 and PASSWORD_1, and cuddio restore putting the real addresses and password back." width="900"></p>

### Clipboard

```bash
cuddio --clip              # copy, clean, paste
cuddio restore --clip
```

Windows and WSL use PowerShell, macOS `pbcopy`/`pbpaste`, and Linux `wl-clipboard`, `xclip` or `xsel`. On Windows, `restore --clip` marks the restored text so clipboard history and cloud sync skip it. The copy you made *before* cleaning stays in any clipboard history, which Cuddio cannot recall.

<p align="center"><img src="docs/gifs/clipboard.gif" alt="Pasting a database error with an IP address and a DSN password, running cuddio --clip, and pasting again to show placeholders." width="900"></p>

### CI checks

```bash
cuddio --check fixtures/support-log.txt     # exit 1 if anything would be hidden
cuddio --report fixtures/support-log.txt    # counts per rule, never the values
cuddio rules --category pii                 # every rule, and which are on
```

Exit codes: `0` nothing to hide (or cleaned), `1` `--check` found something, `2` an error. Errors, `--check` and `--report` never print a matched value, so they are safe in public build logs.

<p align="center"><img src="docs/gifs/check.gif" alt="cuddio --check on an nginx log reports 9 values and exit code 1, --report lists counts per rule, and --check on a clean file reports nothing to hide with exit code 0." width="900"></p>

## PII

| PII | Rule | Default |
|---|---|---|
| Email addresses, including URL-encoded ones | `email` | on |
| UK phone numbers, and international numbers in E.164 (`+...`) | `uk-phone`, `e164-phone` | on |
| Public IPv4 and IPv6 addresses | `ip-address` | on |
| Payment card numbers that pass the Luhn check | `payment-card` | on |
| UK National Insurance numbers | `uk-ni-number` | on |
| Usernames in home directory paths (`/home/x`, `/Users/x`, `C:\Users\x`) | `home-directory` | on |
| Private IP addresses | `private-ip-address` | off |
| UK postcodes (noisy in logs) | `uk-postcode` | off |

In SQL dumps, every value in these columns is hidden whatever it looks like: `email`, `password`, `phone`, `mobile`, `name`, `first_name`, `last_name`, `full_name`, `billing_name`, `shipping_name`, `customer_name`, `contact_name`, `display_name`, `address`, `postcode`, `dob`, `date_of_birth`, `ip_address`, `token`, `remember_token`, `api_token`, `secret`, `two_factor_secret`, `two_factor_recovery_codes`, and Laravel's `sessions.id` and `sessions.payload`. Each also matches with a prefix (`billing_email`), except `name`, so `product_name` is left alone. Add your own with `sql.columns` and `sql.tables`.

Cuddio does not detect names or addresses in free text, outside sensitive SQL columns. **It reduces the risk of sharing personal data; it supports UK GDPR compliance but does not provide it.** Whether you may share data with an AI service is still your organisation's decision.

## Configuration

Cuddio reads `cuddio/config.yaml` in your config directory (`~/.config`, `~/Library/Application Support` or `%AppData%`), then the nearest `.cuddio.yaml`, walking up to the repository root; the project file wins. `--config FILE` replaces the project file. Unknown settings are errors, and errors never repeat values from the file.

```yaml
mode: placeholder              # or fake
placeholder:
  format: '<{label}_{n}>'
rules:
  enable: [uk-postcode]        # rule IDs, categories (secret, pii) or gitleaks
  disable: [high-entropy-assignment]
  custom:
    - id: internal-host
      category: secret
      label: HOST
      pattern: '[a-z]+\.internal\.example\.com'
allowlist:
  values: [hello@example.com]
  patterns: ['^test-']
sql:
  columns: [notes]             # sensitive in every table
  tables:
    orders: [delivery_instructions]
mapping:
  ttl: 30m
  disabled: false              # true refuses to store mappings
```

`cuddio rules` lists every rule (29 built in and 213 derived from gitleaks) with its category, label and whether your config has it on.

<p align="center"><img src="docs/gifs/config.gif" alt="Adding a .cuddio.yaml with an allowlisted address and a custom rule for internal host names, after which the hosts become HOST placeholders and the allowlisted address is kept." width="900"></p>

## Compatibility

| | |
|---|---|
| Platforms | Linux, macOS and Windows, amd64 and arm64; tested in CI on all three, and in WSL |
| Go (for `go install`) | 1.26 or later; CI covers the latest two releases |
| SQL | `mysqldump` (MySQL 8) and `mariadb-dump`, `pg_dump` (plain format with `standard_conforming_strings` on, as it writes by default), `sqlite3 .dump` |
| Text | UTF-8, with LF or CRLF line endings, kept exactly. UTF-16 is refused rather than cleaned |
| Clipboard | Windows PowerShell 5.1 or later, macOS, and Linux with `wl-clipboard`, `xclip` or `xsel` |

See [`SECURITY.md`](SECURITY.md) for exactly what Cuddio reads, writes and stores.

## Testing

```bash
sh scripts/check.sh     # gofmt, go vet, golangci-lint, race tests, 90% coverage gate
sh scripts/e2e.sh       # the built binary, every command, end to end
```

The suite includes golden files for logs and every SQL dialect, SQL output checked by real parsers (the TiDB parser for MySQL, libpg_query for PostgreSQL) and imported into SQLite, fuzz tests for the scanners, memory-bound tests on generated dumps, tests that no matched value ever reaches stdout, stderr or an error, and a check that no network code is linked into the binary. [`CONTRIBUTING.md`](CONTRIBUTING.md) covers setup and adding a rule.

## Acknowledgements

Secret detection includes rules derived from [gitleaks](https://github.com/gitleaks/gitleaks) by Zachary Rice and contributors, MIT licensed, from release v8.30.1. The notice is in [`LICENSE`](LICENSE).

## Links

- [Changelog](CHANGELOG.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Code of conduct](CODE_OF_CONDUCT.md)
- [Website](https://phoenix1331.github.io/cuddio/)
- [MIT licence](LICENSE)
