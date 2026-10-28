#!/bin/sh
# Regenerates internal/notices/notices.txt, the licence notices that
# cuddio licences prints: cuddio's own LICENSE, the gitleaks notice,
# Go's, and every module linked into a Linux, macOS or Windows build.
# Run it after adding or updating a dependency; TestNoticesCoverEveryModule
# fails until it has been run.
set -eu

cd "$(dirname "$0")/.."

out=internal/notices/notices.txt

modules=$(for os in linux darwin windows; do
  GOOS=$os go list -deps -f '{{with .Module}}{{.Path}}{{end}}' .
done | grep -v '^github.com/phoenix1331/cuddio$' | sort -u)

{
  echo "cuddio"
  echo
  cat LICENSE
  echo
  echo "---"
  echo
  cat internal/rules/LICENSE-gitleaks
  echo
  echo "== Go (standard library and runtime) =="
  echo
  cat "$(go env GOROOT)/LICENSE"

  for module in $modules; do
    dir=$(go list -m -f '{{.Dir}}' "$module")
    files=$(find "$dir" -maxdepth 1 -type f \( -iname 'licen[cs]e*' -o -iname 'copying*' -o -iname 'notice*' \) | sort)
    if [ -z "$files" ]; then
      echo "no licence file found for $module" >&2
      exit 1
    fi
    echo
    echo "== $module =="
    for file in $files; do
      echo
      cat "$file"
    done
  done
} > "$out"

echo "wrote $out"
