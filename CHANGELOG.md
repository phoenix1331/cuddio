# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `cuddio --version` reports the release version, or the module version for `go install` builds
- Configuration from a project `.cuddio.yaml` (found by walking up to the repository root) layered over a global `cuddio/config.yaml` in the user config directory; unknown settings are rejected, and errors never repeat values from the file
- `--config FILE` uses a specific config file instead of the project one

[Unreleased]: https://github.com/phoenix1331/cuddio/commits/main
