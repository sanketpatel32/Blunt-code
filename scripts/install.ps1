[CmdletBinding()]
param(
  [string]$PackagePath,
  [string]$PackageUrl,
  [string]$Sha256,
  [string]$Sha256Url,
  [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\BluntCode'),
  [switch]$AddToPath
)

$ErrorActionPreference = 'Stop'

function Assert-SafeInstallDir([string]$Path) {
  $full = [IO.Path]::GetFullPath($Path)
  $local = [IO.Path]::GetFullPath($env:LOCALAPPDATA)
  if ($full -eq $local -or $full -eq [IO.Path]::GetPathRoot($full)) {
    throw "Refusing unsafe install directory: $full"
  }
  return $full
}

function Assert-HttpsUrl([string]$Url) {
  $uri = [Uri]$Url
  if ($uri.Scheme -ne 'https') { throw 'Download URLs must use HTTPS.' }
  return $uri
}

if ([string]::IsNullOrWhiteSpace($PackagePath) -eq [string]::IsNullOrWhiteSpace($PackageUrl)) {
  throw 'Provide exactly one of -PackagePath or -PackageUrl.'
}
if ([string]::IsNullOrWhiteSpace($Sha256) -and [string]::IsNullOrWhiteSpace($Sha256Url)) {
  throw 'Provide -Sha256 or -Sha256Url.'
}

$download = $null
try {
  if ($PackageUrl) {
    $url = Assert-HttpsUrl $PackageUrl
    $download = Join-Path ([IO.Path]::GetTempPath()) ('bluntcode-' + [guid]::NewGuid() + '.zip')
    Invoke-WebRequest -Uri $url -OutFile $download -UseBasicParsing
    $package = $download
  } else {
    $package = (Resolve-Path -LiteralPath $PackagePath).Path
  }
  if (-not $Sha256 -and $Sha256Url) {
    $checksumUri = Assert-HttpsUrl $Sha256Url
    $checksumText = (Invoke-WebRequest -Uri $checksumUri -UseBasicParsing).Content.Trim()
    if ($checksumText -notmatch '^(?<hash>[a-fA-F0-9]{64})(\s+.+)?$') { throw 'Checksum response is invalid.' }
    $Sha256 = $Matches.hash
  }

$install = Assert-SafeInstallDir $InstallDir
$actual = (Get-FileHash -LiteralPath $package -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $Sha256.ToLowerInvariant()) { throw 'Package checksum does not match.' }

$parent = Split-Path -Parent $install
New-Item -ItemType Directory -Force -Path $parent | Out-Null
$staging = Join-Path $parent ('.bluntcode-staging-' + [guid]::NewGuid())
$backup = Join-Path $parent ('.bluntcode-backup-' + [guid]::NewGuid())
try {
  Expand-Archive -LiteralPath $package -DestinationPath $staging -Force
  $payload = Get-ChildItem -LiteralPath $staging -Directory | Select-Object -First 1
  if ($null -eq $payload -or -not (Test-Path (Join-Path $payload.FullName 'bluntcode.exe'))) {
    throw 'Package does not contain a Blunt Code executable.'
  }
  if (Test-Path $install) { Move-Item -LiteralPath $install -Destination $backup }
  Move-Item -LiteralPath $payload.FullName -Destination $install
  if (Test-Path $backup) { Remove-Item -LiteralPath $backup -Recurse -Force }
} catch {
  if (-not (Test-Path $install) -and (Test-Path $backup)) { Move-Item -LiteralPath $backup -Destination $install }
  throw
} finally {
  if (Test-Path $staging) { Remove-Item -LiteralPath $staging -Recurse -Force }
}

if ($AddToPath) {
  $current = [Environment]::GetEnvironmentVariable('Path', 'User')
  $parts = @($current -split ';' | Where-Object { $_ })
  if ($parts -notcontains $install) {
    [Environment]::SetEnvironmentVariable('Path', (($parts + $install) -join ';'), 'User')
  }
}

Write-Host "Installed Blunt Code to $install"
} finally {
  if ($download -and (Test-Path $download)) { Remove-Item -LiteralPath $download -Force }
}
