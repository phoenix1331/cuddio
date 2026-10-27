#!/bin/sh
# Runs every package's tests as native Windows programs, from WSL: each
# test binary is cross-compiled for Windows and started through WSL's
# Windows interop, from its package folder. Checks Windows behaviour
# without CI. The no-network tests are skipped because they need the go
# command, which the Windows side may not have; Linux and CI run them.
set -eu

if ! grep -qi microsoft /proc/version 2> /dev/null; then
    echo "test-windows: needs WSL (Windows Subsystem for Linux)" >&2
    exit 1
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

skip='^(TestBuildExcludesTestOnlyPackages|TestBuildImportsNoNetworkPackages|TestBinaryContainsNoNetworkCode)$'
status=0
for pkg in $(go list ./...); do
    dir=$(go list -f '{{.Dir}}' "$pkg")
    exe="$work/$(echo "$pkg" | tr / _).test.exe"
    GOOS=windows GOARCH=amd64 go test -c -o "$exe" "$pkg"
    # No test files, no binary
    [ -f "$exe" ] || continue
    if (cd "$dir" && "$exe" -test.skip "$skip" > "$work/out" 2>&1); then
        echo "ok   $pkg"
    else
        echo "FAIL $pkg"
        cat "$work/out"
        status=1
    fi
done

exit $status
