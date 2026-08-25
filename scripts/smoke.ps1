# smoke.ps1 - headless end-to-end smoke test for Blunt Code.
# Boots a freshly built exe on a random loopback port, probes the health/meta
# APIs and one SPA route, then shuts it down. Exits 0 only when every probe
# passes. Usage: .\scripts\smoke.ps1 [-ExePath .\bluntcode.exe]

[CmdletBinding()]
param(
    [string]$ExePath = (Join-Path $PSScriptRoot '..\bluntcode.exe')
)

$ErrorActionPreference = 'Stop'

if (-not (Test-Path $ExePath)) {
    Write-Error "Blunt Code executable not found at $ExePath. Build it first (scripts/build.ps1)."
    exit 1
}

# A throwaway LOCALAPPDATA isolates the smoke run from any real installation
# and from other Blunt Code instances holding their data-directory locks.
$isolatedData = Join-Path ([System.IO.Path]::GetTempPath()) ("bluntcode-smoke-" + [guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Path $isolatedData | Out-Null
$env:LOCALAPPDATA_BACKUP = $env:LOCALAPPDATA
$env:LOCALAPPDATA = $isolatedData

$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = (Resolve-Path $ExePath).Path
$psi.Arguments = '--no-browser'
$psi.UseShellExecute = $false
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true
$process = [System.Diagnostics.Process]::Start($psi)

function Stop-SmokeProcess {
    try { if ($process -and -not $process.HasExited) { $process.Kill() } } catch { }
}

$baseUrl = $null
try {
    # The server prints "Blunt Code listening on http://127.0.0.1:<port>/" once ready.
    $deadline = [DateTime]::UtcNow.AddSeconds(20)
    while ([DateTime]::UtcNow -lt $deadline) {
        $line = $process.StandardOutput.ReadLine()
        if ($null -ne $line -and $line -match 'listening on (http://\S+/)') { $baseUrl = $Matches[1]; break }
        if ($process.HasExited) { break }
    }
    if (-not $baseUrl) {
        Write-Host 'FAIL: server never announced its URL.' -ForegroundColor Red
        exit 1
    }
    Write-Host "Server ready at $baseUrl"

    $failures = 0

    # 1) Health endpoint answers with JSON.
    $health = Invoke-RestMethod -Uri ($baseUrl + 'api/v1/health') -TimeoutSec 5
    if (-not $health.status) { Write-Host 'FAIL: /api/v1/health missing status.' -ForegroundColor Red; $failures++ }

    # 2) Meta endpoint reports the version.
    $meta = Invoke-RestMethod -Uri ($baseUrl + 'api/v1/meta') -TimeoutSec 5
    if (-not $meta.version) { Write-Host 'FAIL: /api/v1/meta missing version.' -ForegroundColor Red; $failures++ }

    # 3) SPA fallback serves index.html for a client-side route.
    $spa = Invoke-WebRequest -Uri ($baseUrl + 'workspaces') -TimeoutSec 5 -UseBasicParsing
    if ($spa.Content -notmatch '<div id="root">') { Write-Host 'FAIL: SPA fallback did not serve the app shell.' -ForegroundColor Red; $failures++ }

    # 4) Findings search answers even with an empty database.
    $search = Invoke-RestMethod -Uri ($baseUrl + 'api/v1/findings/search?page=1&page_size=1') -TimeoutSec 5
    if ($null -eq $search.total) { Write-Host 'FAIL: findings/search missing total.' -ForegroundColor Red; $failures++ }

    if ($failures -gt 0) {
        Write-Host "SMOKE FAILED with $failure(s)." -ForegroundColor Red
        exit 1
    }
    Write-Host 'SMOKE PASSED: health, meta, SPA fallback, findings search.' -ForegroundColor Green
    exit 0
}
catch {
    Write-Host "SMOKE FAILED: $_" -ForegroundColor Red
    exit 1
}
finally {
    Stop-SmokeProcess
    $env:LOCALAPPDATA = $env:LOCALAPPDATA_BACKUP
    Remove-Item Env:\LOCALAPPDATA_BACKUP -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 300
    Remove-Item $isolatedData -Recurse -Force -ErrorAction SilentlyContinue
}
