[CmdletBinding()]
param([string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'BluntCode'))

$ErrorActionPreference = 'Stop'
$install = [IO.Path]::GetFullPath($InstallDir)
$local = [IO.Path]::GetFullPath($env:LOCALAPPDATA)
if ($install -eq $local -or $install -eq [IO.Path]::GetPathRoot($install)) {
  throw "Refusing unsafe uninstall directory: $install"
}

$current = [Environment]::GetEnvironmentVariable('Path', 'User')
$parts = @($current -split ';' | Where-Object { $_ -and $_ -ne $install })
[Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')
if (Test-Path -LiteralPath $install) { Remove-Item -LiteralPath $install -Recurse -Force }
Write-Host "Removed Blunt Code from $install"
