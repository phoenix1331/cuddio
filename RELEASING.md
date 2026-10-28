# Releasing

Cuddio follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Before 1.0, a breaking change bumps the minor version (`0.1.x` to `0.2.0`) and is listed under `### Changed` or `### Removed` in the changelog. Breaking includes removed or renamed flags and commands, changed exit codes, config keys, rule IDs, the placeholder or mapping file format, `--check` and `--report` output, and a higher minimum Go version.

## Steps

1. Make sure `main` is green on the `tests` workflow, run by hand so all three operating systems run.
2. Run the local gate: `sh scripts/check.sh`, `govulncheck ./...`, then `goreleaser release --snapshot --clean` and `sh .github/scripts/check-archives.sh`.
3. If a dependency changed since the last release, run `sh scripts/update-notices.sh` (the tests fail until you do).
4. In `CHANGELOG.md`, rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD`, add a fresh empty `## [Unreleased]` above it, and update the links at the bottom:

   ```markdown
   [Unreleased]: https://github.com/phoenix1331/cuddio/compare/vX.Y.Z...HEAD
   [X.Y.Z]: https://github.com/phoenix1331/cuddio/compare/vA.B.C...vX.Y.Z
   ```

   The first release links to `https://github.com/phoenix1331/cuddio/releases/tag/v0.1.0` instead.
5. Check the notes: `sh .github/scripts/release-notes.sh vX.Y.Z`.
6. If the release breaks anything, add a section to `UPGRADING.md` (create it on the first breaking change) with the exact flag or config change needed.
7. Commit: `chore: release vX.Y.Z`.
8. Tag and push, signing the tag:

   ```bash
   git tag -s vX.Y.Z -m "vX.Y.Z"
   git push origin main vX.Y.Z
   ```

9. The `release` workflow runs the gate again, then GoReleaser builds Linux, macOS and Windows binaries for amd64 and arm64, publishes the GitHub release with the changelog section as its notes, and updates the Homebrew tap and Scoop bucket. It fails before publishing anything if the changelog section is missing or an archive holds more than the binary, `LICENSE` and `README.md`.
10. Check every install method: the release download, `go install github.com/phoenix1331/cuddio@vX.Y.Z`, `brew install phoenix1331/tap/cuddio` and `scoop install cuddio`. pkg.go.dev lists the version after the first `go install` of it.

## Never move a tag

The Go module proxy and checksum database record a version the first time anyone fetches it, so a moved or deleted tag breaks `go install` for everyone who already has it. If a release is broken, release a fixed version and retract the broken one in `go.mod`:

```go
retract v0.1.1 // restore wrote values to the wrong placeholders
```

The retraction ships in the fixed release; `go install ...@latest` then skips the broken version and `go list -m -versions` warns about it.

## Setup (once)

- Repositories `phoenix1331/homebrew-tap` and `phoenix1331/scoop-bucket` exist, each with at least one commit.
- A fine-grained personal access token with **Contents: read and write** on those two repositories only is saved as the `TAP_GITHUB_TOKEN` Actions secret on this repository. It expires, so renew it before a release when it is close.
- The repository is public, with its description, topics and homepage (`https://phoenix1331.github.io/cuddio`) set, and Pages publishing from `/docs` on `main`.

## Before 1.0

1.0 is a long-term commitment: v2 and later need a new module path (`github.com/phoenix1331/cuddio/v2`). Stay on 0.x until the flags, config format, rule IDs and placeholder format have settled through real use.
