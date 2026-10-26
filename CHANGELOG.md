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
- Passwords in PDO DSNs (`pgsql:host=db;password=...`) and URL-encoded email addresses (`?email=alice%40example.com`) are now hidden
- Only the secret part of a match is replaced (`DB_PASSWORD=<SECRET_1>`, `mysql://app:<PASSWORD_1>@db`), and empty values, `null`, references such as `${DB_PASSWORD}` and existing placeholders are left alone, so cleaning output twice changes nothing

[Unreleased]: https://github.com/phoenix1331/cuddio/commits/main
