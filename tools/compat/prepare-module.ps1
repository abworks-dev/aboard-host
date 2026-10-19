# Dot-sourced by tools/go.ps1. Maintained input: one fingerprinted replacement.
# Generated, rebuildable module source and modfile live only in ignored .data.
function New-AboardCompatModfile([string]$Root) {
    $patch = Get-Content -Raw -LiteralPath (Join-Path $Root 'tools/compat/libc-windows-flush.json') | ConvertFrom-Json
    $module = [System.IO.File]::ReadAllText((Join-Path $Root 'go.mod'))
    if ($module -notmatch '(?m)^\s*modernc\.org/libc v1\.77\.1\s*$' -or
        $module -notmatch '(?m)^\s*modernc\.org/sqlite v1\.60\.1\s*$' -or
        $module -match '(?m)^replace\b') {
        throw 'Compatibility patch requires the frozen pair and no competing replacement'
    }
    if ($patch.module -ne 'modernc.org/libc' -or $patch.version -ne 'v1.77.1' -or
        $patch.sqliteVersion -ne 'v1.60.1' -or $patch.source -ne 'libc_windows.go') {
        throw 'Unexpected compatibility patch target'
    }
    $originalRoot = Join-Path $env:GOMODCACHE 'modernc.org/libc@v1.77.1'
    $source = Join-Path $originalRoot $patch.source
    if ((Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash.ToLowerInvariant() -ne $patch.sourceSHA256) {
        throw 'Dependency source fingerprint mismatch; refusing compatibility patch'
    }
    $text = [System.IO.File]::ReadAllText($source)
    if ([string]::IsNullOrEmpty($patch.old) -or [string]::IsNullOrEmpty($patch.new) -or
        [regex]::Matches($text, [regex]::Escape($patch.old)).Count -ne 1) {
        throw 'Compatibility patch must match exactly once'
    }
    $encoding = [System.Text.UTF8Encoding]::new($false)
    $patchedBytes = $encoding.GetBytes($text.Replace($patch.old, $patch.new))
    $expectedPatchHash = [Convert]::ToHexString([System.Security.Cryptography.SHA256]::HashData($patchedBytes))
    $directory = Join-Path $Root '.data/cache/compat-build/libc-v1.77.1'
    # Go 1.27 forbids overlays under GOMODCACHE. A private source snapshot is
    # generated instead; this is not a checked-in vendor tree or maintained fork.
    $files = @(Get-ChildItem -LiteralPath $originalRoot -Recurse -Force -File)
    foreach ($file in $files) {
        $relative = [System.IO.Path]::GetRelativePath($originalRoot, $file.FullName)
        $destination = Join-Path $directory $relative
        $isPatch = $relative -eq $patch.source
        $expected = if ($isPatch) { $expectedPatchHash } else { (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash }
        if (Test-Path -LiteralPath $destination) {
            if ((Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash -ne $expected) {
                throw "Generated compatibility source differs from fixed inputs: $relative"
            }
        } else {
            New-Item -ItemType Directory -Force -Path (Split-Path -Parent $destination) | Out-Null
            [byte[]]$bytes = @()
            if ($isPatch) { $bytes = $patchedBytes } else { $bytes = [System.IO.File]::ReadAllBytes($file.FullName) }
            $stream = [System.IO.File]::Open($destination, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
            try { $stream.Write($bytes, 0, $bytes.Length) } finally { $stream.Dispose() }
        }
    }
    if (@(Get-ChildItem -LiteralPath $directory -Force -Recurse -File).Count -ne $files.Count) {
        throw 'Unexpected extra files in generated compatibility source'
    }
    $buildRoot = Split-Path -Parent $directory
    $modfile = Join-Path $buildRoot 'build.mod'
    $quotedPath = $directory | ConvertTo-Json -Compress
    [System.IO.File]::WriteAllText($modfile, ($module + "`nreplace modernc.org/libc v1.77.1 => $quotedPath`n"), $encoding)
    [System.IO.File]::WriteAllText((Join-Path $buildRoot 'build.sum'), [System.IO.File]::ReadAllText((Join-Path $Root 'go.sum')), $encoding)
    return $modfile
}
