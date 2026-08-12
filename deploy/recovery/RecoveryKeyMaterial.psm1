Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function ConvertFrom-RequiredJSONFile {
    param([Parameter(Mandatory)][string]$Path)
    try {
        return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json -ErrorAction Stop
    } catch {
        throw [System.IO.InvalidDataException]::new("Invalid recovery key JSON file: $Path", $_.Exception)
    }
}

function ConvertFrom-UnpaddedBase64 {
    param([Parameter(Mandatory)][string]$Value)
    if ($Value -notmatch '^[A-Za-z0-9+/]+$') {
        throw "Platform encryption key must use unpadded standard Base64"
    }
    $padding = (4 - ($Value.Length % 4)) % 4
    try {
        return [Convert]::FromBase64String($Value + ("=" * $padding))
    } catch {
        throw [System.IO.InvalidDataException]::new("Invalid Base64 key material", $_.Exception)
    }
}

function ConvertFrom-PEMBlock {
    param(
        [Parameter(Mandatory)][string]$PEM,
        [Parameter(Mandatory)][string]$Label
    )
    $pattern = '^-----BEGIN ' + [Regex]::Escape($Label) + '-----\s*(?<body>[A-Za-z0-9+/=\s]+?)\s*-----END ' + [Regex]::Escape($Label) + '-----\s*$'
    $match = [Regex]::Match($PEM, $pattern)
    if (-not $match.Success) {
        throw "Recovery service-ticket key is not a valid $Label PEM block"
    }
    try {
        return [Convert]::FromBase64String(($match.Groups["body"].Value -replace '\s', ''))
    } catch {
        throw [System.IO.InvalidDataException]::new("Recovery service-ticket PEM contains invalid Base64", $_.Exception)
    }
}

function Assert-Ed25519KeyEncoding {
    param(
        [Parameter(Mandatory)][byte[]]$DER,
        [switch]$Public
    )
    $hex = [Convert]::ToHexString($DER).ToLowerInvariant()
    if ($Public) {
        if ($DER.Length -ne 44 -or -not $hex.StartsWith("302a300506032b6570032100", [StringComparison]::Ordinal)) {
            throw "Recovery service-ticket public key must be Ed25519"
        }
        return
    }
    if ($DER.Length -ne 48 -or -not $hex.StartsWith("302e020100300506032b657004220420", [StringComparison]::Ordinal)) {
        throw "Recovery service-ticket private key must be Ed25519 PKCS#8"
    }
}

function Resolve-OpenSSLPath {
    $command = Get-Command openssl -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }
    if ($IsWindows) {
        foreach ($candidate in @(
            "C:\Program Files\Git\usr\bin\openssl.exe",
            "C:\Program Files\Git\mingw64\bin\openssl.exe",
            "C:\Program Files\OpenSSL-Win64\bin\openssl.exe"
        )) {
            if (Test-Path -LiteralPath $candidate -PathType Leaf) {
                return $candidate
            }
        }
    }
    throw "openssl is required to validate recovered service-ticket keys"
}

function Assert-OpenSSLKey {
    param(
        [Parameter(Mandatory)][string]$PEM,
        [Parameter(Mandatory)][string]$WorkingDirectory,
        [Parameter(Mandatory)][string]$OpenSSLPath,
        [switch]$Public
    )
    $path = Join-Path $WorkingDirectory ".key-probe-$([Guid]::NewGuid().ToString('N')).pem"
    try {
        $label = if ($Public) { "PUBLIC KEY" } else { "PRIVATE KEY" }
        Assert-Ed25519KeyEncoding -DER (ConvertFrom-PEMBlock -PEM $PEM -Label $label) -Public:$Public
        [System.IO.File]::WriteAllText($path, $PEM)
        $arguments = if ($Public) {
            @("pkey", "-pubin", "-in", $path, "-pubout")
        } else {
            @("pkey", "-in", $path, "-pubout")
        }
        $publicKey = @(& $OpenSSLPath @arguments 2>$null)
        if ($LASTEXITCODE -ne 0) {
            throw "Recovery service-ticket key is not a valid PEM key"
        }
        return ($publicKey -join "`n").Trim()
    } finally {
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Force
        }
    }
}

