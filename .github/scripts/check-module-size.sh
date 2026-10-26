#!/bin/sh
# Fails if the Go module zip would outgrow its budget: 1 MB in total, 256 KB for examples/
# Module zips ignore export-ignore, so every file outside a nested module reaches go install users
set -eu

cd "$(dirname "$0")/../.."

total_limit=1048576
examples_limit=262144

files=$(git ls-files --cached --others --exclude-standard)
nested=$(printf '%s\n' "$files" | grep '/go\.mod$' | sed 's#go\.mod$##' || true)

total=0
examples=0

while IFS= read -r file; do
  if [ ! -f "$file" ] || [ -L "$file" ]; then
    continue
  fi
  for dir in $nested; do
    case "$file" in
      "$dir"*) continue 2 ;;
    esac
  done

  size=$(wc -c < "$file" | tr -d ' ')
  total=$((total + size))
  case "$file" in
    examples/*) examples=$((examples + size)) ;;
  esac
done <<EOF
$files
EOF

echo "module source: $((total / 1024)) KB of $((total_limit / 1024)) KB; examples/: $((examples / 1024)) KB of $((examples_limit / 1024)) KB"

failed=0
if [ "$total" -gt "$total_limit" ]; then
  echo "module source is over budget: generate large test inputs in code instead of committing them"
  failed=1
fi
if [ "$examples" -gt "$examples_limit" ]; then
  echo "examples/ is over budget: keep example files short"
  failed=1
fi

exit "$failed"
