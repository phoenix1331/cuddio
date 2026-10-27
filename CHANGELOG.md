# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `cuddio --version` reports the release version, or the module version for `go install` builds
- Configuration from a project `.cuddio.yaml` (found by walking up to the repository root) layered over a global `cuddio/config.yaml` in the user config directory; the project file wins, including rule toggles (a project can enable a rule the global file disabled); unknown settings are rejected, and errors never repeat values from the file
- `--config FILE` uses a specific config file instead of the project one
- `cuddio` cleans stdin, and `cuddio FILE` cleans a file, writing to stdout; line endings (LF, CRLF, no final newline) are kept exactly, and PEM blocks such as private keys are scanned as one piece
- `-o FILE` writes to a file instead, replacing it in one step so a failed run leaves nothing half-written; it refuses to write over the input (including through a hard link or redirected stdin) and refuses an existing file unless `--force` is given
- `--format text|sql` chooses how input is read; by default `.sql` files are SQL and everything else is text
- Lines longer than 1 MiB stop the run with an error rather than being split, so a value is never cut in two and missed
- Built-in secret rules: AWS, GitHub, Stripe, Slack, OpenAI, Anthropic and Google keys; JWTs; `Authorization` headers and bearer tokens; cookies; passwords in URLs and DSNs; token query parameters; `.env` assignments whose key names a secret (commented-out lines included); secret fields in JSON and PHP arrays; Laravel `APP_KEY`; private key blocks; and long, random-looking assignment values
- Built-in PII rules: email addresses, UK and E.164 phone numbers, public IPv4 and IPv6 addresses, Luhn-checked payment card numbers, UK National Insurance numbers and usernames in home directory paths (`/home/x`, `/Users/x`, `C:\Users\x`); private IP addresses and UK postcodes are available but off by default
- Custom rules in config (`rules.custom`: `id`, `category`, `label`, RE2 `pattern`, with an optional `(?P<value>...)` group to hide only part of a match); a custom rule with a built-in's ID replaces it
- Config files are checked as they load: invalid patterns, placeholder formats, modes, negative mapping TTLs and duplicate custom rule IDs are reported with the file name, never the offending value
- About 200 secret rules vendored from gitleaks v8.30.1 (GitLab, npm, SendGrid, Shopify, DigitalOcean, Twilio and many more), on by default with gitleaks' entropy and allowlist checks; switch them all off with `rules.disable: [gitleaks]` or one at a time by ID. cuddio's own rules win when both match the same value
- Config errors never repeat a value from the file: unknown settings and rule names are named only as a near miss of a real one ("did you mean disable?"), and invalid custom rule IDs and categories are reported by position
- Passwords in PDO DSNs (`pgsql:host=db;password=...`) and URL-encoded email addresses (`?email=alice%40example.com`) are now hidden
- Only the secret part of a match is replaced (`DB_PASSWORD=<SECRET_1>`, `mysql://app:<PASSWORD_1>@db`), and empty values, `null`, references such as `${DB_PASSWORD}` and existing placeholders are left alone, so cleaning output twice changes nothing
- SQL dumps are cleaned as SQL: `.sql` files, `--format sql` or `--dialect`. MySQL and MariaDB (`mysqldump`), PostgreSQL (`pg_dump`, including `COPY ... FROM stdin` blocks) and SQLite (`sqlite3 .dump`) are supported, and the output stays valid SQL. Every string value and comment goes through the rules, and whole values are hidden in sensitive columns (`email`, `password`, `phone`, `name`, `address`, `postcode`, `remember_token` and more, `billing_email`-style names too, plus Laravel's `sessions.id` and `payload`) even when no pattern matches them
- The dialect is detected from the start of the dump; `--dialect mysql|postgres|sqlite` sets it when cuddio cannot tell
- `sql.columns` and `sql.tables` in config add sensitive columns for every table or for one table
- `--mode fake` (or `mode: fake` in config) writes realistic fake values instead of placeholders, so a cleaned dump still imports and the app still runs: `user1@example.invalid`, phone numbers from Ofcom's drama ranges laid out like the original, documentation IP addresses, published test card numbers, `QQ` National Insurance numbers, `ZZ99` postcodes, plausible names, `REDACTED` passwords, and same-length scrambles for keys and tokens. The same value always gets the same fake, so joins on email still line up, and fakes are cut down to fit `char` and `varchar` columns
- `--map` keeps the placeholder mapping for the session (owner-only file in the user cache directory, expiring after `mapping.ttl`, default 1 hour), so `cuddio restore` can put the real values back into an AI's answer; placeholders cuddio did not hand out are left alone, and restored SQL is re-escaped so it stays valid. Later `--map` runs carry on the numbering, so several files cleaned for one chat never share a placeholder
- `cuddio forget` deletes stored mappings straight away
- `--clip` cleans the clipboard in place (or writes to `--output`), and `restore --clip` puts real values back on it; works on Windows, WSL (through PowerShell, so non-ASCII text survives), macOS and Linux (wl-clipboard, xclip or xsel), with a hint naming what to install when no clipboard tool is found. On Windows and WSL, restored text is kept out of clipboard history and cloud sync
- `--check` prints how many values would be hidden and exits 1 if there are any, for CI; `--report` prints a count per rule ID (and per sensitive SQL column), never the values. Both work on files, stdin, the clipboard and SQL, and leave the input untouched
- `cuddio rules` lists every rule (built-in, gitleaks and custom from config) with its category, label, default state, whether config has it on, and source; `--category secret|pii` narrows the list
- UTF-16 input (what Windows PowerShell 5.1 writes with `>` and `Out-File`) is refused with instructions to convert it, instead of passing through with nothing hidden; `--help` advises giving cuddio the file name and `-o` in Windows PowerShell 5.1, since `|` and `>` re-encode text
- On Windows, `--map`, `restore` and `forget` now report a file sitting where the cache folder should be, as on Linux and macOS, instead of treating it as an empty store (so `--map` no longer writes output it then cannot save a mapping for)
- `--map` refuses to store a mapping when the input already uses one of the session's placeholders for something else (such as cleaning cleaned output), since restore could not tell them apart; placeholder-shaped text in the input is otherwise kept, and new placeholders skip its numbers

[Unreleased]: https://github.com/phoenix1331/cuddio/commits/main
