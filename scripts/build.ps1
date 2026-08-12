$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$web = Join-Path $root 'web'
$static = Join-Path $root 'cmd\bluntcode\static'

Push-Location $web
try {
  npm ci
  if ($LASTEXITCODE -ne 0) { throw "npm ci failed with exit code $LASTEXITCODE" }
  npm run build
  if ($LASTEXITCODE -ne 0) { throw "npm run build failed with exit code $LASTEXITCODE" }
} finally {
  Pop-Location
}

if (Test-Path $static) { Remove-Item -LiteralPath $static -Recurse -Force }
New-Item -ItemType Directory -Path $static -Force | Out-Null
Copy-Item -Path (Join-Path $web 'dist\*') -Destination $static -Recurse -Force

Push-Location $root
try {
  go build -o bluntcode.exe ./cmd/bluntcode
  if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
} finally {
  Pop-Location
}
