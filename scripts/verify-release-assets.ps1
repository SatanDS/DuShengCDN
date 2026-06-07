param(
  [string]$Repo = "SatanDS/SatanDS-DuShengCDN-releases",
  [string]$Tag = "v1.0.0",
  [string]$GitHubToken = $env:GH_TOKEN,
  [string]$LocalDistDir = "",
  [string]$ReleaseSignaturePublicKey = $env:DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY,
  [string]$SecretsFile = "",
  [switch]$VerifySignatures
)

$ErrorActionPreference = "Stop"

$scriptRoot = if ($PSScriptRoot) {
  $PSScriptRoot
} else {
  Split-Path -Parent $MyInvocation.MyCommand.Path
}
$sourceDir = (Resolve-Path (Join-Path $scriptRoot "..")).Path

function Fail($Message) {
  throw $Message
}

if (-not [string]::IsNullOrWhiteSpace($SecretsFile)) {
  $secretPath = (Resolve-Path -LiteralPath $SecretsFile).Path
  if ($secretPath.StartsWith($sourceDir, [System.StringComparison]::OrdinalIgnoreCase)) {
    Fail "SecretsFile must be outside the source repository: $sourceDir"
  }
  $secretJson = Get-Content -Raw -Encoding UTF8 -LiteralPath $secretPath | ConvertFrom-Json
  if ([string]::IsNullOrWhiteSpace($ReleaseSignaturePublicKey) -and $secretJson.release_signature_public_key) {
    $ReleaseSignaturePublicKey = [string]$secretJson.release_signature_public_key
  }
  if ([string]::IsNullOrWhiteSpace($GitHubToken) -and $secretJson.github_token) {
    $GitHubToken = [string]$secretJson.github_token
  }
  if ($secretJson.license_issuer_private_key) {
    Fail "license_issuer_private_key must not be provided to release verification secrets."
  }
}

function New-GitHubHeaders([string]$Accept = "application/vnd.github+json") {
  $headers = @{
    Accept                 = $Accept
    "X-GitHub-Api-Version" = "2022-11-28"
    "User-Agent"           = "dushengcdn-release-verify"
  }
  if (-not [string]::IsNullOrWhiteSpace($GitHubToken)) {
    $headers.Authorization = "Bearer $GitHubToken"
  }
  return $headers
}

function Invoke-GitHubJson($Uri) {
  return Invoke-RestMethod -Method GET -Uri $Uri -Headers (New-GitHubHeaders)
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

function Get-Sha256Hex($Path) {
  return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Read-Checksum($ChecksumPath, $AssetName) {
  $content = Get-Content -Raw -Encoding UTF8 -LiteralPath $ChecksumPath
  foreach ($line in ($content -split "`n")) {
    $trimmed = $line.Trim()
    if ($trimmed -eq "") {
      continue
    }
    $fields = @($trimmed -split '\s+')
    if ($fields.Count -eq 1 -and $fields[0] -match '^[a-fA-F0-9]{64}$') {
      return $fields[0].ToLowerInvariant()
    }
    if ($fields.Count -ge 2 -and $fields[0] -match '^[a-fA-F0-9]{64}$') {
      $fileName = $fields[1].TrimStart("*")
      if ($fileName -eq $AssetName -or [System.IO.Path]::GetFileName($fileName) -eq $AssetName) {
        return $fields[0].ToLowerInvariant()
      }
    }
    if ($trimmed.ToLowerInvariant().StartsWith("sha256(")) {
      $closing = $trimmed.IndexOf(")")
      $prefixLength = "sha256(".Length
      if ($closing -gt $prefixLength) {
        $fileName = $trimmed.Substring($prefixLength, $closing - $prefixLength).Trim()
        $value = $trimmed.Substring($closing + 1).Trim().TrimStart("=")
        $value = $value.Trim()
        if ($value -match '^[a-fA-F0-9]{64}$' -and ($fileName -eq $AssetName -or [System.IO.Path]::GetFileName($fileName) -eq $AssetName)) {
          return $value.ToLowerInvariant()
        }
      }
    }
  }
  Fail "checksum file does not contain a sha256 digest for $AssetName"
}

function Assert-ReleaseSignature($AssetName, $Checksum, $SignaturePath) {
  if ([string]::IsNullOrWhiteSpace($ReleaseSignaturePublicKey)) {
    Fail "DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY is required when -VerifySignatures is used."
  }
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
      Fail "signature verification failed for $AssetName"
    }
  } finally {
    Remove-Item -LiteralPath $verifyFile -Force -ErrorAction SilentlyContinue
  }
}

