[CmdletBinding()]
param(
  [string]$Version = '0.2.0',
  [string]$OutputDir = (Join-Path (Split-Path -Parent $PSScriptRoot) 'dist')
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
& (Join-Path $PSScriptRoot 'build.ps1')
if ($LASTEXITCODE -ne 0) { throw "Build failed with exit code $LASTEXITCODE" }

$output = [IO.Path]::GetFullPath($OutputDir)
$releaseName = "BluntCode-$Version-windows-amd64"
$payload = Join-Path $output $releaseName
$archive = Join-Path $output "$releaseName.zip"
if (Test-Path -LiteralPath $payload) { Remove-Item -LiteralPath $payload -Recurse -Force }
if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive -Force }
New-Item -ItemType Directory -Path $payload -Force | Out-Null

Copy-Item -LiteralPath (Join-Path $root 'bluntcode.exe') -Destination $payload
Copy-Item -LiteralPath (Join-Path $root 'README.md') -Destination $payload
Copy-Item -LiteralPath (Join-Path $root 'LICENSE') -Destination $payload
Copy-Item -LiteralPath (Join-Path $root 'THIRD_PARTY_NOTICES.md') -Destination $payload
Copy-Item -LiteralPath (Join-Path $root 'scripts\uninstall.ps1') -Destination $payload
Copy-Item -LiteralPath (Join-Path $root 'scripts\install-latest.ps1') -Destination $output -Force
Compress-Archive -LiteralPath $payload -DestinationPath $archive -Force
$hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
Set-Content -LiteralPath "$archive.sha256" -Value "$hash  $(Split-Path -Leaf $archive)" -NoNewline
Write-Host "Package: $archive"
Write-Host "SHA256:  $hash"
Write-Host "Installer: $(Join-Path $output 'Install-BluntCode.ps1')"
