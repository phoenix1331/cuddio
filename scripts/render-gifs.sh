#!/bin/sh
# Renders tapes/*.tape into docs/gifs/ with VHS. Needs vhs, ttyd and ffmpeg
# on PATH; run it in WSL or Linux, not the VHS Docker image. It builds
# cuddio from this checkout and records each tape in a fresh copy of
# examples/, with its own config and cache folders, so recordings never
# read or touch your settings or stored mappings.
#
# The clipboard tape uses the Windows clipboard, so it only renders in WSL;
# the clipboard is saved first and put back afterwards.
#
#   sh scripts/render-gifs.sh            every tape
#   sh scripts/render-gifs.sh hero sql   only these
set -eu

cd "$(dirname "$0")/.."

for tool in vhs ttyd ffmpeg; do
    if ! command -v "$tool" > /dev/null 2>&1; then
        echo "render-gifs: $tool is not installed" >&2
        exit 1
    fi
done

work=$(mktemp -d)
saved=""
restore_clipboard() {
    if [ -n "$saved" ]; then
        powershell.exe -NoProfile -Command '[Console]::InputEncoding=[Text.Encoding]::UTF8; Set-Clipboard -Value ([Console]::In.ReadToEnd())' < "$saved" || true
    fi
    rm -rf "$work"
}
trap restore_clipboard EXIT

mkdir -p "$work/bin" docs/gifs
go build -o "$work/bin/cuddio" .
PATH="$work/bin:$PATH"
XDG_CONFIG_HOME="$work/config"
XDG_CACHE_HOME="$work/cache"
export PATH XDG_CONFIG_HOME XDG_CACHE_HOME

wsl=false
if grep -qi microsoft /proc/version 2> /dev/null; then
    wsl=true
fi

if [ "$#" -eq 0 ]; then
    set -- hero sql restore clipboard check config
fi

for name in "$@"; do
    tape="tapes/$name.tape"
    [ -f "$tape" ] || { echo "render-gifs: no $tape" >&2; exit 1; }

    if [ "$name" = clipboard ]; then
        if [ "$wsl" != true ]; then
            echo "render-gifs: skipping clipboard (needs WSL)"
            continue
        fi
        if [ -z "$saved" ]; then
            saved="$work/clipboard"
            # A clipboard holding no text saves as an empty file, which is
            # not written back
            powershell.exe -NoProfile -Command '[Console]::OutputEncoding=[Text.Encoding]::UTF8; $c=Get-Clipboard -Raw; if ($c -ne $null) { [Console]::Out.Write($c) }' > "$saved"
            [ -s "$saved" ] || saved=""
        fi
    fi

    rm -rf "$work/demo"
    cp -R examples "$work/demo"
    rm -rf "$work/cache"
    CUDDIO_DEMO="$work/demo" vhs "$tape" > "$work/vhs.log" 2>&1 || {
        cat "$work/vhs.log" >&2
        exit 1
    }
    echo "render-gifs: docs/gifs/$name.gif ($(($(wc -c < "docs/gifs/$name.gif") / 1024)) KB)"
done
