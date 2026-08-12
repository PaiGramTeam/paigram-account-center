Set-StrictMode -Version Latest

function Invoke-MaterialCommand {
    param(
        [Parameter(Mandatory)][string]$Command,
        [Parameter(Mandatory)][string[]]$Arguments,
        [Parameter(Mandatory)][string]$FailureMessage
    )
    & $Command @Arguments *> $null
    if ($LASTEXITCODE -ne 0) {
        throw $FailureMessage
    }
}

function Resolve-RotationOpenSSL {
    $command = Get-Command openssl -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }
    $gitOpenSSL = "C:\Program Files\Git\usr\bin\openssl.exe"
    if (Test-Path -LiteralPath $gitOpenSSL -PathType Leaf) {
        return $gitOpenSSL
    }
    throw "OpenSSL is required for the rotation rehearsal"
}

function Write-RotationUTF8File {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Value
    )
    [System.IO.File]::WriteAllText($Path, $Value, [System.Text.UTF8Encoding]::new($false))
}

function New-RotationTLSIdentitySet {
    param(
        [Parameter(Mandatory)][string]$OpenSSL,
        [Parameter(Mandatory)][string]$Directory,
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$ServerName,
        [switch]$IncludeClient
    )
    New-Item -ItemType Directory -Path $Directory | Out-Null
    $caKey = Join-Path $Directory "ca.key"
    $caCertificate = Join-Path $Directory "ca.pem"
    Invoke-MaterialCommand -Command $OpenSSL -Arguments @(
        "genpkey", "-algorithm", "ED25519", "-out", $caKey
    ) -FailureMessage "Could not generate $Name rehearsal CA key"
    Invoke-MaterialCommand -Command $OpenSSL -Arguments @(
        "req", "-x509", "-new", "-key", $caKey, "-subj", "/CN=$Name-rehearsal-ca",
        "-days", "2", "-out", $caCertificate
    ) -FailureMessage "Could not generate $Name rehearsal CA certificate"

    $identities = @(
        @{ Kind = "server"; CommonName = $ServerName; Extension = "extendedKeyUsage=serverAuth`nsubjectAltName=DNS:$ServerName" }
    )
    if ($IncludeClient) {
        $identities += @{ Kind = "client"; CommonName = "account-center"; Extension = "extendedKeyUsage=clientAuth" }
    }
    $result = @{ CA = $caCertificate }
    foreach ($identity in $identities) {
        $key = Join-Path $Directory "$($identity.Kind).key"
        $request = Join-Path $Directory "$($identity.Kind).csr"
        $certificate = Join-Path $Directory "$($identity.Kind).crt"
        $extension = Join-Path $Directory "$($identity.Kind).ext"
        Write-RotationUTF8File -Path $extension -Value "basicConstraints=critical,CA:FALSE`nkeyUsage=critical,digitalSignature`n$($identity.Extension)`n"
        Invoke-MaterialCommand -Command $OpenSSL -Arguments @(
            "genpkey", "-algorithm", "ED25519", "-out", $key
        ) -FailureMessage "Could not generate $Name $($identity.Kind) key"
        Invoke-MaterialCommand -Command $OpenSSL -Arguments @(
            "req", "-new", "-key", $key, "-subj", "/CN=$($identity.CommonName)", "-out", $request
        ) -FailureMessage "Could not generate $Name $($identity.Kind) request"
        Invoke-MaterialCommand -Command $OpenSSL -Arguments @(
            "x509", "-req", "-in", $request, "-CA", $caCertificate, "-CAkey", $caKey,
            "-CAcreateserial", "-days", "1", "-extfile", $extension, "-out", $certificate
        ) -FailureMessage "Could not generate $Name $($identity.Kind) certificate"
        $result["$($identity.Kind)Certificate"] = $certificate
        $result["$($identity.Kind)Key"] = $key
    }
    return $result
}

function New-RotationTicketKeyPair {
    param([Parameter(Mandatory)][string]$RepositoryRoot)

    $module = Resolve-Path (Join-Path $RepositoryRoot "contracts/runtime/go")
    $generated = ((@(& go -C $module run ./cmd/service-ticket-keygen)) -join "`n") | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0 -or -not $generated.private_key_pem -or -not $generated.public_key_pem) {
        throw "Could not generate service-ticket rehearsal keys"
    }
    return $generated
}

function New-RotationTrustBundle {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string[]]$Authorities
    )
    $content = ($Authorities | ForEach-Object { Get-Content -LiteralPath $_ -Raw }) -join ""
    Write-RotationUTF8File -Path $Path -Value $content
}

function Assert-RotationCertificate {
    param(
        [Parameter(Mandatory)][string]$OpenSSL,
        [Parameter(Mandatory)][string]$TrustBundle,
        [Parameter(Mandatory)][string]$Certificate,
        [Parameter(Mandatory)][ValidateSet("sslserver", "sslclient")][string]$Purpose,
        [Parameter(Mandatory)][bool]$ShouldSucceed
    )
    & $OpenSSL verify -purpose $Purpose -CAfile $TrustBundle $Certificate *> $null
    if (($LASTEXITCODE -eq 0) -ne $ShouldSucceed) {
        throw "TLS trust result did not match the expected rotation stage"
    }
}

function Get-RotationPEMNeedle {
    param([Parameter(Mandatory)][string]$Path)

    return ((Get-Content -LiteralPath $Path) | Where-Object { $_ -and $_ -notlike "---*" } | Select-Object -First 1)
}

Export-ModuleMember -Function Resolve-RotationOpenSSL, Write-RotationUTF8File, New-RotationTLSIdentitySet, New-RotationTicketKeyPair, New-RotationTrustBundle, Assert-RotationCertificate, Get-RotationPEMNeedle
