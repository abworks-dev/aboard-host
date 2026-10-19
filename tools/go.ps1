param([Parameter(Mandatory=$true)][string[]]$GoArgs)

# Reproducible project-local tools/caches. Does not install or change global Go.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$env:GOENV = 'off'
$env:GOTELEMETRY = 'off'
$env:GOWORK = 'off'
$env:GOFLAGS = ''
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
$env:GOTOOLCHAIN = 'go1.27.2'
$env:GOPATH = Join-Path $root '.data/go'
$env:GOMODCACHE = Join-Path $root '.data/cache/go-mod'
$env:GOCACHE = Join-Path $root '.data/cache/go-build'
$env:TEMP = Join-Path $root '.data/logs/001-validation/tmp'
$env:TMP = $env:TEMP
$env:GOTMPDIR = $env:TEMP
New-Item -ItemType Directory -Force -Path $env:TEMP | Out-Null
if ((Get-Location).Path -ne $root) { throw 'Run tools/go.ps1 from the verified project root' }
$compatLease = $null
try {
if ($GoArgs[0] -in @('build','test','vet','run','list')) {
    $buildCache = Join-Path $root '.data/cache/compat-build'
    New-Item -ItemType Directory -Force -Path $buildCache | Out-Null
    # A build-input lease, not a business/workspace lock. Concurrent preparation
    # fails explicitly instead of rewriting inputs while another compiler reads.
    $compatLease = [System.IO.File]::Open((Join-Path $buildCache 'prepare.lock'), [System.IO.FileMode]::OpenOrCreate, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
    if ($GoArgs | Where-Object { $_ -match '^--?(overlay|modfile|mod)(=|$)' }) {
        throw 'Do not override the required, fingerprinted compatibility build inputs'
    }
    . (Join-Path $PSScriptRoot 'compat/prepare-module.ps1')
    $modfile = New-AboardCompatModfile $root
    $GoArgs = @($GoArgs[0], '-modfile', $modfile, '-mod=readonly') + @($GoArgs | Select-Object -Skip 1)
}
& go @GoArgs
$goExit = $LASTEXITCODE
} finally {
    if ($null -ne $compatLease) { $compatLease.Dispose() }
}
if ($goExit -ne 0) { exit $goExit }
