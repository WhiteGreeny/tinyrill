#requires -Version 7.3
param(
    [Parameter(Mandatory)][string]$TargetsFile,
    [ValidateRange(0.001,168)][double]$DurationHours = 8,
    [ValidateRange(1,3600)][int]$IntervalSeconds = 30,
    [string]$OutputFile = 'local/soak-results.jsonl'
)
$ErrorActionPreference = 'Stop'
$targets = @(Get-Content -LiteralPath $TargetsFile -Raw | ConvertFrom-Json)
if ($targets.Count -lt 1 -or $targets.Count -gt 16) { throw 'Provide 1..16 customer probes.' }
foreach ($target in $targets) {
    $uri = [Uri]$target.url
    if ($uri.Scheme -notin @('http','https') -or $uri.UserInfo -or $uri.Query -or $uri.Fragment) { throw 'Use an HTTP(S) read-only health URL without credentials, query or fragment.' }
    $proxy = [Uri]$target.proxy
    if ($proxy.Scheme -notin @('http','socks5h') -or !$proxy.IsLoopback -or $proxy.UserInfo) { throw 'Proxy must be HTTP or SOCKS5h on loopback without credentials.' }
    if ($target.customer -notmatch '^[a-zA-Z0-9._-]{1,64}$') { throw 'Use a non-secret customer identifier.' }
    if (!$target.expected_status) { $target | Add-Member expected_status 200 }
}
$destination = [IO.Path]::GetFullPath($OutputFile)
$parent = Split-Path $destination -Parent
New-Item -ItemType Directory -Force $parent | Out-Null
$curl = (Get-Command curl -CommandType Application | Select-Object -First 1).Source
$discard = if ($IsWindows) { 'NUL' } else { '/dev/null' }
$deadline = [DateTime]::UtcNow.AddHours($DurationHours)
$failures = 0
while ([DateTime]::UtcNow -lt $deadline) {
    foreach ($target in $targets) {
        # -q disables ~/.curlrc; explicit proxy plus empty no-proxy prevents bypass.
        $result = & $curl -q --silent --output $discard --connect-timeout 10 --max-time 20 --proxy $target.proxy --noproxy '' --write-out '%{http_code} %{time_total}' --url $target.url
        $exitCode = $LASTEXITCODE
        $parts = ([string]$result).Trim().Split(' ')
        $ok = $exitCode -eq 0 -and $parts[0] -eq [string]$target.expected_status
        if (!$ok) { $failures++ }
        $row = [ordered]@{at=[DateTime]::UtcNow.ToString('o');customer=$target.customer;ok=$ok;curl_exit=$exitCode;http_status=$parts[0];seconds=$parts[-1]}
        if ($target.pid) {
            $process = Get-Process -Id $target.pid -ErrorAction SilentlyContinue
            if ($process) { $row.rss_bytes=$process.WorkingSet64; $row.cpu_seconds=$process.CPU }
            else { $row.process_missing=$true }
        }
        $row | ConvertTo-Json -Compress | Add-Content -Encoding utf8NoBOM -LiteralPath $destination
        if (!$ok) { Write-Warning "Probe failed for $($target.customer), curl exit $exitCode / HTTP $($parts[0])" }
    }
    if ([DateTime]::UtcNow -lt $deadline) { Start-Sleep -Seconds $IntervalSeconds }
}
Write-Output "Probe run finished; failures=$failures; results=$destination"
if ($failures -gt 0) { exit 1 }