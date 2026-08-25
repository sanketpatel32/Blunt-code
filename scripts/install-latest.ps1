[CmdletBinding()]
param(
  [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\BluntCode'),
  [switch]$NoLaunch
)

$ErrorActionPreference = 'Stop'
# Piped execution (irm | iex): silence download progress and ensure TLS 1.2 on
# older Windows builds whose .NET defaults predate it.
$ProgressPreference = 'SilentlyContinue'
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch { }
$repository = 'sanketpatel32/Blunt-code'

function Assert-SafeInstallDir([string]$Path) {
  $full = [IO.Path]::GetFullPath($Path)
  $local = [IO.Path]::GetFullPath($env:LOCALAPPDATA)
  if ($full -eq $local -or $full -eq [IO.Path]::GetPathRoot($full)) {
    throw "Refusing unsafe install directory: $full"
  }
  return $full
}

function New-StartMenuShortcut([string]$Executable, [string]$WorkingDirectory) {
  $programs = [Environment]::GetFolderPath('Programs')
  $shortcutPath = Join-Path $programs 'Blunt Code.lnk'
  $shell = New-Object -ComObject WScript.Shell
  $shortcut = $shell.CreateShortcut($shortcutPath)
  $shortcut.TargetPath = $Executable
  $shortcut.WorkingDirectory = $WorkingDirectory
  $shortcut.IconLocation = "$Executable,0"
  $shortcut.Save()
}

if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
  throw 'LOCALAPPDATA is not available on this Windows account.'
}

$install = Assert-SafeInstallDir $InstallDir
$headers = @{ Accept = 'application/vnd.github+json'; 'User-Agent' = 'BluntCode-Installer' }
$release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repository/releases/latest" -Headers $headers
$archive = @($release.assets | Where-Object { $_.name -match '^BluntCode-.+-windows-amd64\.zip$' }) | Select-Object -First 1
if ($null -eq $archive) {
  throw 'The latest release does not contain a Windows amd64 ZIP.'
}
$checksum = @($release.assets | Where-Object { $_.name -eq "$($archive.name).sha256" }) | Select-Object -First 1
if ($null -eq $checksum) {
  throw "The latest release is missing $($archive.name).sha256."
}

$temporary = Join-Path ([IO.Path]::GetTempPath()) ('.bluntcode-install-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $temporary -Force | Out-Null
try {
  $archivePath = Join-Path $temporary $archive.name
  $checksumPath = Join-Path $temporary $checksum.name
  Write-Host 'Downloading Blunt Code…'
  Invoke-WebRequest -Uri $archive.browser_download_url -OutFile $archivePath -Headers $headers
  Invoke-WebRequest -Uri $checksum.browser_download_url -OutFile $checksumPath -Headers $headers

  $expected = (Get-Content -LiteralPath $checksumPath -Raw).Trim().Split()[0].ToLowerInvariant()
  if ($expected -notmatch '^[a-f0-9]{64}$') {
    throw 'The release checksum is invalid.'
  }
  $actual = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $expected) {
    throw 'Download checksum mismatch. Nothing was installed.'
  }

  $running = Get-Process -Name 'bluntcode' -ErrorAction SilentlyContinue
  if ($running) {
    throw 'Close Blunt Code before installing an update.'
  }

  $staging = Join-Path $temporary 'staging'
  Expand-Archive -LiteralPath $archivePath -DestinationPath $staging -Force
  $payloads = @(Get-ChildItem -LiteralPath $staging -Directory)
  $payload = @($payloads | Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName 'bluntcode.exe') }) | Select-Object -First 1
  if ($null -eq $payload -or $payloads.Count -ne 1) {
    throw 'The release archive does not contain one valid Blunt Code application folder.'
  }

  $parent = Split-Path -Parent $install
  New-Item -ItemType Directory -Path $parent -Force | Out-Null
  $backup = Join-Path $parent ('.bluntcode-backup-' + [guid]::NewGuid())
  try {
    if (Test-Path -LiteralPath $install) {
      Move-Item -LiteralPath $install -Destination $backup
    }
    Move-Item -LiteralPath $payload.FullName -Destination $install
    if (Test-Path -LiteralPath $backup) {
      Remove-Item -LiteralPath $backup -Recurse -Force
    }
  } catch {
    if (-not (Test-Path -LiteralPath $install) -and (Test-Path -LiteralPath $backup)) {
      Move-Item -LiteralPath $backup -Destination $install
    }
    throw
  }

  $executable = Join-Path $install 'bluntcode.exe'
  try {
    New-StartMenuShortcut -Executable $executable -WorkingDirectory $install
  } catch {
    Write-Warning "Installed Blunt Code, but could not create its Start-menu shortcut: $($_.Exception.Message)"
  }
  Write-Host "Installed Blunt Code to $install"
  Write-Host 'Your reports and settings stay in %LOCALAPPDATA%\BluntCode.'
  if (-not $NoLaunch) {
    Start-Process -FilePath $executable -WorkingDirectory $install
  }
} finally {
  if (Test-Path -LiteralPath $temporary) {
    Remove-Item -LiteralPath $temporary -Recurse -Force
  }
}
