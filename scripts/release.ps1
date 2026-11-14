#requires -Version 7.3
param(
    [ValidateSet('windows','linux','darwin')][string]$TargetOS,
    [ValidateSet('amd64','arm64')][string]$TargetArch,
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$')][string]$Version = 'dev'
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true

function Write-Checksums([string]$Directory) {
    $checksums = foreach ($file in Get-ChildItem -LiteralPath $Directory -File -Recurse | Sort-Object FullName) {
        $relative = [IO.Path]::GetRelativePath($Directory,$file.FullName).Replace('\','/')
        '{0}  {1}' -f (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant(),$relative
    }
    $checksums | Set-Content -Encoding utf8NoBOM (Join-Path $Directory 'SHA256SUMS')
}

function Write-ArchiveChecksum([string]$Archive) {
    '{0}  {1}' -f (Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash.ToLowerInvariant(),[IO.Path]::GetFileName($Archive) |
        Set-Content -Encoding utf8NoBOM "$Archive.sha256"
}

function New-ReleaseArchive([string]$Directory, [string]$Archive, [string]$Executable = '') {
    if (Test-Path -LiteralPath $Archive) { throw "Archive already exists: $Archive" }
    if ($Archive.EndsWith('.zip')) {
        [IO.Compression.ZipFile]::CreateFromDirectory($Directory,$Archive,[IO.Compression.CompressionLevel]::Optimal,$false)
    } else {
        # Cross builds from Windows still need an executable Unix binary.
        $fileStream = [IO.File]::Create($Archive)
        $gzip = [IO.Compression.GZipStream]::new($fileStream,[IO.Compression.CompressionLevel]::Optimal)
        $tar = [System.Formats.Tar.TarWriter]::new($gzip)
        try {
            foreach ($file in Get-ChildItem -LiteralPath $Directory -File -Recurse | Sort-Object FullName) {
                $relative = [IO.Path]::GetRelativePath($Directory,$file.FullName).Replace('\','/')
                $entry = [System.Formats.Tar.PaxTarEntry]::new([System.Formats.Tar.TarEntryType]::RegularFile,$relative)
                $entry.Mode = [IO.UnixFileMode]$(if ($relative -eq $Executable) {493} else {420})
                $entry.DataStream = [IO.File]::OpenRead($file.FullName)
                try { $tar.WriteEntry($entry) } finally { $entry.DataStream.Dispose() }
            }
        } finally { $tar.Dispose(); $gzip.Dispose(); $fileStream.Dispose() }
    }
    Write-ArchiveChecksum $Archive
}

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
    $releaseDir = Join-Path $repo "dist/$Version"
    $artifactName = "tinyrill-$Version-$TargetOS-$TargetArch"
    $target = Join-Path $releaseDir $artifactName
    $verification = Join-Path $releaseDir "$artifactName-verification"
    $extension = if ($TargetOS -eq 'windows') { '.zip' } else { '.tar.gz' }
    $archive = "$target$extension"
    $verificationArchive = "$verification$extension"
    foreach ($outputPath in @($target,$verification,$archive,$verificationArchive)) {
        if (Test-Path -LiteralPath $outputPath) { throw "Output already exists: $outputPath. Use a new version or move the incomplete build aside." }
    }
    New-Item -ItemType Directory -Force $releaseDir | Out-Null
    # One source archive per version, shared by every platform. Never mix commits.
    $sourceArchive = Join-Path $releaseDir "tinyrill-$Version-source.tar.gz"
    $releaseRecord = Join-Path $releaseDir 'release.json'
    if (Test-Path -LiteralPath $releaseRecord) {
        $record = Get-Content -LiteralPath $releaseRecord -Raw | ConvertFrom-Json
        if ($record.version -ne $Version -or $record.source_commit -ne $revision -or
            $record.source_archive -ne [IO.Path]::GetFileName($sourceArchive) -or
            !(Test-Path -LiteralPath $sourceArchive) -or
            (Get-FileHash -LiteralPath $sourceArchive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $record.source_sha256) {
            throw 'This version already belongs to a different commit or its source archive is damaged.'
        }
    } else {
        if (Test-Path -LiteralPath $sourceArchive) { throw 'Source archive exists without a release record.' }
        git archive --format=tar.gz --prefix=tinyrill-source/ "--output=$sourceArchive" HEAD
        $record = [ordered]@{schema_version=1; version=$Version; source_commit=$revision; source_archive=[IO.Path]::GetFileName($sourceArchive); source_sha256=(Get-FileHash -LiteralPath $sourceArchive -Algorithm SHA256).Hash.ToLowerInvariant()}
        $record | ConvertTo-Json | Set-Content -Encoding utf8NoBOM $releaseRecord
        Write-ArchiveChecksum $sourceArchive
    }
    $toolsDir = Join-Path $repo "_build/tools/$hostOS-$hostArch"
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
    New-Item -ItemType Directory $verification | Out-Null
    $binaryName = if ($TargetOS -eq 'windows') { 'tinyrill.exe' } else { 'tinyrill' }
    $binary = Join-Path $target $binaryName
    go mod verify
    go build -trimpath -buildvcs=false -ldflags "-s -w -buildid= -X main.Version=$Version -X main.Commit=$revision" -o $binary .
    go version -m $binary | Set-Content -Encoding utf8NoBOM (Join-Path $verification 'build-info.txt')
    & $vulnTool -json ./... | Set-Content -Encoding utf8NoBOM (Join-Path $verification 'vulnerabilities-source.json')
    # JSON mode always exits successfully on findings. Convert the saved report
    # through the text handler so reachable vulnerabilities fail the release.
    Get-Content -LiteralPath (Join-Path $verification 'vulnerabilities-source.json') -Raw |
        & $vulnTool -mode=convert | Set-Content -Encoding utf8NoBOM (Join-Path $verification 'vulnerabilities-source.txt')
    & $vulnTool -mode=binary -json $binary | Set-Content -Encoding utf8NoBOM (Join-Path $verification 'vulnerabilities-binary.json')
    Get-Content -LiteralPath (Join-Path $verification 'vulnerabilities-binary.json') -Raw |
        & $vulnTool -mode=convert | Set-Content -Encoding utf8NoBOM (Join-Path $verification 'vulnerabilities-binary.txt')
    & $sbomTool app -json -licenses -std -main . -output (Join-Path $verification 'sbom.cdx.json') .
    # Ship original dependency notices as well as machine-readable license evidence.
    $licensesDir = Join-Path $verification 'licenses'
    New-Item -ItemType Directory $licensesDir | Out-Null
    $goroot = (go env GOROOT).Trim()
    [IO.File]::WriteAllBytes((Join-Path $licensesDir 'Go-LICENSE'),[IO.File]::ReadAllBytes((Join-Path $goroot 'LICENSE')))
    # This retained TCP probe was copied into the repo, so go list cannot discover its notice.
    $embeddedNoticeDir = Join-Path $licensesDir 'embedded/internal/ping'
    New-Item -ItemType Directory -Force $embeddedNoticeDir | Out-Null
    [IO.File]::WriteAllBytes((Join-Path $embeddedNoticeDir 'LICENSE'),[IO.File]::ReadAllBytes((Join-Path $repo 'internal/ping/LICENSE')))
    $moduleLines = go list -deps -f '{{with .Module}}{{.Path}}|{{.Version}}|{{.Main}}|{{if .Replace}}{{.Replace.Path}}|{{.Replace.Version}}|{{.Replace.Dir}}{{else}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}{{end}}' .
    $licenseIndex = foreach ($line in ($moduleLines | Where-Object { $_ } | Sort-Object -Unique)) {
        # Module identities cannot contain |; the last field retains any | in a local directory.
        $fields = $line.Split('|',6)
        if ($fields.Count -ne 6) { throw 'Unexpected Go module metadata' }
        if ($fields[2] -eq 'true') { continue }
        $actual = [pscustomobject]@{Path=$fields[3]; Version=$fields[4]; Dir=$fields[5]}
        if (!$actual.Dir) { continue }
        $notices = @(Get-ChildItem -LiteralPath $actual.Dir -File | Where-Object { $_.Name -match '^(LICEN[CS]E|COPYING|NOTICE|COPYRIGHT)([.-].*)?$' })
        if (!$notices.Count) { throw "No license material found for $($actual.Path)@$($actual.Version)" }
        $label = ($actual.Path + '@' + $actual.Version).Replace('/','_')
        $noticeDir = Join-Path $licensesDir $label
        New-Item -ItemType Directory -Force $noticeDir | Out-Null
        foreach ($notice in $notices) {
            [IO.File]::WriteAllBytes((Join-Path $noticeDir $notice.Name),[IO.File]::ReadAllBytes($notice.FullName))
        }
        [ordered]@{module=$fields[0]; source=$actual.Path; version=$actual.Version; directory=$label}
    }
    ConvertTo-Json -InputObject @($licenseIndex) -Depth 4 | Set-Content -Encoding utf8NoBOM (Join-Path $licensesDir 'index.json')
    # Keep the exact notice bytes, with source labels, in one runtime document.
    $noticesStream = [IO.File]::Create((Join-Path $target 'THIRD_PARTY_NOTICES.txt'))
    try {
        foreach ($notice in Get-ChildItem -LiteralPath $licensesDir -Recurse -File | Where-Object Name -ne 'index.json' | Sort-Object FullName) {
            $label = [IO.Path]::GetRelativePath($licensesDir,$notice.FullName).Replace('\','/')
            $header = [Text.Encoding]::UTF8.GetBytes("===== $label =====`n")
            $contents = [IO.File]::ReadAllBytes($notice.FullName)
            $newline = [Text.Encoding]::UTF8.GetBytes("`n`n")
            $noticesStream.Write($header,0,$header.Length)
            $noticesStream.Write($contents,0,$contents.Length)
            $noticesStream.Write($newline,0,$newline.Length)
        }
    } finally { $noticesStream.Dispose() }
    Copy-Item -LiteralPath LICENSE -Destination $target
    $runtimeExamples = Join-Path $target 'examples'
    New-Item -ItemType Directory $runtimeExamples | Out-Null
    Copy-Item -LiteralPath examples/customer-a-ec.yaml,examples/customer-c-atrust.yaml -Destination $runtimeExamples
    $quickstart = (Get-Content docs/runtime-readme.md -Raw).Replace('@VERSION@',$Version).Replace('@PLATFORM@',"$TargetOS/$TargetArch").Replace('@COMMIT@',$revision).Replace('@SOURCE_ARCHIVE@',[IO.Path]::GetFileName($sourceArchive)).Replace('@VERIFICATION_ARCHIVE@',[IO.Path]::GetFileName($verificationArchive))
    $quickstart | Set-Content -Encoding utf8NoBOM (Join-Path $target 'README.md')
    Copy-Item -LiteralPath LICENSE,THIRD_PARTY.md,docker-compose.yml -Destination $verification
    Copy-Item -LiteralPath examples,docs -Destination $verification -Recurse
    Copy-Item -LiteralPath docs/verification-readme.md -Destination (Join-Path $verification 'README.md')
    $acceptanceScripts = Join-Path $verification 'scripts'
    New-Item -ItemType Directory $acceptanceScripts | Out-Null
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'soak.ps1') -Destination $acceptanceScripts
    Write-Checksums $target
    New-ReleaseArchive $target $archive $binaryName
    $provenance = [ordered]@{
        schema_version = 2; version = $Version; source_commit = $revision
        source_repository = 'https://github.com/WhiteGreeny/tinyrill'
        upstream_baseline = '8576e7a16076c4175d6dbe276573a353cf7260be'
        go = $goVersion; goos = $TargetOS; goarch = $TargetArch; cgo = $env:CGO_ENABLED
        govulncheck = 'v1.9.0'; cyclonedx_gomod = 'v1.12.0'; created_utc = [DateTime]::UtcNow.ToString('o')
        compatibility = 'candidate; see docs/acceptance.md'; signed = $false
        binary = $binaryName; binary_sha256 = (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
        runtime_archive = [IO.Path]::GetFileName($archive); runtime_sha256 = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
        source_archive = $record.source_archive; source_sha256 = $record.source_sha256
    }
    $provenance | ConvertTo-Json | Set-Content -Encoding utf8NoBOM (Join-Path $verification 'provenance.json')
    Write-Checksums $verification
    New-ReleaseArchive $verification $verificationArchive
    Write-Output "Runtime: $archive"
    Write-Output "Verification: $verificationArchive"
    Write-Output "Corresponding source: $sourceArchive"
} finally {
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name,$previous[$name]) }
    Pop-Location
}
