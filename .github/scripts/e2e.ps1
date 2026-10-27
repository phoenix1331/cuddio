# End-to-end checks that only Windows PowerShell 5.1 can make: a UTF-8
# file named on the command line comes back intact, and the UTF-16 that
# PowerShell 5.1 writes with > is refused rather than passed through.
# CUDDIO_BIN names a binary to test instead of building one.

# Continue, not Stop: in PowerShell 5.1, redirecting a native command's
# stderr turns each line into an error record, which Stop would throw.
# Exit codes are checked by hand instead.
$ErrorActionPreference = 'Continue'

$temp = $env:RUNNER_TEMP
if (-not $temp) { $temp = $env:TEMP }
$work = Join-Path $temp 'cuddio-e2e'
New-Item -ItemType Directory -Force -Path $work | Out-Null
$bin = $env:CUDDIO_BIN
if (-not $bin) {
    $bin = Join-Path $work 'cuddio.exe'
    go build -o $bin .
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
}

function Fail($message) {
    Write-Error "e2e.ps1: FAIL: $message"
    exit 1
}

# UTF-8 without a byte order mark, written by .NET so PowerShell's own
# encoding choices play no part. The non-ASCII text is built from code
# points: PowerShell 5.1 reads a script without a BOM as Windows-1252.
$name = 'Ll' + [char]0x0177 + 'r caf' + [char]0x00E9
$in = Join-Path $work 'app.log'
$out = Join-Path $work 'clean.log'
[IO.File]::WriteAllText($in, "login alice@shop.test as $name`r`n", (New-Object Text.UTF8Encoding $false))

& $bin $in -o $out
if ($LASTEXITCODE -ne 0) { Fail "cuddio FILE -o exited $LASTEXITCODE" }
$cleaned = [IO.File]::ReadAllText($out, [Text.Encoding]::UTF8)
if ($cleaned -ne "login <EMAIL_1> as $name`r`n") { Fail "unexpected output: $cleaned" }

# Windows PowerShell 5.1 writes UTF-16 with >, which no rule can match
$utf16 = Join-Path $work 'utf16.log'
'login alice@shop.test' > $utf16
& $bin $utf16 2> $null
if ($LASTEXITCODE -ne 2) { Fail "UTF-16 input exited $LASTEXITCODE, want 2" }

Write-Output 'e2e.ps1: passed'
