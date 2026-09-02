# Regenerates the Windows resources embedded in bluntcode.exe:
#   assets/bluntcode.ico                    multi-size icon rendered from the logo SVG
#   cmd/bluntcode/rsrc_windows_amd64.syso   icon + VERSIONINFO the Go linker picks up
#
# Run this after the logo changes or after bumping internal/build/version.go
# (the exe's Properties sheet stamps the version read from that single source),
# then rebuild. Committed outputs mean a plain `go build` stays icon-complete
# on machines without these tools.
#
# Requires on PATH: python (with Pillow), go. Locates Edge and go-winres.
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

$versionGo = Join-Path $root 'internal\build\version.go'
$versionMatch = Select-String -LiteralPath $versionGo -Pattern 'const Version = "([^"]+)"'
if (-not $versionMatch) { throw "Could not read the version from $versionGo" }
$version = $versionMatch.Matches[0].Groups[1].Value

$svg = Join-Path $root 'web\public\bluntcode-mark.svg'
if (-not (Test-Path -LiteralPath $svg)) { throw "Logo SVG not found: $svg" }

$edge = @(
  (Join-Path $env:ProgramFiles 'Microsoft\Edge\Application\msedge.exe'),
  (Join-Path ${env:ProgramFiles(x86)} 'Microsoft\Edge\Application\msedge.exe')
) | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
if (-not $edge) { throw 'Microsoft Edge was not found (used to rasterize the SVG).' }

$temp = Join-Path ([IO.Path]::GetTempPath()) ('bc-icon-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $temp -Force | Out-Null
try {
  # 1. Rasterize the master SVG at 1024 px with a transparent background.
  $png = Join-Path $temp 'icon-1024.png'
  Start-Process -FilePath $edge -Wait -NoNewWindow -ArgumentList @(
    '--headless', '--disable-gpu', '--hide-scrollbars',
    '--window-size=1024,1024', '--default-background-color=00000000',
    "--screenshot=$png", ([Uri]$svg).AbsoluteUri
  )
  if (-not (Test-Path -LiteralPath $png)) { throw 'Edge did not produce the rasterized PNG.' }

  # 2. Pack 10 frame sizes (16..256 px) into one .ico.
  $ico = Join-Path $temp 'bluntcode.ico'
  $python = @'
import sys
from PIL import Image
src, dst = sys.argv[1], sys.argv[2]
sizes = [(16,16),(20,20),(24,24),(32,32),(40,40),(48,48),(64,64),(96,96),(128,128),(256,256)]
Image.open(src).convert("RGBA").save(dst, format="ICO", sizes=sizes)
ico = Image.open(dst)
assert sorted(ico.info["sizes"]) == sorted(sizes), ico.info["sizes"]
print("icon frames:", len(sizes))
'@
  $pyFile = Join-Path $temp 'make_ico.py'
  Set-Content -LiteralPath $pyFile -Value $python -Encoding ASCII
  & python $pyFile $png $ico
  if ($LASTEXITCODE -ne 0) { throw "Pillow failed to build the .ico (exit $LASTEXITCODE)." }

  # 3. Emit the COFF resource the Go linker links into the exe.
  $gopath = (& go env GOPATH) | Select-Object -First 1
  $goWinres = Join-Path $gopath 'bin\go-winres.exe'
  if (-not (Test-Path -LiteralPath $goWinres)) {
    throw "go-winres not found at $goWinres. Install it: go install github.com/tc-hib/go-winres@latest"
  }
  & $goWinres simply --arch amd64 --icon $ico `
    --file-description 'Blunt Code' --product-name 'Blunt Code' `
    --product-version $version --file-version $version `
    --out (Join-Path $root 'cmd\bluntcode\rsrc')
  if ($LASTEXITCODE -ne 0) { throw "go-winres failed (exit $LASTEXITCODE)." }

  # 4. Keep the committed .ico in sync with what the .syso embeds.
  New-Item -ItemType Directory -Path (Join-Path $root 'assets') -Force | Out-Null
  Copy-Item -LiteralPath $ico -Destination (Join-Path $root 'assets\bluntcode.ico') -Force

  Write-Host "Windows resources refreshed for version ${version}:"
  Write-Host ("  {0}" -f (Join-Path $root 'assets\bluntcode.ico'))
  Write-Host ("  {0}" -f (Join-Path $root 'cmd\bluntcode\rsrc_windows_amd64.syso'))
  Write-Host 'Rebuild with: go build -o bluntcode.exe ./cmd/bluntcode'
} finally {
  if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Recurse -Force }
}
