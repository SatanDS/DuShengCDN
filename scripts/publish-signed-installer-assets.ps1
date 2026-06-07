param(
  [Parameter(Mandatory = $true)]
  [ValidateSet("install-commercial.sh", "install-agent.sh", "install-dns-worker.sh")]
  [string[]]$Asset,

  [string]$Repo = "SatanDS/SatanDS-DuShengCDN-releases",
  [string]$Tag = "v1.0.0",
  [string]$SourceDir = "",
  [string]$DistDir = "",
  [string]$ReleaseSignaturePublicKey = $env:DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY,
  [string]$ReleaseSigningPrivateKey = $env:DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY,
  [string]$GitHubToken = $env:GH_TOKEN,
  [string]$SecretsFile = "",
  [switch]$Upload,
  [switch]$Force
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
  $DistDir = Join-Path $SourceDir "dist\manual-installer-assets"
}

if (-not [string]::IsNullOrWhiteSpace($SecretsFile)) {
  $secretPath = (Resolve-Path -LiteralPath $SecretsFile).Path
  if ($secretPath.StartsWith($SourceDir, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "SecretsFile must be outside the source repository: $SourceDir"
  }
  $secretJson = Get-Content -Raw -Encoding UTF8 -LiteralPath $secretPath | ConvertFrom-Json
  if ([string]::IsNullOrWhiteSpace($ReleaseSignaturePublicKey) -and $secretJson.release_signature_public_key) {
    $ReleaseSignaturePublicKey = [string]$secretJson.release_signature_public_key
  }
  if ([string]::IsNullOrWhiteSpace($ReleaseSigningPrivateKey) -and $secretJson.release_signing_private_key) {
    $ReleaseSigningPrivateKey = [string]$secretJson.release_signing_private_key
  }
  if ([string]::IsNullOrWhiteSpace($GitHubToken) -and $secretJson.github_token) {
    $GitHubToken = [string]$secretJson.github_token
  }
  if ($secretJson.license_issuer_private_key) {
    throw "license_issuer_private_key must not be provided to release signing secrets."
  }
}

function Fail($Message) {
  throw $Message
}

function Require-Secret($Value, $Name) {
  if ([string]::IsNullOrWhiteSpace($Value)) {
    Fail "$Name is required. Pass it as a parameter or set the matching environment variable."
  }
}

function Get-Sha256Hex($Path) {
  return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
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
    Fail "$Name must be base64"
  }
  if ($bytes.Length -ne $ExpectedLength) {
    Fail "$Name must decode to $ExpectedLength bytes"
  }
  return $bytes
}

function Assert-ReleaseSignature($AssetName, $Checksum, $SignaturePath) {
  $signatureText = (Get-Content -Raw -Encoding UTF8 -LiteralPath $SignaturePath).Trim()
  [void](Read-Base64Bytes $ReleaseSignaturePublicKey "DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY" 32)
  [void](Read-Base64Bytes $signatureText "$AssetName signature" 64)

  $verifyProgram = @'
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func decode(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	out, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		out, err = base64.RawStdEncoding.DecodeString(value)
	}
	return out, err
}

func main() {
	if len(os.Args) != 6 {
		fmt.Fprintln(os.Stderr, "usage: verify public-key tag asset checksum signature")
		os.Exit(2)
	}
	publicKey, err := decode(os.Args[1])
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		fmt.Fprintln(os.Stderr, "invalid public key")
		os.Exit(1)
	}
	signature, err := decode(os.Args[5])
	if err != nil || len(signature) != ed25519.SignatureSize {
		fmt.Fprintln(os.Stderr, "invalid signature")
		os.Exit(1)
	}
	payload := []byte(strings.Join([]string{
		"dushengcdn-release-v1",
		strings.TrimSpace(os.Args[2]),
		strings.TrimSpace(os.Args[3]),
		strings.ToLower(strings.TrimSpace(os.Args[4])),
		"",
	}, "\n"))
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		fmt.Fprintln(os.Stderr, "signature verification failed")
		os.Exit(1)
	}
}
'@

  $verifyFile = Join-Path ([System.IO.Path]::GetTempPath()) ("dushengcdn-release-verify-" + [guid]::NewGuid().ToString() + ".go")
  [System.IO.File]::WriteAllText($verifyFile, $verifyProgram, (New-Object System.Text.UTF8Encoding($false)))
  try {
    & go run $verifyFile $ReleaseSignaturePublicKey $Tag $AssetName $Checksum $signatureText
    if ($LASTEXITCODE -ne 0) {
      Fail "generated signature verification failed for $AssetName"
    }
  } finally {
    Remove-Item -LiteralPath $verifyFile -Force -ErrorAction SilentlyContinue
  }
  if ($LASTEXITCODE -ne 0) {
    Fail "generated signature verification failed for $AssetName"
  }
}

