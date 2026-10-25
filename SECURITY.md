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

cuddio processes text locally and never makes a network connection; the test suite fails if network code is linked into the binary. Redaction is best effort: a clean result is not a guarantee that no secret or personal data remains, so review the output before you share it. Use cuddio at your own risk.

## Disclosure Policy

Once a fix is released, the vulnerability will be documented in `CHANGELOG.md` under the relevant version. We follow coordinated disclosure and will not publish details until a fix is available.
