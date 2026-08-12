#requires -Version 7.4

[CmdletBinding()]
param(
    [ValidateRange(330, 86400)]
    [int]$TicketRetirementDelaySeconds = 330
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Import-Module (Join-Path $PSScriptRoot "../PodmanCompose.psm1") -Force
Import-Module (Join-Path $PSScriptRoot "RotationMaterial.psm1") -Force
Import-Module (Join-Path $PSScriptRoot "RotationPodman.psm1") -Force

$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "../..")
$suffix = [Guid]::NewGuid().ToString("N").Substring(0, 10)
$prefix = "pai-rotation-$suffix"
$project = "$prefix-project"
$temporaryRoot = Join-Path ([System.IO.Path]::GetTempPath()) $prefix
$composeFile = Join-Path $temporaryRoot "compose.yaml"
$alpineImage = "docker.io/library/alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40"
$containers = @{
    Account = "$prefix-account-probe"
    Platform = "$prefix-platform-probe"
    Consumer = "$prefix-consumer-probe"
}
$secrets = @{
    TicketSigning = "$prefix-ticket-signing"
    TicketPublicKeyring = "$prefix-ticket-public-keyring"
    EncryptionKeyring = "$prefix-credential-encryption-keyring"
    ControlTrust = "$prefix-control-trust"
    ControlServerCertificate = "$prefix-control-server-certificate"
    ControlServerKey = "$prefix-control-server-key"
    ControlClientCertificate = "$prefix-control-client-certificate"
    ControlClientKey = "$prefix-control-client-key"
    RuntimeTrust = "$prefix-runtime-trust"
    RuntimeServerCertificate = "$prefix-runtime-server-certificate"
    RuntimeServerKey = "$prefix-runtime-server-key"
    AccountTrust = "$prefix-account-trust"
    AccountServerCertificate = "$prefix-account-server-certificate"
    AccountServerKey = "$prefix-account-server-key"
}

function ConvertTo-RawBase64 {
    param([Parameter(Mandatory)][byte[]]$Value)
    return [Convert]::ToBase64String($Value).TrimEnd("=")
}

function New-RehearsalStage {
    param(
        [Parameter(Mandatory)][string]$Signing,
        [Parameter(Mandatory)][string]$PublicKeyring,
        [Parameter(Mandatory)][string]$EncryptionKeyring,
        [Parameter(Mandatory)][string]$ControlTrust,
        [Parameter(Mandatory)][hashtable]$ControlIdentity,
        [Parameter(Mandatory)][string]$RuntimeTrust,
        [Parameter(Mandatory)][hashtable]$RuntimeIdentity,
        [Parameter(Mandatory)][string]$AccountTrust,
        [Parameter(Mandatory)][hashtable]$AccountIdentity
    )
    return @{
        Files = @{
            $secrets.TicketSigning = $Signing
            $secrets.TicketPublicKeyring = $PublicKeyring
            $secrets.EncryptionKeyring = $EncryptionKeyring
            $secrets.ControlTrust = $ControlTrust
            $secrets.ControlServerCertificate = $ControlIdentity.serverCertificate
            $secrets.ControlServerKey = $ControlIdentity.serverKey
            $secrets.ControlClientCertificate = $ControlIdentity.clientCertificate
            $secrets.ControlClientKey = $ControlIdentity.clientKey
            $secrets.RuntimeTrust = $RuntimeTrust
            $secrets.RuntimeServerCertificate = $RuntimeIdentity.serverCertificate
            $secrets.RuntimeServerKey = $RuntimeIdentity.serverKey
            $secrets.AccountTrust = $AccountTrust
            $secrets.AccountServerCertificate = $AccountIdentity.serverCertificate
            $secrets.AccountServerKey = $AccountIdentity.serverKey
        }
        AccountTargets = @($secrets.TicketSigning, $secrets.ControlTrust, $secrets.ControlClientCertificate, $secrets.ControlClientKey, $secrets.AccountServerCertificate, $secrets.AccountServerKey)
        PlatformTargets = @($secrets.TicketPublicKeyring, $secrets.EncryptionKeyring, $secrets.ControlTrust, $secrets.ControlServerCertificate, $secrets.ControlServerKey, $secrets.RuntimeTrust, $secrets.RuntimeServerCertificate, $secrets.RuntimeServerKey)
        ConsumerTargets = @($secrets.RuntimeTrust, $secrets.AccountTrust)
    }
}

