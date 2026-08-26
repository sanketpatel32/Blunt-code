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

$running = Get-Process -Name 'bluntcode' -ErrorAction SilentlyContinue
if ($running) {
  Write-Host 'Blunt Code is running — closing it for uninstall...' -ForegroundColor Yellow
  foreach ($p in $running) { try { $null = $p.CloseMainWindow() } catch {} }
  $waited = 0
  while ($running -and $waited -lt 5) { Start-Sleep -Seconds 1; $waited += 1; $running = Get-Process -Name 'bluntcode' -ErrorAction SilentlyContinue }
  if ($running) { foreach ($p in $running) { try { Stop-Process -InputObject $p -Force -ErrorAction SilentlyContinue } catch {} }; Start-Sleep -Seconds 1; $running = Get-Process -Name 'bluntcode' -ErrorAction SilentlyContinue }
}
if ($running) { throw 'Close Blunt Code before uninstalling. Still running — Task Manager -> End task bluntcode.exe.' }

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
