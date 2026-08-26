[CmdletBinding()]
param(
  [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\BluntCode'),
  # Pin a specific release (e.g. -Version 0.6.0). Empty means latest.
  [string]$Version = '',
  # Quiet mode: only errors and the final result line are printed; implies -NoLaunch.
  [switch]$Silent,
  # Additionally create a desktop shortcut next to the Start-menu one.
  [switch]$DesktopShortcut,
  # Print what would be installed and exit without changing anything.
  [switch]$WhatIf,
  # Seconds to wait for a running Blunt Code to exit before refusing. Used by
  # the in-app updater, which closes the app right after launching us.
  [int]$WaitForCloseSeconds = 0,
  [switch]$NoLaunch
)

$ErrorActionPreference = 'Stop'
# Piped execution (irm | iex): silence download progress and ensure TLS 1.2 on
# older Windows builds whose .NET defaults predate it. Our own meter below
# replaces PowerShell's native progress UI, which slows transfers down badly.
$ProgressPreference = 'SilentlyContinue'
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch { }
$repository = 'sanketpatel32/Blunt-code'
$script:Quiet = [bool]$Silent

function Write-Info([string]$Text, [string]$Color = 'White') {
  if (-not $script:Quiet) { Write-Host $Text -ForegroundColor $Color }
}

function Assert-SafeInstallDir([string]$Path) {
  $full = [IO.Path]::GetFullPath($Path)
  $local = [IO.Path]::GetFullPath($env:LOCALAPPDATA)
  if ($full -eq $local -or $full -eq [IO.Path]::GetPathRoot($full)) {
    throw "Refusing unsafe install directory: $full"
  }
  return $full
}

function New-ShellShortcut([string]$Executable, [string]$WorkingDirectory, [string]$ShortcutPath) {
  $shell = New-Object -ComObject WScript.Shell
  $shortcut = $shell.CreateShortcut($ShortcutPath)
  $shortcut.TargetPath = $Executable
  $shortcut.WorkingDirectory = $WorkingDirectory
  $shortcut.IconLocation = "$Executable,0"
  $shortcut.Save()
}

function New-StartMenuShortcut([string]$Executable, [string]$WorkingDirectory) {
  $programs = [Environment]::GetFolderPath('Programs')
  New-ShellShortcut -Executable $Executable -WorkingDirectory $WorkingDirectory -ShortcutPath (Join-Path $programs 'Blunt Code.lnk')
}

function New-DesktopShortcut([string]$Executable, [string]$WorkingDirectory) {
  $desktop = [Environment]::GetFolderPath('Desktop')
  New-ShellShortcut -Executable $Executable -WorkingDirectory $WorkingDirectory -ShortcutPath (Join-Path $desktop 'Blunt Code.lnk')
}

function Write-Step([int]$Number, [int]$Total, [string]$Text) {
  if ($script:Quiet) { return }
  Write-Host ''
  Write-Host "[$Number/$Total] $Text" -ForegroundColor Cyan
}

# Installed-version probe: asks the existing bluntcode.exe directly so an
# upgrade, reinstall, or downgrade is labeled honestly. Any failure reads as
# an unknown previous version rather than blocking the install.
function Get-InstalledVersion([string]$Executable) {
  if (-not (Test-Path -LiteralPath $Executable)) { return '' }
  try {
    $out = (& $Executable '--version' 2>$null | Select-Object -First 1)
    if ($out -match '(\d+\.\d+\.\d+)') { return $Matches[1] }
  } catch { }
  return ''
}

function Compare-Versions([string]$A, [string]$B) {
  try {
    $left = [version]$A
    $right = [version]$B
    return $left.CompareTo($right)
  } catch { return 0 }
}

# Streams $Url to disk behind a single-line progress meter. GitHub serves this
# script as application/octet-stream, so piped sessions decode it as ANSI:
# every literal printed here must stay plain ASCII.
function Save-Download([string]$Url, [string]$OutFile) {
  $request = [Net.HttpWebRequest]::Create($Url)
  $request.UserAgent = 'BluntCode-Installer'
  $response = $request.GetResponse()
  try {
    $net = $response.GetResponseStream()
    $file = [IO.File]::Open($OutFile, [IO.FileMode]::Create)
    try {
      $buffer = New-Object byte[] 65536
      $received = [long]0
      $total = [long]$response.ContentLength
      # Redraw the meter at most twice per megabyte; CI-style redirected
      # output gets one coarse line every few MB instead of carriage returns.
      $drawnAt = [long]0
      $live = -not [Console]::IsOutputRedirected
      while (($read = $net.Read($buffer, 0, $buffer.Length)) -gt 0) {
        $file.Write($buffer, 0, $read)
        $received += $read
        if ($live -and $total -ge 1048576 -and (($received - $drawnAt) -ge 131072 -or $received -eq $total)) {
          $drawnAt = $received
          $percent = [int](100 * $received / $total)
          $meter = ('  {0:N1} / {1:N1} MB ({2}%)' -f ($received / 1MB), ([Math]::Max($total, 1) / 1MB), $percent).PadRight(44)
          [Console]::Write("`r$meter")
        } elseif (-not $live -and ($received - $drawnAt) -ge 4194304) {
          $drawnAt = $received
          Write-Host ('  downloaded {0:N1} MB...' -f ($received / 1MB))
        }
      }
      if ($live) { [Console]::Write("`r" + (' ' * 60) + "`r") }
    } finally {
      $file.Dispose()
    }
  } finally {
    $response.Close()
  }
}

if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
  throw 'LOCALAPPDATA is not available on this Windows account.'
}
$install = Assert-SafeInstallDir $InstallDir

