[CmdletBinding()]
param(
  [string]$Version = '0.49.1',
  # No $PSScriptRoot here: Windows PowerShell 5.1 leaves it empty inside
  # param() default expressions, so resolve after the body starts.
  [string]$OutputDir = '',
  # Reuse the existing node_modules / dist / bluntcode.exe instead of running
  # build.ps1, which starts with `npm ci` and deletes all of node_modules.
  # Only correct when the tree is already built and verified; if anything is
  # stale, drop this switch and do a full build.
  [switch]$SkipBuild,
  # Ed25519 private key from `go run ./cmd/releasesign keygen`. Signs the
  # .sha256 asset into .sha256.sig for the in-app updater. Required once
  # internal/selfupdate/signing_key.go pins a public key.
  [string]$SigningKeyFile = ''
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
if (-not $OutputDir) { $OutputDir = Join-Path $root 'dist' }
if (-not $SkipBuild) {
  & (Join-Path $PSScriptRoot 'build.ps1')
  if ($LASTEXITCODE -ne 0) { throw "Build failed with exit code $LASTEXITCODE" }
}

$output = [IO.Path]::GetFullPath($OutputDir)

# The zip must never ship a binary whose reported version disagrees with the
# release label: v0.23.0 was packaged with -SkipBuild over a stale root exe and
# shipped a 0.21.2 binary, so every in-app update "succeeded" while the app
# kept offering 0.23.0. Refuse to package on mismatch.
$packagedExe = Join-Path $root 'bluntcode.exe'
if (-not (Test-Path -LiteralPath $packagedExe)) {
  throw "bluntcode.exe not found at $packagedExe - build first (drop -SkipBuild, or build in the worktree)."
}
$versionLine = (& $packagedExe '--version' 2>$null | Select-Object -First 1)
if ("$versionLine" -notmatch "(\d+\.\d+\.\d+)" -or $Matches[1] -ne $Version) {
  throw "Version mismatch: bluntcode.exe reports '$versionLine' but packaging $Version. The exe is stale - rebuild from the exact tagged commit."
}

# Size budget check: ensure binary was stripped (-ldflags="-s -w") and fits under 18 MB budget
$exeSize = (Get-Item -LiteralPath $packagedExe).Length
if ($exeSize -gt 18MB) {
  throw "Binary size budget exceeded: bluntcode.exe is $([math]::Round($exeSize/1MB, 2)) MB (budget <= 18 MB). Rebuild with -ldflags='-s -w'."
}

$releaseName = "BluntCode-$Version-windows-amd64"
$payload = Join-Path $output $releaseName
$archive = Join-Path $output "$releaseName.zip"
if (Test-Path -LiteralPath $payload) { Remove-Item -LiteralPath $payload -Recurse -Force }
foreach ($stale in @($archive, "$archive.sha256", "$archive.sha256.sig")) {
  if (Test-Path -LiteralPath $stale) { Remove-Item -LiteralPath $stale -Force }
}
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

# The in-app updater refuses a release whose .sha256 is not signed by the key
# compiled into the running binary, so never publish one without the other.
$keySource = Get-Content -LiteralPath (Join-Path $root 'internal\selfupdate\signing_key.go') -Raw
$pinnedKey = if ($keySource -match 'ReleaseSigningKey = "([^"]*)"') { $Matches[1] } else { '' }
if ($SigningKeyFile) {
  # Resolve before Push-Location so a relative path means the caller's cwd.
  $SigningKeyFile = (Resolve-Path -LiteralPath $SigningKeyFile).Path
  Push-Location $root
  try {
    go run ./cmd/releasesign sign -key $SigningKeyFile -in "$archive.sha256" -out "$archive.sha256.sig"
    if ($LASTEXITCODE -ne 0) { throw "Signing $archive.sha256 failed with exit code $LASTEXITCODE" }
    if ($pinnedKey) {
      go run ./cmd/releasesign verify -pub $pinnedKey -in "$archive.sha256" -sig "$archive.sha256.sig"
      if ($LASTEXITCODE -ne 0) { throw "The signing key does not match ReleaseSigningKey in internal/selfupdate/signing_key.go." }
    }
  } finally {
    Pop-Location
  }
  Write-Host "Signature: $archive.sha256.sig"
} elseif ($pinnedKey) {
  throw "internal/selfupdate/signing_key.go pins a release signing key; pass -SigningKeyFile or in-app updates will reject this release."
}
Write-Host "Package: $archive"
Write-Host "SHA256:  $hash"
Write-Host "Installer: $(Join-Path $output 'install-latest.ps1')"
