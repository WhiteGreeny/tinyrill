#requires -Version 7.3
param(
    [ValidateSet('windows','linux','darwin')][string]$TargetOS,
    [ValidateSet('amd64','arm64')][string]$TargetArch,
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$')][string]$Version = 'dev'
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$repo = Split-Path $PSScriptRoot -Parent
Push-Location $repo
$names = @('GOOS','GOARCH','CGO_ENABLED','GOBIN','GOTOOLCHAIN')
$previous = @{}
foreach ($name in $names) { $previous[$name] = [Environment]::GetEnvironmentVariable($name) }
try {
    if (git status --porcelain) { throw 'Release builds require a clean Git checkout, including the build scripts.' }
    $revision = (git rev-parse HEAD).Trim()
    $goVersion = (Get-Content .go-version -Raw).Trim()
    $env:GOTOOLCHAIN = 'go' + $goVersion
    $hostOS = (go env GOHOSTOS).Trim()
    $hostArch = (go env GOHOSTARCH).Trim()
    if (!$TargetOS) { $TargetOS = $hostOS }
    if (!$TargetArch) { $TargetArch = $hostArch }
    if ($TargetOS -eq 'darwin' -and $hostOS -ne 'darwin') { throw 'macOS releases require a native macOS builder with CGO/Keychain.' }
    $target = Join-Path $repo "dist/tinyrill-$TargetOS-$TargetArch"
    if (Test-Path -LiteralPath $target) { throw "Output already exists: $target. Move it aside before rebuilding." }
    $toolsDir = Join-Path $repo 'dist/tools'
    New-Item -ItemType Directory -Force $toolsDir | Out-Null
    $env:GOOS = $hostOS
    $env:GOARCH = $hostArch
    $env:CGO_ENABLED = '0'
    $env:GOBIN = $toolsDir
    go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
    go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1.12.0
    $suffix = if ($hostOS -eq 'windows') { '.exe' } else { '' }
    $vulnTool = Join-Path $toolsDir "govulncheck$suffix"
    $sbomTool = Join-Path $toolsDir "cyclonedx-gomod$suffix"
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = if ($TargetOS -eq 'darwin') { '1' } else { '0' }
    New-Item -ItemType Directory $target | Out-Null
    $binaryName = if ($TargetOS -eq 'windows') { 'tinyrill.exe' } else { 'tinyrill' }
    $binary = Join-Path $target $binaryName
    go mod verify
    go build -trimpath -buildvcs=false -ldflags "-s -w -buildid= -X main.Version=$Version -X main.Commit=$revision" -o $binary .
    go version -m $binary | Set-Content -Encoding utf8NoBOM (Join-Path $target 'build-info.txt')
    & $vulnTool -json ./... | Set-Content -Encoding utf8NoBOM (Join-Path $target 'vulnerabilities-source.json')
    & $vulnTool -mode=binary -json $binary | Set-Content -Encoding utf8NoBOM (Join-Path $target 'vulnerabilities-binary.json')
    & $sbomTool app -json -licenses -std -main . -output (Join-Path $target 'sbom.cdx.json') .
    # Ship original dependency notices as well as machine-readable license evidence.
    $licensesDir = Join-Path $target 'licenses'
    New-Item -ItemType Directory $licensesDir | Out-Null
    $goroot = (go env GOROOT).Trim()
    Copy-Item -LiteralPath (Join-Path $goroot 'LICENSE') -Destination (Join-Path $licensesDir 'Go-LICENSE')
    $licenseIndex = foreach ($line in (go list -m -f '{{json .}}' all)) {
        $module = $line | ConvertFrom-Json
        if ($module.Main) { continue }
        $actual = if ($module.Replace) { $module.Replace } else { $module }
        if (!$actual.Dir) { continue }
        $notices = @(Get-ChildItem -LiteralPath $actual.Dir -File | Where-Object { $_.Name -match '^(LICEN[CS]E|COPYING|NOTICE|COPYRIGHT)([.-].*)?$' })
        if (!$notices.Count) { throw "No license material found for $($actual.Path)@$($actual.Version)" }
        $label = ($actual.Path + '@' + $actual.Version).Replace('/','_')
        $noticeDir = Join-Path $licensesDir $label
        New-Item -ItemType Directory -Force $noticeDir | Out-Null
        foreach ($notice in $notices) { Copy-Item -LiteralPath $notice.FullName -Destination $noticeDir }
        [ordered]@{module=$module.Path; source=$actual.Path; version=$actual.Version; directory=$label}
    }
    ConvertTo-Json -InputObject @($licenseIndex) -Depth 4 | Set-Content -Encoding utf8NoBOM (Join-Path $licensesDir 'index.json')
    Copy-Item -LiteralPath LICENSE,README.md,THIRD_PARTY.md -Destination $target
    Copy-Item -LiteralPath examples,docs -Destination $target -Recurse
    $sourceArchive = Join-Path $target 'source.tar.gz'
    git archive --format=tar.gz --prefix=tinyrill-source/ "--output=$sourceArchive" HEAD
    $provenance = [ordered]@{
        schema_version = 1; version = $Version; source_commit = $revision
        source_repository = 'https://github.com/WhiteGreeny/tinyrill'
        upstream_baseline = '8576e7a16076c4175d6dbe276573a353cf7260be'
        go = $goVersion; goos = $TargetOS; goarch = $TargetArch; cgo = $env:CGO_ENABLED
        govulncheck = 'v1.9.0'; cyclonedx_gomod = 'v1.12.0'; created_utc = [DateTime]::UtcNow.ToString('o')
        compatibility = 'candidate; see docs/acceptance.md'; signed = $false
    }
    $provenance | ConvertTo-Json | Set-Content -Encoding utf8NoBOM (Join-Path $target 'provenance.json')
    $checksums = foreach ($file in Get-ChildItem -LiteralPath $target -File -Recurse | Sort-Object FullName) {
        $relative = [IO.Path]::GetRelativePath($target,$file.FullName).Replace('\','/')
        '{0}  {1}' -f (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant(),$relative
    }
    $checksums | Set-Content -Encoding utf8NoBOM (Join-Path $target 'SHA256SUMS')
    $archive = if ($TargetOS -eq 'windows') { "$target.zip" } else { "$target.tar.gz" }
    if (Test-Path -LiteralPath $archive) { throw "Archive already exists: $archive" }
    if ($TargetOS -eq 'windows') {
        [IO.Compression.ZipFile]::CreateFromDirectory($target,$archive,[IO.Compression.CompressionLevel]::Optimal,$false)
    } else {
        # Cross builds from Windows still need an executable Unix binary.
        $fileStream = [IO.File]::Create($archive)
        $gzip = [IO.Compression.GZipStream]::new($fileStream,[IO.Compression.CompressionLevel]::Optimal)
        $tar = [System.Formats.Tar.TarWriter]::new($gzip)
        try {
            foreach ($file in Get-ChildItem -LiteralPath $target -File -Recurse | Sort-Object FullName) {
                $relative = [IO.Path]::GetRelativePath($target,$file.FullName).Replace('\','/')
                $entry = [System.Formats.Tar.PaxTarEntry]::new([System.Formats.Tar.TarEntryType]::RegularFile,$relative)
                $entry.Mode = [IO.UnixFileMode]$(if ($relative -eq $binaryName) {493} else {420})
                $entry.DataStream = [IO.File]::OpenRead($file.FullName)
                try { $tar.WriteEntry($entry) } finally { $entry.DataStream.Dispose() }
            }
        } finally { $tar.Dispose(); $gzip.Dispose(); $fileStream.Dispose() }
    }
    '{0}  {1}' -f (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant(),[IO.Path]::GetFileName($archive) |
        Set-Content -Encoding utf8NoBOM "$archive.sha256"
    Write-Output "Verified candidate artifacts: $target"
} finally {
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name,$previous[$name]) }
    Pop-Location
}