function Assert-RehearsalStage {
    param([Parameter(Mandatory)][hashtable]$Stage)

    foreach ($consumer in @(
        @{ Container = $containers.Account; Targets = $Stage.AccountTargets },
        @{ Container = $containers.Platform; Targets = $Stage.PlatformTargets },
        @{ Container = $containers.Consumer; Targets = $Stage.ConsumerTargets }
    )) {
        $expected = @{}
        foreach ($target in $consumer.Targets) {
            $expected[$target] = $Stage.Files[$target]
        }
        Assert-RotationMountedSecrets -Container $consumer.Container -Expected $expected
    }
}

function Set-RehearsalSecrets {
    param(
        [Parameter(Mandatory)][hashtable]$Next,
        [Parameter(Mandatory)][string[]]$Names
    )
    foreach ($name in $Names) {
        New-OrReplaceRotationSecret -Name $name -Path $Next.Files[$name] -Replace
    }
}

function Assert-TLSDomainRotation {
    param(
        [Parameter(Mandatory)][string]$OpenSSL,
        [Parameter(Mandatory)][hashtable]$OldIdentity,
        [Parameter(Mandatory)][hashtable]$NewIdentity,
        [Parameter(Mandatory)][hashtable]$Trust,
        [switch]$IncludeClient
    )
    $identityKinds = @(@{ Name = "serverCertificate"; Purpose = "sslserver" })
    if ($IncludeClient) {
        $identityKinds += @{ Name = "clientCertificate"; Purpose = "sslclient" }
    }
    foreach ($kind in $identityKinds) {
        Assert-RotationCertificate -OpenSSL $OpenSSL -TrustBundle $Trust.Old -Certificate $OldIdentity[$kind.Name] -Purpose $kind.Purpose -ShouldSucceed $true
        Assert-RotationCertificate -OpenSSL $OpenSSL -TrustBundle $Trust.Overlap -Certificate $OldIdentity[$kind.Name] -Purpose $kind.Purpose -ShouldSucceed $true
        Assert-RotationCertificate -OpenSSL $OpenSSL -TrustBundle $Trust.New -Certificate $OldIdentity[$kind.Name] -Purpose $kind.Purpose -ShouldSucceed $false
        Assert-RotationCertificate -OpenSSL $OpenSSL -TrustBundle $Trust.Overlap -Certificate $NewIdentity[$kind.Name] -Purpose $kind.Purpose -ShouldSucceed $true
        Assert-RotationCertificate -OpenSSL $OpenSSL -TrustBundle $Trust.New -Certificate $NewIdentity[$kind.Name] -Purpose $kind.Purpose -ShouldSucceed $true
    }
}

function Wait-ForTicketRetirement {
    param([Parameter(Mandatory)][datetimeoffset]$NotBefore)

    Write-Host "Waiting until all five-minute tickets and the 30-second verifier leeway have elapsed."
    while ([DateTimeOffset]::UtcNow -lt $NotBefore) {
        $remaining = [Math]::Ceiling(($NotBefore - [DateTimeOffset]::UtcNow).TotalSeconds)
        Start-Sleep -Seconds ([Math]::Min(30, [Math]::Max(1, $remaining)))
    }
}

function Invoke-ApplicationRotationChecks {
    & go -C (Join-Path $repositoryRoot "contracts/runtime/go") test -count=1 -run '^TestFileIssuerReloadsSigningKeyAndPublicKeyringSupportsOverlap$' ./serviceticket *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Service-ticket overlap check failed"
    }
    & go -C (Join-Path $repositoryRoot "services/platform-mihomo") test -tags=integration -count=1 -run '^TestCredentialKeyRotationReencryptsPersistentPostgreSQLRecords$' ./integration *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Persistent credential re-encryption check failed"
    }
}

