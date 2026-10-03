# verify.ps1 - the one command that proves the tree is shippable.
# Runs the full local gate in order and stops at the first red: gofmt (CRLF-
# tolerant), go vet, go build, go test, then the web half (tsc -b, vitest,
# contrast audit). There is no CI for this repo - this script is the CI.
#
# Usage:  powershell -File scripts\verify.ps1          # everything
#         powershell -File scripts\verify.ps1 -SkipWeb # Go-only iterations

[CmdletBinding()]
param(
    # The web suite is the slow half (a couple of minutes); skip it while
    # iterating on Go-only changes, never before shipping.
    [switch]$SkipWeb
)

$ErrorActionPreference = 'Stop'
# $PSScriptRoot is empty inside param() defaults on Windows PowerShell 5.1,
# so resolve the repo root here in the body.
$root = Split-Path -Parent $PSScriptRoot

$script:greenGates = 0

function Invoke-Gate {
    param([string]$Name, [scriptblock]$Action)
    Write-Host ("== {0}" -f $Name) -ForegroundColor Cyan
    $global:LASTEXITCODE = 0
    & $Action
    if ($LASTEXITCODE -ne 0) {
        Write-Host ("FAIL {0} (exit {1})" -f $Name, $LASTEXITCODE) -ForegroundColor Red
        exit 1
    }
    $script:greenGates++
    Write-Host ("ok   {0}" -f $Name) -ForegroundColor Green
}

function ConvertTo-Lf([string]$text) { $text -replace "`r`n", "`n" }

# gofmt writes UTF-8. PowerShell decodes a native command's stdout with
# [Console]::OutputEncoding, which on a stock Windows console is still ibm850 -
# a legacy IBM code page. Every non-ASCII character in gofmt's output then
# decodes to mojibake, so the comparison below against the UTF-8 file on disk
# fails on any file containing an em-dash, a multiplication sign or an arrow.
#
# Measured on this machine with the default console: 83 correctly-formatted
# files reported as gofmt drift, and after setting UTF-8: zero. The gate was
# reporting failures that did not exist, and - worse - it was only ever wrong
# in the direction of noise, which is how a real formatting regression would
# have slipped through unnoticed. The value was 'ibm850'.
#
# Pin it for the whole script rather than per-call: every other gate shells out
# to a tool too, and npm/go both emit UTF-8 on Windows.
$previousEncoding = [Console]::OutputEncoding
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

Invoke-Gate 'gofmt' {
    # A CRLF checkout makes gofmt -l name files whose only difference from
    # gofmt output is line endings, so every file it flags is re-checked
    # modulo \r before it counts as drift.
    $flagged = @(& gofmt -l cmd internal)
    $drifted = @()
    foreach ($file in $flagged) {
        $path = Join-Path $root $file
        $disk = ConvertTo-Lf ([System.IO.File]::ReadAllText($path))
        $want = ConvertTo-Lf (& gofmt $path | Out-String)
        if ($disk.TrimEnd("`n") -ne $want.TrimEnd("`n")) { $drifted += $file }
    }
    if ($drifted.Count -gt 0) {
        Write-Host "gofmt drift in:" -ForegroundColor Red
        $drifted | ForEach-Object { Write-Host ("  " + $_) -ForegroundColor Red }
        Write-Host "run: gofmt -w <files> (or: go fmt ./...)" -ForegroundColor Yellow
        $global:LASTEXITCODE = 1
    }
}

Invoke-Gate 'go vet' { & go vet ./... }

Invoke-Gate 'go build' { & go build ./... }

Invoke-Gate 'go test' { & go test ./... }

if (-not $SkipWeb) {
    Invoke-Gate 'web: tsc -b' {
        Push-Location (Join-Path $root 'web')
        try { & npx tsc -b } finally { Pop-Location }
    }
    Invoke-Gate 'web: vitest' {
        Push-Location (Join-Path $root 'web')
        try { & npm test } finally { Pop-Location }
    }
    Invoke-Gate 'web: contrast audit' {
        Push-Location (Join-Path $root 'web')
        try { & npm run audit:contrast } finally { Pop-Location }
    }
}

Write-Host ("`nverify: {0} gates green - tree is shippable." -f $script:greenGates) -ForegroundColor Green
[Console]::OutputEncoding = $previousEncoding
exit 0
