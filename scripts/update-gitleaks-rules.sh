#!/bin/sh
# Regenerates internal/rules/gitleaks_gen.go from gitleaks' default config.
# With no arguments it rebuilds the pinned version; to update, pass the new
# release tag and the SHA-256 of its config/gitleaks.toml:
#   sh scripts/update-gitleaks-rules.sh v8.30.1 e163e53b...
set -eu

cd "$(dirname "$0")/.."

version=${1:-v8.30.1}
sum=${2:-e163e53b9e7e8a8511e77271e2b323ed057759542a6d988258afe3a1fa329caf}
# generic-api-key: slow, noisy, covered by built-in assignment rules
# gcp-api-key: its allowlist holds live-format Google keys push protection may block
exclude=generic-api-key,gcp-api-key
out=internal/rules/gitleaks_gen.go

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL -o "$tmp/gitleaks.toml" "https://raw.githubusercontent.com/gitleaks/gitleaks/$version/config/gitleaks.toml"

if command -v sha256sum > /dev/null 2>&1; then
  actual=$(sha256sum "$tmp/gitleaks.toml" | cut -d ' ' -f 1)
else
  actual=$(shasum -a 256 "$tmp/gitleaks.toml" | cut -d ' ' -f 1)
fi
if [ "$actual" != "$sum" ]; then
  echo "checksum mismatch for gitleaks $version: got $actual" >&2
  exit 1
fi

go -C tools/gitleaksgen run . -in "$tmp/gitleaks.toml" -version "$version" -sha256 "$sum" -exclude "$exclude" > "$tmp/gitleaks_gen.go"
mv "$tmp/gitleaks_gen.go" "$out"
echo "wrote $out from gitleaks $version"
echo "next: go test ./internal/rules, review the diff, update the pinned version in this script and CONTEXT.md"