function Remove-RehearsalResources {
    $failures = [System.Collections.Generic.List[string]]::new()
    if (Get-Command podman -ErrorAction SilentlyContinue) {
        & podman rm --force --time 0 --ignore @($containers.Values) *> $null
        if ($LASTEXITCODE -ne 0) {
            $failures.Add("containers")
        }
        & podman pod exists "pod_$project" *> $null
        if ($LASTEXITCODE -eq 0) {
            & podman pod rm --force --time 0 "pod_$project" *> $null
            if ($LASTEXITCODE -ne 0) {
                $failures.Add("pod")
            }
        } elseif ($LASTEXITCODE -ne 1) {
            $failures.Add("pod lookup")
        }
        foreach ($secretName in $secrets.Values) {
            & podman secret exists $secretName *> $null
            if ($LASTEXITCODE -eq 0) {
                & podman secret rm $secretName *> $null
                if ($LASTEXITCODE -ne 0) {
                    $failures.Add("secret $secretName")
                }
            } elseif ($LASTEXITCODE -ne 1) {
                $failures.Add("secret lookup $secretName")
            }
        }
        & podman network exists "${project}_default" *> $null
        if ($LASTEXITCODE -eq 0) {
            & podman network rm --force "${project}_default" *> $null
            if ($LASTEXITCODE -ne 0) {
                $failures.Add("network")
            }
        } elseif ($LASTEXITCODE -ne 1) {
            $failures.Add("network lookup")
        }
    }
    if (Test-Path -LiteralPath $temporaryRoot) {
        try {
            Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
        } catch {
            $failures.Add("temporary files")
        }
    }
    if ($failures.Count -gt 0) {
        throw "Rotation rehearsal cleanup failed: $($failures -join ', ')"
    }
}

