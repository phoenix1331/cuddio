#!/bin/sh
# Prints the CHANGELOG.md section for a version, without its heading, for the
# GitHub release notes. Fails if the section is missing or empty.
#   sh .github/scripts/release-notes.sh v0.1.0 > notes.md
set -eu

cd "$(dirname "$0")/../.."

version=${1:?usage: release-notes.sh vX.Y.Z}
version=${version#v}

# From "## [X.Y.Z]" up to the next "## [" heading or the link block
notes=$(awk -v heading="## [$version]" '
  index($0, heading) == 1 { found = 1; next }
  found && (/^## \[/ || /^\[[^]]+\]: /) { exit }
  found { print }
' CHANGELOG.md | sed -e '/./,$!d')

if [ -z "$notes" ]; then
  echo "CHANGELOG.md has no notes under ## [$version]" >&2
  exit 1
fi

printf '%s\n' "$notes"
