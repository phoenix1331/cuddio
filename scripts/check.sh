#!/bin/sh
# Runs the quality gate in order: formatting, vet, lint, race tests and the coverage threshold
set -eu

cd "$(dirname "$0")/.."
minimum=90

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  echo "gofmt: these files need formatting:"
  echo "$unformatted"
  exit 1
fi
echo "gofmt: ok"

go vet ./...
echo "go vet: ok"

golangci-lint run
go test -race -coverprofile=coverage.out ./...

total=$(go tool cover -func=coverage.out | awk '/^total:/ { sub("%", "", $3); print $3 }')
echo "coverage: ${total}% (minimum ${minimum}%)"
if ! awk -v total="$total" -v minimum="$minimum" 'BEGIN { exit !(total >= minimum) }'; then
  echo "coverage is below ${minimum}%"
  exit 1
fi