function Invoke-GitHubJson($Uri, $Method = "GET") {
  $headers = @{
    Authorization          = "Bearer $GitHubToken"
    Accept                 = "application/vnd.github+json"
    "X-GitHub-Api-Version" = "2022-11-28"
    "User-Agent"           = "dushengcdn-manual-release-tool"
  }
  return Invoke-RestMethod -Method $Method -Uri $Uri -Headers $headers
}

Require-Secret $ReleaseSignaturePublicKey "DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY"
Require-Secret $ReleaseSigningPrivateKey "DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY"
if ($Upload) {
  Require-Secret $GitHubToken "GH_TOKEN"
}

$signScript = Join-Path $SourceDir "scripts\sign-release-asset.go"
if (-not (Test-Path -LiteralPath $signScript)) {
  Fail "sign-release-asset.go was not found: $signScript"
}

New-Item -ItemType Directory -Force -Path $DistDir | Out-Null

$generated = @()
foreach ($assetName in $Asset) {
  $sourcePath = Join-Path $SourceDir "scripts\$assetName"
  if (-not (Test-Path -LiteralPath $sourcePath)) {
    Fail "source installer was not found: $sourcePath"
  }

  $assetPath = Join-Path $DistDir $assetName
  $shaPath = "$assetPath.sha256"
  $sigPath = "$assetPath.sig"

  $content = Get-Content -Raw -Encoding UTF8 -LiteralPath $sourcePath
  $content = $content.Replace("__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY__", $ReleaseSignaturePublicKey)
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($assetPath, $content, $utf8NoBom)

  $checksum = Get-Sha256Hex $assetPath
  [System.IO.File]::WriteAllText($shaPath, "$checksum  $assetName`n", $utf8NoBom)

  & go run $signScript `
    -private-key $ReleaseSigningPrivateKey `
    -tag $Tag `
    -asset $assetName `
    -checksum-file $shaPath `
    -signature-file $sigPath
  if ($LASTEXITCODE -ne 0) {
    Fail "failed to sign $assetName"
  }
  Assert-ReleaseSignature $assetName $checksum $sigPath

  $generated += [pscustomobject]@{
    Asset = $assetName
    Path = $assetPath
    Sha256 = $shaPath
    Signature = $sigPath
    Checksum = $checksum
  }
}

$generated | Format-Table -AutoSize

if (-not $Upload) {
  Write-Host "Generated signed installer assets in $DistDir"
  Write-Host "Rerun with -Upload -Force to replace matching assets in $Repo@$Tag."
  exit 0
}

if (-not $Force) {
  Fail "Refusing to upload without -Force."
}

$release = Invoke-GitHubJson "https://api.github.com/repos/$Repo/releases/tags/$Tag"
$uploadBase = $release.upload_url -replace "\{.*$", ""

foreach ($item in $generated) {
  foreach ($path in @($item.Path, $item.Sha256, $item.Signature)) {
    $name = [System.IO.Path]::GetFileName($path)
    $existing = @($release.assets | Where-Object { $_.name -eq $name })
    foreach ($assetInfo in $existing) {
      Write-Host "Deleting existing asset $name ($($assetInfo.id))"
      Invoke-GitHubJson "https://api.github.com/repos/$Repo/releases/assets/$($assetInfo.id)" "DELETE" | Out-Null
    }

    $uploadUri = "${uploadBase}?name=$([System.Uri]::EscapeDataString($name))"
    Write-Host "Uploading $name"
    $headers = @{
      Authorization          = "Bearer $GitHubToken"
      Accept                 = "application/vnd.github+json"
      "X-GitHub-Api-Version" = "2022-11-28"
      "User-Agent"           = "dushengcdn-manual-release-tool"
      "Content-Type"         = "application/octet-stream"
    }
    Invoke-RestMethod -Method POST -Uri $uploadUri -Headers $headers -InFile $path | Out-Null
  }
}

Write-Host "Uploaded signed installer assets to $Repo@$Tag."