function Assert-RecoveryKeyMaterial {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$AccountEncryptionKeyFile,
        [Parameter(Mandatory)][string]$AccountServiceTicketSigningKeyFile,
        [Parameter(Mandatory)][string]$PlatformEncryptionKeyringFile,
        [Parameter(Mandatory)][string]$AccountServiceTicketPublicKeyringFile,
        [Parameter(Mandatory)][string]$WorkingDirectory
    )

    $opensslPath = Resolve-OpenSSLPath
    foreach ($path in @(
        $AccountEncryptionKeyFile,
        $AccountServiceTicketSigningKeyFile,
        $PlatformEncryptionKeyringFile,
        $AccountServiceTicketPublicKeyringFile
    )) {
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "Required recovery key material is missing: $path"
        }
    }

    $accountEncryptionKey = (Get-Content -LiteralPath $AccountEncryptionKeyFile -Raw).Trim()
    $rawKeyBytes = [Text.Encoding]::UTF8.GetBytes($accountEncryptionKey)
    $validAccountKey = $rawKeyBytes.Length -eq 32
    if (-not $validAccountKey) {
        try {
            $validAccountKey = [Convert]::FromBase64String($accountEncryptionKey).Length -eq 32
        } catch {
            $validAccountKey = $false
        }
    }
    if (-not $validAccountKey) {
        throw "Account encryption recovery key must decode to exactly 32 bytes"
    }

    $signingKey = ConvertFrom-RequiredJSONFile -Path $AccountServiceTicketSigningKeyFile
    if ([string]::IsNullOrWhiteSpace([string]$signingKey.kid) -or [string]::IsNullOrWhiteSpace([string]$signingKey.private_key_pem)) {
        throw "Account service-ticket signing key is incomplete"
    }
    $signingPublicKey = Assert-OpenSSLKey -PEM ([string]$signingKey.private_key_pem) -WorkingDirectory $WorkingDirectory -OpenSSLPath $opensslPath

    $publicKeyring = ConvertFrom-RequiredJSONFile -Path $AccountServiceTicketPublicKeyringFile
    $publicKeys = @($publicKeyring.keys)
    if ($publicKeys.Count -eq 0) {
        throw "Account service-ticket public keyring is empty"
    }
    $seenTicketKeyIDs = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach ($entry in $publicKeys) {
        if ([string]::IsNullOrWhiteSpace([string]$entry.kid) -or -not $seenTicketKeyIDs.Add([string]$entry.kid)) {
            throw "Account service-ticket public keyring has an empty or duplicate key ID"
        }
        $normalizedPublicKey = Assert-OpenSSLKey -PEM ([string]$entry.public_key_pem) -WorkingDirectory $WorkingDirectory -OpenSSLPath $opensslPath -Public
        if ([string]$entry.kid -eq [string]$signingKey.kid -and $normalizedPublicKey -ne $signingPublicKey) {
            throw "Account signing key does not match its Platform public key"
        }
    }
    if (-not $seenTicketKeyIDs.Contains([string]$signingKey.kid)) {
        throw "Account signing key ID is absent from the Platform public keyring"
    }

    $platformKeyring = ConvertFrom-RequiredJSONFile -Path $PlatformEncryptionKeyringFile
    if ([string]$platformKeyring.active_kid -notmatch '^[A-Za-z0-9_-]{1,64}$') {
        throw "Platform encryption keyring has an invalid active key ID"
    }
    $platformKeys = @($platformKeyring.keys)
    if ($platformKeys.Count -eq 0) {
        throw "Platform encryption keyring is empty"
    }
    $seenEncryptionKeyIDs = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach ($entry in $platformKeys) {
        if ([string]$entry.kid -notmatch '^[A-Za-z0-9_-]{1,64}$' -or -not $seenEncryptionKeyIDs.Add([string]$entry.kid)) {
            throw "Platform encryption keyring has an invalid or duplicate key ID"
        }
        if ((ConvertFrom-UnpaddedBase64 -Value ([string]$entry.key_base64)).Length -ne 32) {
            throw "Platform encryption keys must decode to exactly 32 bytes"
        }
    }
    if (-not $seenEncryptionKeyIDs.Contains([string]$platformKeyring.active_kid)) {
        throw "Platform active encryption key is absent from its keyring"
    }
}

Export-ModuleMember -Function Assert-RecoveryKeyMaterial
