$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
$env:GOPATH = "$PWD\.cache\gopath"
$env:GOMODCACHE = "$PWD\.cache\gomod"
$env:GOCACHE = "$PWD\.cache\gobuild"
if (!(Test-Path -LiteralPath '.env.dev')) { go run ./cmd/setup -dev -out .env.dev; if ($LASTEXITCODE -ne 0) { throw 'Development setup failed' } }
Get-Content -LiteralPath '.env.dev' | ForEach-Object { if ($_ -match '^([A-Z_]+)=(.*)$') { [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process') } }
$env:CERTREE_DB = 'data/dev.db'
Push-Location apps/web
npm.cmd ci --cache "$PSScriptRoot\..\.cache\npm"
if ($LASTEXITCODE -ne 0) { throw 'Dependency installation failed' }
npm.cmd run build
if ($LASTEXITCODE -ne 0) { throw 'Web build failed' }
Pop-Location
go run ./apps/server
