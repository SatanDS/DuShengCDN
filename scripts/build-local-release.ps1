param(
  [string]$Version = "v1.0.0",
  [string]$SourceDir = "",
  [string]$DistDir = "",
  [string]$SecretsFile = "",
  [string]$ReleaseSignaturePublicKey = $env:DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY,
  [string]$ReleaseSigningPrivateKey = $env:DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY,
  [string]$ReleaseSigningPrivateKeyFile = $env:DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY_FILE,
  [string]$CommercialLicensePublicKeys = $env:DUSHENGCDN_LICENSE_PUBLIC_KEYS,
  [string]$GoBin = "",
  [string]$GarbleBin = "",
  [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"

$scriptRoot = if ($PSScriptRoot) {
  $PSScriptRoot
} else {
  Split-Path -Parent $MyInvocation.MyCommand.Path
}
if ([string]::IsNullOrWhiteSpace($SourceDir)) {
  $SourceDir = (Resolve-Path (Join-Path $scriptRoot "..")).Path
} else {
  $SourceDir = (Resolve-Path $SourceDir).Path
}
if ([string]::IsNullOrWhiteSpace($DistDir)) {
  $DistDir = Join-Path $SourceDir ("dist\local-release-" + $Version)
}

function Fail($Message) {
  throw $Message
}

function Read-SecretFile($Path, $Name) {
  if ([string]::IsNullOrWhiteSpace($Path)) {
    return ""
  }
  $resolved = (Resolve-Path -LiteralPath $Path).Path
  if ($resolved.StartsWith($SourceDir, [System.StringComparison]::OrdinalIgnoreCase)) {
    Fail "$Name file must be outside the source repository: $SourceDir"
  }
  return (Get-Content -Raw -Encoding UTF8 -LiteralPath $resolved).Trim()
}

function Protect-SecretFile($Path) {
  if ($PSVersionTable.PSEdition -eq "Core" -and -not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
    chmod 600 $Path 2>$null
    return
  }
  try {
    $acl = Get-Acl -LiteralPath $Path
    $acl.SetAccessRuleProtection($true, $false)
    $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
    $rule = New-Object System.Security.AccessControl.FileSystemAccessRule($identity, "FullControl", "Allow")
    $acl.SetAccessRule($rule)
    Set-Acl -LiteralPath $Path -AclObject $acl
  } catch {
    Write-Warning "Could not restrict temporary release signing key ACL: $($_.Exception.Message)"
  }
}

if (-not [string]::IsNullOrWhiteSpace($SecretsFile)) {
  $secretPath = (Resolve-Path -LiteralPath $SecretsFile).Path
  if ($secretPath.StartsWith($SourceDir, [System.StringComparison]::OrdinalIgnoreCase)) {
    Fail "SecretsFile must be outside the source repository: $SourceDir"
  }
  $secretJson = Get-Content -Raw -Encoding UTF8 -LiteralPath $secretPath | ConvertFrom-Json
  if ([string]::IsNullOrWhiteSpace($ReleaseSignaturePublicKey) -and $secretJson.release_signature_public_key) {
    $ReleaseSignaturePublicKey = [string]$secretJson.release_signature_public_key
  }
  if ([string]::IsNullOrWhiteSpace($ReleaseSigningPrivateKey) -and $secretJson.release_signing_private_key) {
    $ReleaseSigningPrivateKey = [string]$secretJson.release_signing_private_key
  }
  if ([string]::IsNullOrWhiteSpace($CommercialLicensePublicKeys) -and $secretJson.license_public_keys) {
    $CommercialLicensePublicKeys = [string]$secretJson.license_public_keys
  }
  if ($secretJson.license_issuer_private_key) {
    Fail "license_issuer_private_key must not be provided to release build secrets. Use license_public_keys only."
  }
}

if (-not [string]::IsNullOrWhiteSpace($ReleaseSigningPrivateKey) -and -not [string]::IsNullOrWhiteSpace($ReleaseSigningPrivateKeyFile)) {
  Fail "Use only one of DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY or DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY_FILE."
}
if ([string]::IsNullOrWhiteSpace($ReleaseSigningPrivateKey) -and -not [string]::IsNullOrWhiteSpace($ReleaseSigningPrivateKeyFile)) {
  $ReleaseSigningPrivateKey = Read-SecretFile $ReleaseSigningPrivateKeyFile "DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY"
}

function Require-Secret($Value, $Name) {
  if ([string]::IsNullOrWhiteSpace($Value)) {
    Fail "$Name is required."
  }
}

function Convert-Base64UrlToBase64($Value) {
  $normalized = $Value.Trim().Replace("-", "+").Replace("_", "/")
  switch ($normalized.Length % 4) {
    2 { $normalized += "==" }
    3 { $normalized += "=" }
    1 { Fail "invalid base64 value length" }
  }
  return $normalized
}

function Read-Base64Bytes($Value, $Name, $ExpectedLength) {
  try {
    $bytes = [Convert]::FromBase64String((Convert-Base64UrlToBase64 $Value))
  } catch {
    Fail "$Name must be base64 or base64url"
  }
  if ($bytes.Length -ne $ExpectedLength) {
    Fail "$Name must decode to $ExpectedLength bytes"
  }
  return $bytes
}

function Read-Base64BytesOneOf($Value, $Name, [int[]]$ExpectedLengths) {
  try {
    $bytes = [Convert]::FromBase64String((Convert-Base64UrlToBase64 $Value))
  } catch {
    Fail "$Name must be base64 or base64url"
  }
  if ($ExpectedLengths -notcontains $bytes.Length) {
    Fail "$Name must decode to one of these byte lengths: $($ExpectedLengths -join ', ')"
  }
  return $bytes
}

function Convert-HexToBytes($Value, $Name) {
  $hex = $Value.Trim()
  if (($hex.Length % 2) -ne 0 -or $hex -notmatch '^[0-9a-fA-F]+$') {
    Fail "$Name must be hex encoded"
  }
  $bytes = New-Object byte[] ($hex.Length / 2)
  for ($i = 0; $i -lt $bytes.Length; $i++) {
    $bytes[$i] = [Convert]::ToByte($hex.Substring($i * 2, 2), 16)
  }
  return $bytes
}

function Normalize-LicensePublicKeys($Value) {
  $fields = @($Value -split '[,;\s]+' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
  if ($fields.Count -eq 0) {
    Fail "DUSHENGCDN_LICENSE_PUBLIC_KEYS is required."
  }
  foreach ($field in $fields) {
    try {
      $decoded = [Convert]::FromBase64String((Convert-Base64UrlToBase64 $field))
    } catch {
      try {
        $decoded = Convert-HexToBytes $field "DUSHENGCDN_LICENSE_PUBLIC_KEYS"
      } catch {
        Fail "DUSHENGCDN_LICENSE_PUBLIC_KEYS contains an invalid public key."
      }
    }
    if ($decoded.Length -ne 32) {
      Fail "DUSHENGCDN_LICENSE_PUBLIC_KEYS contains a public key with invalid length."
    }
  }
  return ($fields -join ",")
}

function Get-Sha256Hex($Path) {
  return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Write-ChecksumAndSignature($AssetName) {
  $assetPath = Join-Path $DistDir $AssetName
  $shaPath = "$assetPath.sha256"
  $sigPath = "$assetPath.sig"
  $checksum = Get-Sha256Hex $assetPath
  [System.IO.File]::WriteAllText($shaPath, "$checksum  $AssetName`n", (New-Object System.Text.UTF8Encoding($false)))
  $privateKeyTemp = Join-Path ([System.IO.Path]::GetTempPath()) ("dushengcdn-release-signing-key-" + [guid]::NewGuid().ToString())
  try {
    [System.IO.File]::WriteAllText($privateKeyTemp, $ReleaseSigningPrivateKey, (New-Object System.Text.UTF8Encoding($false)))
    Protect-SecretFile $privateKeyTemp
    $env:GOTOOLCHAIN = "local"
    & $GoBin run (Join-Path $SourceDir "scripts\sign-release-asset.go") `
      -tag $Version `
      -asset $AssetName `
      -checksum-file $shaPath `
      -signature-file $sigPath `
      -private-key-file $privateKeyTemp
  } finally {
    Remove-Item Env:GOTOOLCHAIN -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $privateKeyTemp -Force -ErrorAction SilentlyContinue
  }
  if ($LASTEXITCODE -ne 0) {
    Fail "failed to sign $AssetName"
  }
}

function Invoke-CommandChecked($Title, [scriptblock]$Block) {
  Write-Host "==> $Title"
  & $Block
  if ($LASTEXITCODE -ne 0) {
    Fail "$Title failed"
  }
}

function Resolve-CommandPath($Configured, $Name) {
  if (-not [string]::IsNullOrWhiteSpace($Configured)) {
    return (Resolve-Path -LiteralPath $Configured).Path
  }
  $cmd = Get-Command $Name -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $cmd) {
    Fail "$Name was not found in PATH."
  }
  return $cmd.Source
}

Require-Secret $ReleaseSignaturePublicKey "DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY"
Require-Secret $ReleaseSigningPrivateKey "DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY"
[void](Read-Base64Bytes $ReleaseSignaturePublicKey "DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY" 32)
[void](Read-Base64BytesOneOf $ReleaseSigningPrivateKey "DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY" @(32, 64))
Require-Secret $CommercialLicensePublicKeys "DUSHENGCDN_LICENSE_PUBLIC_KEYS"
$CommercialLicensePublicKeys = Normalize-LicensePublicKeys $CommercialLicensePublicKeys
$GoBin = Resolve-CommandPath $GoBin "go"
$GarbleBin = Resolve-CommandPath $GarbleBin "garble"

New-Item -ItemType Directory -Force -Path $DistDir | Out-Null

if (-not $SkipFrontend) {
  Invoke-CommandChecked "Build frontend" {
    Push-Location (Join-Path $SourceDir "dushengcdn_server\web")
    try {
      corepack pnpm install --frozen-lockfile
      $env:NEXT_PUBLIC_APP_VERSION = $Version
      corepack pnpm build
    } finally {
      Remove-Item Env:NEXT_PUBLIC_APP_VERSION -ErrorAction SilentlyContinue
      Pop-Location
    }
  }
}

$buildWatermark = "SatanDS/DuShengCDN:$Version:local:$(git -C $SourceDir rev-parse --short HEAD 2>$null)"

$serverTargets = @(
  @{ GOOS = "linux"; GOARCH = "amd64"; Asset = "dushengcdn-server-linux-amd64" },
  @{ GOOS = "linux"; GOARCH = "arm64"; Asset = "dushengcdn-server-linux-arm64" },
  @{ GOOS = "darwin"; GOARCH = "amd64"; Asset = "dushengcdn-server-darwin-amd64" },
  @{ GOOS = "darwin"; GOARCH = "arm64"; Asset = "dushengcdn-server-darwin-arm64" },
  @{ GOOS = "windows"; GOARCH = "amd64"; Asset = "dushengcdn-server-windows-amd64.exe" }
)

foreach ($target in $serverTargets) {
  $asset = $target.Asset
  Invoke-CommandChecked "Build $asset" {
    Push-Location (Join-Path $SourceDir "dushengcdn_server")
    try {
      $env:CGO_ENABLED = "0"
      $env:GOOS = $target.GOOS
      $env:GOARCH = $target.GOARCH
      $env:GOTOOLCHAIN = "local"
      & $GarbleBin -literals build -trimpath -buildvcs=false `
        -ldflags "-s -w -X 'dushengcdn/common.Version=$Version' -X 'dushengcdn/common.ReleaseSignaturePublicKey=$ReleaseSignaturePublicKey' -X 'dushengcdn/common.CommercialBuildMode=required-online' -X 'dushengcdn/common.CommercialBuildWatermark=$buildWatermark' -X 'dushengcdn/common.CommercialLicensePublicKeys=$CommercialLicensePublicKeys'" `
        -o (Join-Path $DistDir $asset) .
    } finally {
      Remove-Item Env:CGO_ENABLED,Env:GOOS,Env:GOARCH,Env:GOTOOLCHAIN -ErrorAction SilentlyContinue
      Pop-Location
    }
  }
  Write-ChecksumAndSignature $asset
}

$agentTargets = @(
  @{ GOOS = "linux"; GOARCH = "amd64"; Asset = "dushengcdn-agent-linux-amd64" },
  @{ GOOS = "linux"; GOARCH = "arm64"; Asset = "dushengcdn-agent-linux-arm64" },
  @{ GOOS = "darwin"; GOARCH = "amd64"; Asset = "dushengcdn-agent-darwin-amd64" },
  @{ GOOS = "darwin"; GOARCH = "arm64"; Asset = "dushengcdn-agent-darwin-arm64" }
)

foreach ($target in $agentTargets) {
  $asset = $target.Asset
  Invoke-CommandChecked "Build $asset" {
    Push-Location (Join-Path $SourceDir "dushengcdn_agent")
    try {
      $env:CGO_ENABLED = "0"
      $env:GOOS = $target.GOOS
      $env:GOARCH = $target.GOARCH
      $env:GOTOOLCHAIN = "local"
      & $GarbleBin -literals build -trimpath -buildvcs=false `
        -ldflags "-s -w -X 'dushengcdn-agent/internal/config.AgentVersion=$Version' -X 'dushengcdn-agent/internal/config.ReleaseSignaturePublicKey=$ReleaseSignaturePublicKey'" `
        -o (Join-Path $DistDir $asset) ./cmd/agent
    } finally {
      Remove-Item Env:CGO_ENABLED,Env:GOOS,Env:GOARCH,Env:GOTOOLCHAIN -ErrorAction SilentlyContinue
      Pop-Location
    }
  }
  Write-ChecksumAndSignature $asset
}

$dnsTargets = @(
  @{ GOOS = "linux"; GOARCH = "amd64"; Asset = "dushengcdn-dns-worker-linux-amd64" },
  @{ GOOS = "linux"; GOARCH = "arm64"; Asset = "dushengcdn-dns-worker-linux-arm64" },
  @{ GOOS = "darwin"; GOARCH = "amd64"; Asset = "dushengcdn-dns-worker-darwin-amd64" },
  @{ GOOS = "darwin"; GOARCH = "arm64"; Asset = "dushengcdn-dns-worker-darwin-arm64" },
  @{ GOOS = "windows"; GOARCH = "amd64"; Asset = "dushengcdn-dns-worker-windows-amd64.exe" }
)

foreach ($target in $dnsTargets) {
  $asset = $target.Asset
  Invoke-CommandChecked "Build $asset" {
    Push-Location (Join-Path $SourceDir "dushengcdn_server")
    try {
      $env:CGO_ENABLED = "0"
      $env:GOOS = $target.GOOS
      $env:GOARCH = $target.GOARCH
      $env:GOTOOLCHAIN = "local"
      & $GarbleBin -literals build -trimpath -buildvcs=false `
        -ldflags "-s -w -X 'main.version=$Version'" `
        -o (Join-Path $DistDir $asset) ./cmd/dns-worker
    } finally {
      Remove-Item Env:CGO_ENABLED,Env:GOOS,Env:GOARCH,Env:GOTOOLCHAIN -ErrorAction SilentlyContinue
      Pop-Location
    }
  }
  Write-ChecksumAndSignature $asset
}

foreach ($installer in @("install-commercial.sh", "install-agent.sh", "install-dns-worker.sh")) {
  $source = Join-Path $SourceDir "scripts\$installer"
  $dest = Join-Path $DistDir $installer
  $content = Get-Content -Raw -Encoding UTF8 -LiteralPath $source
  $content = $content.Replace("__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY__", $ReleaseSignaturePublicKey)
  [System.IO.File]::WriteAllText($dest, $content, (New-Object System.Text.UTF8Encoding($false)))
  Write-ChecksumAndSignature $installer
}

Write-Host "Local release assets built in $DistDir"