function Save-RemoteAsset($AssetInfo, $Destination) {
  $uri = [string]$AssetInfo.url
  Invoke-WebRequest -Method GET -Uri $uri -Headers (New-GitHubHeaders "application/octet-stream") -OutFile $Destination | Out-Null
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

$errors = New-Object System.Collections.Generic.List[string]
$cleanupDir = ""

try {
  if (-not [string]::IsNullOrWhiteSpace($LocalDistDir)) {
    $distPath = (Resolve-Path -LiteralPath $LocalDistDir).Path
    $assetNames = @(Get-ChildItem -LiteralPath $distPath -File | ForEach-Object { $_.Name })
    $assetObjects = @{}
    foreach ($item in Get-ChildItem -LiteralPath $distPath -File) {
      $assetObjects[$item.Name] = [pscustomobject]@{ Name = $item.Name; Path = $item.FullName }
    }
    $latestTag = "(local)"
    $prerelease = $false
    $draft = $false
  } else {
    $release = Invoke-GitHubJson "https://api.github.com/repos/$Repo/releases/tags/$Tag"
    $latest = Invoke-GitHubJson "https://api.github.com/repos/$Repo/releases/latest"
    if ($latest.tag_name -ne $Tag) {
      $errors.Add("latest release mismatch: expected $Tag, got $($latest.tag_name)")
    }
    if ($release.prerelease -ne $false) {
      $errors.Add("$Tag must be prerelease=false")
    }
    if ($release.draft -ne $false) {
      $errors.Add("$Tag must be draft=false")
    }
    $assetNames = @($release.assets | ForEach-Object { $_.name })
    $assetObjects = @{}
    foreach ($item in $release.assets) {
      $assetObjects[$item.name] = $item
    }
    $latestTag = $latest.tag_name
    $prerelease = [bool]$release.prerelease
    $draft = [bool]$release.draft
  }

  $assetSet = @{}
  foreach ($name in $assetNames) {
    $assetSet[$name] = $true
  }

  foreach ($base in $expectedBaseAssets) {
    foreach ($name in @($base, "$base.sha256", "$base.sig")) {
      if (-not $assetSet.ContainsKey($name)) {
        $errors.Add("missing release asset: $name")
      }
    }
  }

  $unexpected = @()
  foreach ($name in $assetNames) {
    if ($name.EndsWith(".sha256") -or $name.EndsWith(".sig")) {
      continue
    }
    if ($expectedBaseAssets -notcontains $name) {
      $unexpected += $name
    }
  }
  if ($unexpected.Count -gt 0) {
    $errors.Add("unexpected base assets: $($unexpected -join ', ')")
  }

  if ($errors.Count -eq 0) {
    $verifyDir = $LocalDistDir
    if ([string]::IsNullOrWhiteSpace($LocalDistDir)) {
      $cleanupDir = Join-Path ([System.IO.Path]::GetTempPath()) ("dushengcdn-release-assets-" + [guid]::NewGuid().ToString())
      New-Item -ItemType Directory -Force -Path $cleanupDir | Out-Null
      foreach ($base in $expectedBaseAssets) {
        foreach ($name in @($base, "$base.sha256", "$base.sig")) {
          Save-RemoteAsset $assetObjects[$name] (Join-Path $cleanupDir $name)
        }
      }
      $verifyDir = $cleanupDir
    }

    foreach ($base in $expectedBaseAssets) {
      $assetPath = Join-Path $verifyDir $base
      $shaPath = Join-Path $verifyDir "$base.sha256"
      $sigPath = Join-Path $verifyDir "$base.sig"
      $expected = Read-Checksum $shaPath $base
      $actual = Get-Sha256Hex $assetPath
      if ($actual -ne $expected) {
        $errors.Add("checksum mismatch for $base")
        continue
      }
      if ($VerifySignatures) {
        try {
          Assert-ReleaseSignature $base $expected $sigPath
        } catch {
          $errors.Add($_.Exception.Message)
        }
      }
    }
  }

  $summary = [pscustomobject]@{
    Repository = $Repo
    Tag = $Tag
    Latest = $latestTag
    Prerelease = $prerelease
    Draft = $draft
    AssetCount = $assetNames.Count
    ExpectedBaseAssetCount = $expectedBaseAssets.Count
    ChecksumsVerified = ($errors.Count -eq 0)
    SignaturesVerified = [bool]$VerifySignatures
  }
  $summary | Format-List

  if ($errors.Count -gt 0) {
    foreach ($message in $errors) {
      Write-Error $message
    }
    exit 1
  }

  Write-Host "Release assets verified."
} finally {
  if (-not [string]::IsNullOrWhiteSpace($cleanupDir)) {
    Remove-Item -LiteralPath $cleanupDir -Recurse -Force -ErrorAction SilentlyContinue
  }
}
