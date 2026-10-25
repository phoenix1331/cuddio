#!/bin/sh
# Fails if any GoReleaser archive in dist/ holds anything besides the binary, LICENSE and README.md
set -eu

cd "$(dirname "$0")/../.."

found=0
failed=0

for archive in dist/*.tar.gz dist/*.zip; do
  [ -e "$archive" ] || continue
  found=$((found + 1))

  case "$archive" in
    *.zip) entries=$(unzip -Z1 "$archive") ;;
    *) entries=$(tar -tzf "$archive") ;;
  esac

  binary=0
  for entry in $entries; do
    case "$entry" in
      cuddio|cuddio.exe) binary=1 ;;
      LICENSE|README.md) ;;
      *)
        echo "$archive: unexpected file $entry"
        failed=1
        ;;
    esac
  done

  if [ "$binary" -eq 0 ]; then
    echo "$archive: no cuddio binary"
    failed=1
  fi
done

if [ "$found" -eq 0 ]; then
  echo "no archives in dist/: run goreleaser release --snapshot --clean first"
  exit 1
fi

if [ "$failed" -ne 0 ]; then
  exit 1
fi

echo "archives: ok ($found checked, each holds only the binary, LICENSE and README.md)"
