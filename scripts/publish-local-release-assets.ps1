param(
  [Parameter(Mandatory = $true)]
  [string]$DistDir,

  [string]$Repo = "SatanDS/SatanDS-DuShengCDN-releases",
  [string]$Tag = "v1.0.0",
  [string]$GitHubToken = $env:GH_TOKEN,
  [string]$SecretsFile = "",
  [switch]$Force,
  [switch]$PruneReleaseAssets
)

$ErrorActionPreference = "Stop"

$scriptRoot = if ($PSScriptRoot) {
  $PSScriptRoot
} else {
  Split-Path -Parent $MyInvocation.MyCommand.Path
}
$sourceDir = (Resolve-Path (Join-Path $scriptRoot "..")).Path
$distPath = (Resolve-Path -LiteralPath $DistDir).Path

function Fail($Message) {
  throw $Message
}

if (-not [string]::IsNullOrWhiteSpace($SecretsFile)) {
  $secretPath = (Resolve-Path -LiteralPath $SecretsFile).Path
  if ($secretPath.StartsWith($sourceDir, [System.StringComparison]::OrdinalIgnoreCase)) {
    Fail "SecretsFile must be outside the source repository: $sourceDir"
  }
  $secretJson = Get-Content -Raw -Encoding UTF8 -LiteralPath $secretPath | ConvertFrom-Json
  if ([string]::IsNullOrWhiteSpace($GitHubToken) -and $secretJson.github_token) {
    $GitHubToken = [string]$secretJson.github_token
  }
  if ($secretJson.license_issuer_private_key) {
    Fail "license_issuer_private_key must not be provided to release publishing secrets."
  }
}

function Require-Secret($Value, $Name) {
  if ([string]::IsNullOrWhiteSpace($Value)) {
    Fail "$Name is required. Pass it as a parameter or set the matching environment variable."
  }
}

function New-GitHubHeaders([string]$Accept = "application/vnd.github+json", [string]$ContentType = "") {
  $headers = @{
    Authorization          = "Bearer $GitHubToken"
    Accept                 = $Accept
    "X-GitHub-Api-Version" = "2022-11-28"
    "User-Agent"           = "dushengcdn-local-release-publish"
  }
  if (-not [string]::IsNullOrWhiteSpace($ContentType)) {
    $headers["Content-Type"] = $ContentType
  }
  return $headers
}

function Invoke-GitHubJson($Uri, $Method = "GET") {
  return Invoke-RestMethod -Method $Method -Uri $Uri -Headers (New-GitHubHeaders)
}

$expectedBaseAssets = @(
  "dushengcdn-server-linux-amd64",
  "dushengcdn-server-linux-arm64",
  "dushengcdn-server-darwin-amd64",
  "dushengcdn-server-darwin-arm64",
  "dushengcdn-server-windows-amd64.exe",
  "dushengcdn-agent-linux-amd64",
  "dushengcdn-agent-linux-arm64",
  "dushengcdn-agent-darwin-amd64",
  "dushengcdn-agent-darwin-arm64",
  "dushengcdn-dns-worker-linux-amd64",
  "dushengcdn-dns-worker-linux-arm64",
  "dushengcdn-dns-worker-darwin-amd64",
  "dushengcdn-dns-worker-darwin-arm64",
  "dushengcdn-dns-worker-windows-amd64.exe",
  "install-commercial.sh",
  "install-agent.sh",
  "install-dns-worker.sh"
)

$expectedUploadNames = @{}
foreach ($base in $expectedBaseAssets) {
  foreach ($name in @($base, "$base.sha256", "$base.sig")) {
    $expectedUploadNames[$name] = $true
  }
}

if (-not $Force) {
  Fail "Refusing to publish without -Force."
}
Require-Secret $GitHubToken "GH_TOKEN"

$files = @(Get-ChildItem -LiteralPath $distPath -File | Sort-Object Name)
if ($files.Count -eq 0) {
  Fail "No files found in DistDir: $distPath"
}
foreach ($name in $expectedUploadNames.Keys) {
  if (-not (Test-Path -LiteralPath (Join-Path $distPath $name))) {
    Fail "DistDir is missing required release asset: $name"
  }
}
foreach ($file in $files) {
  if (-not $expectedUploadNames.ContainsKey($file.Name)) {
    Fail "DistDir contains unexpected file: $($file.Name)"
  }
}

$expectedNames = @{}
foreach ($file in $files) {
  $expectedNames[$file.Name] = $true
}

$release = Invoke-GitHubJson "https://api.github.com/repos/$Repo/releases/tags/$Tag"
$uploadBase = $release.upload_url -replace "\{.*$", ""

$assetsToDelete = New-Object System.Collections.Generic.List[object]
foreach ($asset in $release.assets) {
  if ($expectedNames.ContainsKey($asset.name) -or $PruneReleaseAssets) {
    $assetsToDelete.Add($asset)
  }
}

foreach ($asset in $assetsToDelete) {
  Write-Host "Deleting existing asset $($asset.name) ($($asset.id))"
  Invoke-GitHubJson "https://api.github.com/repos/$Repo/releases/assets/$($asset.id)" "DELETE" | Out-Null
}

foreach ($file in $files) {
  $uploadUri = "${uploadBase}?name=$([System.Uri]::EscapeDataString($file.Name))"
  Write-Host "Uploading $($file.Name)"
  Invoke-RestMethod `
    -Method POST `
    -Uri $uploadUri `
    -Headers (New-GitHubHeaders "application/vnd.github+json" "application/octet-stream") `
    -InFile $file.FullName | Out-Null
}

Write-Host "Uploaded $($files.Count) release assets to $Repo@$Tag."
