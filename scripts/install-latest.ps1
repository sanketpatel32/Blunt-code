[CmdletBinding()]
param(
  [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\BluntCode'),
  [switch]$NoLaunch
)

$ErrorActionPreference = 'Stop'
# Piped execution (irm | iex): silence download progress and ensure TLS 1.2 on
# older Windows builds whose .NET defaults predate it. Our own meter below
# replaces PowerShell's native progress UI, which slows transfers down badly.
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

function Write-Step([int]$Number, [int]$Total, [string]$Text) {
  Write-Host ''
  Write-Host "[$Number/$Total] $Text" -ForegroundColor Cyan
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

Write-Step 1 5 'Fetching the latest release info...'
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
$version = "$($release.tag_name)".TrimStart('v')
if (-not $version) { $version = 'latest' }

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

  Write-Step 4 5 "Installing to $install"
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

  Write-Step 5 5 'Creating the Start-menu shortcut...'
  $executable = Join-Path $install 'bluntcode.exe'
  try {
    New-StartMenuShortcut -Executable $executable -WorkingDirectory $install
  } catch {
    Write-Warning "Installed Blunt Code, but could not create its Start-menu shortcut: $($_.Exception.Message)"
  }

  Write-Host ''
  Write-Host "Installed Blunt Code $version to $install" -ForegroundColor Green
  Write-Host 'Your reports and settings stay in %LOCALAPPDATA%\BluntCode.'
  if (-not $NoLaunch) {
    Write-Host 'Launching Blunt Code...'
    Start-Process -FilePath $executable -WorkingDirectory $install
  }
} finally {
  if (Test-Path -LiteralPath $temporary) {
    Remove-Item -LiteralPath $temporary -Recurse -Force
  }
}
