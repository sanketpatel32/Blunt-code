[CmdletBinding()]
param(
  [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\BluntCode'),
  [switch]$RemoveData
)

$ErrorActionPreference = 'Stop'
$install = [IO.Path]::GetFullPath($InstallDir)
$local = [IO.Path]::GetFullPath($env:LOCALAPPDATA)
if ($install -eq $local -or $install -eq [IO.Path]::GetPathRoot($install)) {
  throw "Refusing unsafe uninstall directory: $install"
}

if (Get-Process -Name 'bluntcode' -ErrorAction SilentlyContinue) {
  throw 'Close Blunt Code before uninstalling.'
}

# The one-line installer creates this shortcut; a leftover .lnk would point at
# nothing once the install directory below is gone.
$shortcut = Join-Path ([Environment]::GetFolderPath('Programs')) 'Blunt Code.lnk'
if (Test-Path -LiteralPath $shortcut) { Remove-Item -LiteralPath $shortcut -Force }

$current = [Environment]::GetEnvironmentVariable('Path', 'User')
$parts = @($current -split ';' | Where-Object { $_ -and $_ -ne $install })
[Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')
if (Test-Path -LiteralPath $install) { Remove-Item -LiteralPath $install -Recurse -Force }
if ($RemoveData) {
  $data = Join-Path $env:LOCALAPPDATA 'BluntCode'
  if (Test-Path -LiteralPath $data) { Remove-Item -LiteralPath $data -Recurse -Force }
}
Write-Host "Removed Blunt Code from $install"
