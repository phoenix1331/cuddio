# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.x     | Latest minor only |

While cuddio is pre-1.0, fixes are released on the latest minor version only.

## Reporting a Vulnerability

Please do not open a public GitHub issue for security vulnerabilities, and never include real secrets or personal data in a report.

Report privately through [GitHub security advisories](https://github.com/phoenix1331/cuddio/security/advisories/new). Include:

- A description of the vulnerability and its potential impact
- Steps to reproduce, using fake data only
- The cuddio version (`cuddio --version`) and your operating system

You will receive an acknowledgement within 48 hours. If the issue is confirmed, a fix will be released as soon as possible and you will be credited in the changelog unless you prefer to remain anonymous.

## Scope

Use cuddio at your own risk. Redaction is best effort: a clean result, or `--check` reporting nothing to hide, is not a guarantee that no secret or personal data remains. Review the output before you share it. cuddio reduces the risk of sharing personal data; it does not make sharing it compliant with UK GDPR or any policy on its own.

### What cuddio touches

| | |
|---|---|
| Network | None, ever. The test suite fails if a network package is imported, or if any dial, listen, resolver or HTTP code is linked into the binary |
| Input | The file you name, stdin, or the clipboard (`--clip`). Input files are only read, never written, including through links or redirection |
| Output | stdout, the clipboard (`--clip`), or the `-o` file, which is written to a temporary file (mode `0600`) and renamed into place. `-o` refuses the input file and, without `--force`, any existing file |
| Config | Reads `cuddio/config.yaml` in your config directory and the nearest `.cuddio.yaml`, walking up no further than the repository root. Errors never repeat values from these files |
| Mappings | Only with `--map`: `mapping.json` in a `cuddio` folder in your cache directory (`~/.cache`, `~/Library/Caches` or `%LocalAppData%`), folder `0700`, file `0600`. It holds the real values behind each placeholder, **unencrypted**. It expires after `mapping.ttl` (default 1 hour), is purged on every use, and `cuddio forget` deletes it at once. `mapping.disabled: true` in config turns storage off. On Windows, Go cannot set Unix modes, so the file relies on the per-user permissions of `%LocalAppData%` |
| Clipboard | `--clip` runs the platform's own tools: PowerShell on Windows and WSL, `pbcopy`/`pbpaste` on macOS, `wl-clipboard`, `xclip` or `xsel` on Linux. Text goes through their stdin and stdout, never their arguments, and their error output is never shown. `restore --clip` marks restored text so Windows clipboard history and cloud sync skip it. **The copy you made before cleaning stays in any clipboard history or clipboard manager**: cuddio cannot recall it |
| Errors and reports | Never print a matched value. `--check` and `--report` print counts and rule IDs only |

### What cuddio detects

- **Secrets:** 20 built-in rules (cloud and AI provider keys, GitHub, Stripe and Slack tokens, JWTs, `Authorization` headers, bearer tokens, cookies, passwords in URLs and DSNs, token query parameters, secret `.env` assignments and JSON fields, Laravel `APP_KEY`, private keys, long random-looking values) and 213 rules vendored from [gitleaks](https://github.com/gitleaks/gitleaks)
- **Personal data:** email addresses, UK and E.164 phone numbers, public IP addresses, Luhn-checked card numbers, UK National Insurance numbers and usernames in home directory paths. UK postcodes and private IP addresses are detected only when enabled
- **SQL dumps:** every value in sensitive columns (emails, passwords, phones, names, addresses, postcodes, dates of birth, IP addresses, tokens, secrets, Laravel session IDs and payloads), whether or not a pattern matches. `cuddio rules` lists every rule

### What cuddio does not detect

- Names and addresses in free text, outside sensitive SQL columns
- Secrets with no recognisable shape or context, such as a random string on its own line, or a token in a URL path (`/reset/abc123`) rather than a query parameter
- Phone numbers in national formats other than the UK's, unless written in E.164 (`+...`)
- Text outside quoted values in SQL mode; use text mode for loose text
- Images, PDFs and other binary files. UTF-16 text (as Windows PowerShell 5.1 writes) is refused rather than cleaned

### Reporting

Report privately (above) anything that makes cuddio print, store or send a value it should not: a value in an error, report or `--check` output; a changed input file; a mapping file readable by other users; network access; a temporary file left behind. A value that no rule detects is a false negative: open a public issue with the "False negative" template, using made-up values in the same shape, never the real one.

## Disclosure Policy

Once a fix is released, the vulnerability will be documented in `CHANGELOG.md` under the relevant version. We follow coordinated disclosure and will not publish details until a fix is available.
