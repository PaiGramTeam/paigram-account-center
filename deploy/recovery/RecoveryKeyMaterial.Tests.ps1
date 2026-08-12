#requires -Version 7.4

Import-Module (Join-Path $PSScriptRoot "RecoveryKeyMaterial.psm1") -Force
$script:openssl = if (Get-Command openssl -ErrorAction SilentlyContinue) {
    (Get-Command openssl).Source
} else {
    "C:\Program Files\Git\usr\bin\openssl.exe"
}

Describe "Recovery key material validation" {
    BeforeEach {
        $accountKey = Join-Path $TestDrive "account-key"
        $signingKey = Join-Path $TestDrive "signing.json"
        $publicKeyring = Join-Path $TestDrive "public.json"
        $encryptionKeyring = Join-Path $TestDrive "encryption.json"
        [System.IO.File]::WriteAllText($accountKey, "0123456789abcdef0123456789abcdef")

        $generated = ((@(& go -C (Resolve-Path (Join-Path $PSScriptRoot "../../contracts/runtime/go")) run ./cmd/service-ticket-keygen)) -join "`n") | ConvertFrom-Json
        @{ kid = "ticket"; private_key_pem = $generated.private_key_pem } |
            ConvertTo-Json | Set-Content -LiteralPath $signingKey
        @{ keys = @(@{ kid = "ticket"; public_key_pem = $generated.public_key_pem }) } |
            ConvertTo-Json | Set-Content -LiteralPath $publicKeyring
        @{ active_kid = "enc"; keys = @(@{
            kid = "enc"
            key_base64 = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"
        }) } | ConvertTo-Json | Set-Content -LiteralPath $encryptionKeyring
    }

    It "accepts the runtime Ed25519 and unpadded Base64 formats" {
        Assert-RecoveryKeyMaterial `
            -AccountEncryptionKeyFile $accountKey `
            -AccountServiceTicketSigningKeyFile $signingKey `
            -PlatformEncryptionKeyringFile $encryptionKeyring `
            -AccountServiceTicketPublicKeyringFile $publicKeyring `
            -WorkingDirectory $TestDrive
    }

    It "rejects a matching RSA ticket key pair" {
        $rsaPrivate = Join-Path $TestDrive "rsa-private.pem"
        $rsaPublic = Join-Path $TestDrive "rsa-public.pem"
        & $script:openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out $rsaPrivate *> $null
        & $script:openssl pkey -in $rsaPrivate -pubout -out $rsaPublic *> $null
        @{ kid = "rsa"; private_key_pem = [System.IO.File]::ReadAllText($rsaPrivate) } |
            ConvertTo-Json | Set-Content -LiteralPath $signingKey
        @{ keys = @(@{ kid = "rsa"; public_key_pem = [System.IO.File]::ReadAllText($rsaPublic) }) } |
            ConvertTo-Json | Set-Content -LiteralPath $publicKeyring

        {
            Assert-RecoveryKeyMaterial `
                -AccountEncryptionKeyFile $accountKey `
                -AccountServiceTicketSigningKeyFile $signingKey `
                -PlatformEncryptionKeyringFile $encryptionKeyring `
                -AccountServiceTicketPublicKeyringFile $publicKeyring `
                -WorkingDirectory $TestDrive
        } | Should Throw "Recovery service-ticket private key must be Ed25519 PKCS#8"
    }

    It "rejects padded Platform encryption key Base64" {
        @{ active_kid = "enc"; keys = @(@{
            kid = "enc"
            key_base64 = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
        }) } | ConvertTo-Json | Set-Content -LiteralPath $encryptionKeyring

        {
            Assert-RecoveryKeyMaterial `
                -AccountEncryptionKeyFile $accountKey `
                -AccountServiceTicketSigningKeyFile $signingKey `
                -PlatformEncryptionKeyringFile $encryptionKeyring `
                -AccountServiceTicketPublicKeyringFile $publicKeyring `
                -WorkingDirectory $TestDrive
        } | Should Throw "Platform encryption key must use unpadded standard Base64"
    }
}