# --- Pre-flight checks -------------------------------------------------------
if (-not [Environment]::Is64BitOperatingSystem) {
  throw 'Blunt Code publishes 64-bit (amd64) builds only; this operating system is 32-bit.'
}
$drive = New-Object IO.DriveInfo((Split-Path -Qualifier $install))
if ($drive.AvailableFreeSpace -lt 300MB) {
  throw ('Not enough free disk space on {0}: {1:N0} MB free, at least 300 MB required.' -f $drive.Name, ($drive.AvailableFreeSpace / 1MB))
}
if ($Silent) { $NoLaunch = $true }

Write-Step 1 5 'Fetching release info...'
$headers = @{ Accept = 'application/vnd.github+json'; 'User-Agent' = 'BluntCode-Installer' }
if ($Version) {
  $releaseEndpoint = "https://api.github.com/repos/$repository/releases/tags/v$Version"
} else {
  $releaseEndpoint = "https://api.github.com/repos/$repository/releases/latest"
}
try {
  $release = Invoke-RestMethod -Uri $releaseEndpoint -Headers $headers
} catch {
  if ($Version) {
    throw "Release v$Version was not found in $repository. Check the version (example: -Version 0.6.0) or drop the flag for the latest."
  }
  throw
}
$archive = @($release.assets | Where-Object { $_.name -match '^BluntCode-.+-windows-amd64\.zip$' }) | Select-Object -First 1
if ($null -eq $archive) {
  throw 'The release does not contain a Windows amd64 ZIP.'
}
$checksum = @($release.assets | Where-Object { $_.name -eq "$($archive.name).sha256" }) | Select-Object -First 1
if ($null -eq $checksum) {
  throw "The release is missing $($archive.name).sha256."
}
$version = "$($release.tag_name)".TrimStart('v')
if (-not $version) { $version = 'latest' }

# --- Upgrade awareness -------------------------------------------------------
$executable = Join-Path $install 'bluntcode.exe'
$installedVersion = Get-InstalledVersion $executable
$actionLabel = 'Installing'
if ($installedVersion) {
  switch (Compare-Versions $installedVersion $version) {
    { $_ -lt 0 } { $actionLabel = "Upgrading $installedVersion ->" ; break }
    { $_ -eq 0 } { $actionLabel = "Reinstalling $installedVersion" ; break }
    default { $actionLabel = "DOWNGRADING $installedVersion ->" }
  }
}

if ($WhatIf) {
  Write-Host 'What-if plan (nothing was changed):'
  Write-Host ("  release      : {0} (tag {1})" -f $archive.name, $release.tag_name)
  Write-Host ("  size         : {0:N1} MB" -f ($archive.size / 1MB))
  Write-Host ("  install dir  : {0}" -f $install)
  Write-Host ("  action       : {0} {1}" -f $actionLabel.TrimEnd(), $version)
  Write-Host ("  shortcuts    : Start-menu{0}, launch after install: {1}" -f ($(if ($DesktopShortcut) { ' + Desktop' } else { '' })), ($(if ($NoLaunch) { 'no' } else { 'yes' })))
  exit 0
}

Write-Step 2 5 "Downloading $($archive.name)"
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('.bluntcode-install-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $temporary -Force | Out-Null
try {
  $archivePath = Join-Path $temporary $archive.name
  $checksumPath = Join-Path $temporary $checksum.name
  Save-Download $archive.browser_download_url $archivePath
  Save-Download $checksum.browser_download_url $checksumPath

  Write-Step 3 5 'Verifying the SHA-256 checksum...'
  $expected = (Get-Content -LiteralPath $checksumPath -Raw).Trim().Split()[0].ToLowerInvariant()
  if ($expected -notmatch '^[a-f0-9]{64}$') {
    throw 'The release checksum is invalid.'
  }
  $actual = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $expected) {
    throw 'Download checksum mismatch. Nothing was installed.'
  }

  Write-Step 4 5 "$actionLabel $version at $install"
  $running = Get-Process -Name 'bluntcode' -ErrorAction SilentlyContinue
  if ($running) {
    $waited = 0
    while ($running -and $waited -lt $WaitForCloseSeconds) {
      Start-Sleep -Seconds 1
      $waited += 1
      $running = Get-Process -Name 'bluntcode' -ErrorAction SilentlyContinue
    }
  }
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
    # Rollback: restore the previous installation whenever the swap left the
    # target directory missing, so an interrupted upgrade never strands the
    # user without a working copy.
    if (-not (Test-Path -LiteralPath $install) -and (Test-Path -LiteralPath $backup)) {
      Move-Item -LiteralPath $backup -Destination $install
      Write-Warning 'Installation failed; your previous Blunt Code copy was restored.'
    }
    throw
  }

  Write-Step 5 5 'Creating shortcuts...'
  try {
    New-StartMenuShortcut -Executable $executable -WorkingDirectory $install
    if ($DesktopShortcut) {
      New-DesktopShortcut -Executable $executable -WorkingDirectory $install
    }
  } catch {
    Write-Warning "Installed Blunt Code, but could not create every shortcut: $($_.Exception.Message)"
  }

  if ($Silent) {
    Write-Output "Installed Blunt Code $version to $install"
  } else {
    Write-Host ''
    Write-Host "Installed Blunt Code $version to $install" -ForegroundColor Green
    Write-Host 'Your reports and settings stay in %LOCALAPPDATA%\BluntCode.'
    if (-not $NoLaunch) {
      Write-Host 'Launching Blunt Code...'
      Start-Process -FilePath $executable -WorkingDirectory $install
    }
  }
} finally {
  if (Test-Path -LiteralPath $temporary) {
    Remove-Item -LiteralPath $temporary -Recurse -Force
  }
}