try {
    foreach ($command in @("podman", "go")) {
        if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
            throw "$command is required for the rotation rehearsal"
        }
    }
    $podmanCompose = Assert-PodmanComposeAvailable
    $openssl = Resolve-RotationOpenSSL
    New-Item -ItemType Directory -Path $temporaryRoot | Out-Null
    New-RotationComposeFile -Path $composeFile -Image $alpineImage -Containers $containers -Secrets $secrets

    $oldTLS = @{
        Control = New-RotationTLSIdentitySet -OpenSSL $openssl -Directory (Join-Path $temporaryRoot "control-old") -Name "control-old" -ServerName "platform-control.internal" -IncludeClient
        Runtime = New-RotationTLSIdentitySet -OpenSSL $openssl -Directory (Join-Path $temporaryRoot "runtime-old") -Name "runtime-old" -ServerName "platform-runtime.internal"
        Account = New-RotationTLSIdentitySet -OpenSSL $openssl -Directory (Join-Path $temporaryRoot "account-old") -Name "account-old" -ServerName "account-grpc.internal"
    }
    $newTLS = @{
        Control = New-RotationTLSIdentitySet -OpenSSL $openssl -Directory (Join-Path $temporaryRoot "control-new") -Name "control-new" -ServerName "platform-control.internal" -IncludeClient
        Runtime = New-RotationTLSIdentitySet -OpenSSL $openssl -Directory (Join-Path $temporaryRoot "runtime-new") -Name "runtime-new" -ServerName "platform-runtime.internal"
        Account = New-RotationTLSIdentitySet -OpenSSL $openssl -Directory (Join-Path $temporaryRoot "account-new") -Name "account-new" -ServerName "account-grpc.internal"
    }
    $trust = @{}
    foreach ($domain in @("Control", "Runtime", "Account")) {
        $trust[$domain] = @{
            Old = Join-Path $temporaryRoot "$($domain.ToLowerInvariant())-trust-old.pem"
            Overlap = Join-Path $temporaryRoot "$($domain.ToLowerInvariant())-trust-overlap.pem"
            New = Join-Path $temporaryRoot "$($domain.ToLowerInvariant())-trust-new.pem"
        }
        New-RotationTrustBundle -Path $trust[$domain].Old -Authorities @($oldTLS[$domain].CA)
        New-RotationTrustBundle -Path $trust[$domain].Overlap -Authorities @($oldTLS[$domain].CA, $newTLS[$domain].CA)
        New-RotationTrustBundle -Path $trust[$domain].New -Authorities @($newTLS[$domain].CA)
        Assert-TLSDomainRotation -OpenSSL $openssl -OldIdentity $oldTLS[$domain] -NewIdentity $newTLS[$domain] -Trust $trust[$domain] -IncludeClient:($domain -eq "Control")
    }

    $oldTicket = New-RotationTicketKeyPair -RepositoryRoot $repositoryRoot
    $newTicket = New-RotationTicketKeyPair -RepositoryRoot $repositoryRoot
    $oldEncryptionKey = [byte[]]::new(32)
    $newEncryptionKey = [byte[]]::new(32)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($oldEncryptionKey)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($newEncryptionKey)
    $oldEncryptionBase64 = ConvertTo-RawBase64 -Value $oldEncryptionKey
    $newEncryptionBase64 = ConvertTo-RawBase64 -Value $newEncryptionKey

    $material = @{}
    foreach ($name in @("signing-old", "signing-new", "ticket-old", "ticket-overlap", "ticket-new", "encryption-old", "encryption-overlap", "encryption-new")) {
        $material[$name] = Join-Path $temporaryRoot $name
    }
    Write-RotationUTF8File -Path $material["signing-old"] -Value (@{ kid = "ticket-old"; private_key_pem = $oldTicket.private_key_pem } | ConvertTo-Json -Compress)
    Write-RotationUTF8File -Path $material["signing-new"] -Value (@{ kid = "ticket-new"; private_key_pem = $newTicket.private_key_pem } | ConvertTo-Json -Compress)
    Write-RotationUTF8File -Path $material["ticket-old"] -Value (@{ keys = @(@{ kid = "ticket-old"; public_key_pem = $oldTicket.public_key_pem }) } | ConvertTo-Json -Depth 4 -Compress)
    Write-RotationUTF8File -Path $material["ticket-overlap"] -Value (@{ keys = @(@{ kid = "ticket-old"; public_key_pem = $oldTicket.public_key_pem }, @{ kid = "ticket-new"; public_key_pem = $newTicket.public_key_pem }) } | ConvertTo-Json -Depth 4 -Compress)
    Write-RotationUTF8File -Path $material["ticket-new"] -Value (@{ keys = @(@{ kid = "ticket-new"; public_key_pem = $newTicket.public_key_pem }) } | ConvertTo-Json -Depth 4 -Compress)
    Write-RotationUTF8File -Path $material["encryption-old"] -Value (@{ active_kid = "enc-old"; keys = @(@{ kid = "enc-old"; key_base64 = $oldEncryptionBase64 }) } | ConvertTo-Json -Depth 4 -Compress)
    Write-RotationUTF8File -Path $material["encryption-overlap"] -Value (@{ active_kid = "enc-new"; keys = @(@{ kid = "enc-old"; key_base64 = $oldEncryptionBase64 }, @{ kid = "enc-new"; key_base64 = $newEncryptionBase64 }) } | ConvertTo-Json -Depth 4 -Compress)
    Write-RotationUTF8File -Path $material["encryption-new"] -Value (@{ active_kid = "enc-new"; keys = @(@{ kid = "enc-new"; key_base64 = $newEncryptionBase64 }) } | ConvertTo-Json -Depth 4 -Compress)

    $initial = New-RehearsalStage -Signing $material["signing-old"] -PublicKeyring $material["ticket-old"] -EncryptionKeyring $material["encryption-old"] -ControlTrust $trust.Control.Old -ControlIdentity $oldTLS.Control -RuntimeTrust $trust.Runtime.Old -RuntimeIdentity $oldTLS.Runtime -AccountTrust $trust.Account.Old -AccountIdentity $oldTLS.Account
    $overlap = New-RehearsalStage -Signing $material["signing-old"] -PublicKeyring $material["ticket-overlap"] -EncryptionKeyring $material["encryption-overlap"] -ControlTrust $trust.Control.Overlap -ControlIdentity $oldTLS.Control -RuntimeTrust $trust.Runtime.Overlap -RuntimeIdentity $oldTLS.Runtime -AccountTrust $trust.Account.Overlap -AccountIdentity $oldTLS.Account
    $switched = New-RehearsalStage -Signing $material["signing-new"] -PublicKeyring $material["ticket-overlap"] -EncryptionKeyring $material["encryption-overlap"] -ControlTrust $trust.Control.Overlap -ControlIdentity $newTLS.Control -RuntimeTrust $trust.Runtime.Overlap -RuntimeIdentity $newTLS.Runtime -AccountTrust $trust.Account.Overlap -AccountIdentity $newTLS.Account
    $retired = New-RehearsalStage -Signing $material["signing-new"] -PublicKeyring $material["ticket-new"] -EncryptionKeyring $material["encryption-new"] -ControlTrust $trust.Control.New -ControlIdentity $newTLS.Control -RuntimeTrust $trust.Runtime.New -RuntimeIdentity $newTLS.Runtime -AccountTrust $trust.Account.New -AccountIdentity $newTLS.Account

    foreach ($entry in $initial.Files.GetEnumerator()) {
        New-OrReplaceRotationSecret -Name $entry.Key -Path $entry.Value
    }
    Invoke-RotationRecreate -PodmanCompose $podmanCompose -Project $project -ComposeFile $composeFile
    Assert-RehearsalStage -Stage $initial

    Set-RehearsalSecrets -Next $overlap -Names @($secrets.TicketPublicKeyring, $secrets.EncryptionKeyring, $secrets.ControlTrust, $secrets.RuntimeTrust, $secrets.AccountTrust)
    & podman restart --time 0 @($containers.Values) *> $null
    if ($LASTEXITCODE -ne 0) { throw "Could not restart overlap-stage consumers" }
    Assert-RehearsalStage -Stage $initial
    Invoke-RotationRecreate -PodmanCompose $podmanCompose -Project $project -ComposeFile $composeFile
    Assert-RehearsalStage -Stage $overlap

    Set-RehearsalSecrets -Next $switched -Names @($secrets.TicketSigning, $secrets.ControlServerCertificate, $secrets.ControlServerKey, $secrets.ControlClientCertificate, $secrets.ControlClientKey, $secrets.RuntimeServerCertificate, $secrets.RuntimeServerKey, $secrets.AccountServerCertificate, $secrets.AccountServerKey)
    & podman restart --time 0 @($containers.Values) *> $null
    if ($LASTEXITCODE -ne 0) { throw "Could not restart identity-switch consumers" }
    Assert-RehearsalStage -Stage $overlap
    Invoke-RotationRecreate -PodmanCompose $podmanCompose -Project $project -ComposeFile $composeFile
    Assert-RehearsalStage -Stage $switched
    $retirementNotBefore = [DateTimeOffset]::UtcNow.AddSeconds($TicketRetirementDelaySeconds)

    Invoke-ApplicationRotationChecks
    Wait-ForTicketRetirement -NotBefore $retirementNotBefore
    Set-RehearsalSecrets -Next $retired -Names @($secrets.TicketPublicKeyring, $secrets.EncryptionKeyring, $secrets.ControlTrust, $secrets.RuntimeTrust, $secrets.AccountTrust)
    & podman restart --time 0 @($containers.Values) *> $null
    if ($LASTEXITCODE -ne 0) { throw "Could not restart retirement-stage consumers" }
    Assert-RehearsalStage -Stage $switched
    Invoke-RotationRecreate -PodmanCompose $podmanCompose -Project $project -ComposeFile $composeFile
    Assert-RehearsalStage -Stage $retired

    $sensitiveNeedles = @($oldEncryptionBase64, $newEncryptionBase64)
    foreach ($ticket in @($oldTicket, $newTicket)) {
        $sensitiveNeedles += (($ticket.private_key_pem -split "`n") | Where-Object { $_ -and $_ -notlike "---*" } | Select-Object -First 1)
    }
    foreach ($domain in @("Control", "Runtime", "Account")) {
        foreach ($generation in @($oldTLS, $newTLS)) {
            $sensitiveNeedles += Get-RotationPEMNeedle -Path $generation[$domain].serverKey
            if ($domain -eq "Control") {
                $sensitiveNeedles += Get-RotationPEMNeedle -Path $generation[$domain].clientKey
            }
        }
    }
    Assert-NoRotationSecretDisclosure -Containers @($containers.Values) -SensitiveNeedles $sensitiveNeedles
    Write-Host "External-secret three-stage rotation rehearsal passed."
} finally {
    Remove-RehearsalResources
}
