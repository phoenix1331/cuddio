#!/bin/sh
# End-to-end check of the cuddio binary: builds it, then runs every command
# on generated input and checks the output and exit codes. Runs on Linux,
# macOS and Windows (Git Bash). Config and cache folders point into a
# temporary directory, so a developer's own settings and stored mappings
# are never read or touched.
set -eu

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# native turns a POSIX path into one a Windows program understands
native() {
    if command -v cygpath > /dev/null 2>&1; then
        cygpath -w "$1"
    else
        printf '%s' "$1"
    fi
}

bin="$work/cuddio"
case "$(uname -s)" in
    MINGW* | MSYS* | CYGWIN*) bin="$bin.exe" ;;
esac
go build -o "$bin" .

mkdir -p "$work/home" "$work/run"
HOME=$(native "$work/home")
XDG_CONFIG_HOME="$HOME"
XDG_CACHE_HOME="$HOME"
APPDATA="$HOME"
LOCALAPPDATA="$HOME"
export HOME XDG_CONFIG_HOME XDG_CACHE_HOME APPDATA LOCALAPPDATA
cd "$work/run"

checks=0
fail() {
    echo "e2e: FAIL: $*" >&2
    exit 1
}

# expect CODE COMMAND...: run cuddio, keeping stdout in out.txt and stderr
# in err.txt, and fail unless it exits with CODE
expect() {
    want=$1
    shift
    set +e
    "$bin" "$@" > out.txt 2> err.txt
    got=$?
    set -e
    checks=$((checks + 1))
    [ "$got" -eq "$want" ] || fail "cuddio $* exited $got, want $want: $(cat err.txt)"
}

has() { grep -q -- "$1" "$2" || fail "$2 lacks '$1': $(cat "$2")"; }
lacks() { if grep -q -- "$1" "$2"; then fail "$2 still holds '$1'"; fi; }

# The values below are fake, but in the shapes cuddio's rules look for
printf 'login alice@shop.test from 203.0.113.9 as Llŷr\ndsn mysql://app:s3cr3t-pw@db.shop.test/app\nDB_PASSWORD=hunter2-not-real\n' > app.log
cp app.log original.log

expect 0 --version
has 'cuddio' out.txt

expect 0 app.log
has '<EMAIL_1>' out.txt
has '<IP_1>' out.txt
has 'Llŷr' out.txt
for secret in alice@shop.test 203.0.113.9 s3cr3t-pw hunter2-not-real; do lacks "$secret" out.txt; done

# stdin, and line endings kept exactly
printf 'a alice@shop.test\r\nb\r\nno final newline' > crlf.log
set +e
"$bin" < crlf.log > out.txt
set -e
printf 'a <EMAIL_1>\r\nb\r\nno final newline' > want.txt
cmp -s out.txt want.txt || fail "CRLF input did not come back byte for byte"

# -o: never the input, never an existing file without --force
expect 0 app.log -o clean.log
has '<EMAIL_1>' clean.log
expect 2 app.log -o clean.log
expect 0 app.log -o clean.log --force
expect 2 app.log -o app.log --force
cmp -s app.log original.log || fail "the input file changed"

# Native paths (C:\... on Windows) for -o and --config
mkdir -p "$work/run/sub dir"
expect 0 app.log -o "$(native "$work/run/sub dir/out.log")"
has '<EMAIL_1>' "sub dir/out.log"
printf 'rules:\n  disable: [email]\n' > "sub dir/cuddio.yaml"
expect 0 app.log --config "$(native "$work/run/sub dir/cuddio.yaml")"
has 'alice@shop.test' out.txt
has '<IP_1>' out.txt

# --check and --report print counts only
expect 1 --check app.log
has 'would be hidden' out.txt
lacks alice@shop.test out.txt
printf 'nothing secret\n' > plain.log
expect 0 --check plain.log
has 'nothing to hide' out.txt
expect 0 --report app.log
has 'total' out.txt
lacks alice@shop.test out.txt

# SQL, with dialect detection and an explicit dialect
printf -- "-- MySQL dump 10.13\nINSERT INTO \`users\` (\`name\`, \`email\`) VALUES ('Alice O\\\\'Brien','alice@shop.test');\n" > dump.sql
expect 0 dump.sql
has "'<NAME_1>','<EMAIL_1>'" out.txt
printf 'COPY public.users (name, email) FROM stdin;\nAlice\talice@shop.test\n\\.\n' > copy.sql
expect 0 copy.sql --dialect postgres
has '<NAME_1>	<EMAIL_1>' out.txt
expect 0 dump.sql --mode fake
has 'user1@example.invalid' out.txt

# --map, restore and forget
expect 0 app.log --map -o mapped.log
printf 'Write to <EMAIL_1> about <IP_1>; ignore <EMAIL_9>.\n' > answer.md
expect 0 restore answer.md
has 'Write to alice@shop.test about 203.0.113.9; ignore <EMAIL_9>.' out.txt
expect 0 restore mapped.log -o restored.log
cmp -s restored.log app.log || fail "restore did not give the original back"
expect 0 forget
expect 2 restore answer.md
has 'no stored mapping' err.txt

# rules
expect 0 rules
has 'gitleaks' out.txt
expect 0 rules --category pii
has 'email' out.txt
lacks 'aws-access-key' out.txt

# UTF-16, as Windows PowerShell 5.1 writes files, is refused, not passed through
printf '\377\376a\000@\000b\000.\000c\000' > utf16.log
expect 2 utf16.log
has 'UTF-16' err.txt

echo "e2e: $checks commands passed"
