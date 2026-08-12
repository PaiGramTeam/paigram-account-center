Set-StrictMode -Version Latest

function New-OrReplaceRotationSecret {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Path,
        [switch]$Replace
    )
    $arguments = @("secret", "create")
    if ($Replace) {
        $arguments += "--replace"
    }
    $arguments += @($Name, $Path)
    & podman @arguments *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Could not provision rehearsal secret $Name"
    }
}

function New-RotationComposeFile {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Image,
        [Parameter(Mandatory)][hashtable]$Containers,
        [Parameter(Mandatory)][hashtable]$Secrets
    )
    $accountSecrets = @($Secrets.TicketSigning, $Secrets.ControlTrust, $Secrets.ControlClientCertificate, $Secrets.ControlClientKey, $Secrets.AccountServerCertificate, $Secrets.AccountServerKey)
    $platformSecrets = @($Secrets.TicketPublicKeyring, $Secrets.EncryptionKeyring, $Secrets.ControlTrust, $Secrets.ControlServerCertificate, $Secrets.ControlServerKey, $Secrets.RuntimeTrust, $Secrets.RuntimeServerCertificate, $Secrets.RuntimeServerKey)
    $consumerSecrets = @($Secrets.RuntimeTrust, $Secrets.AccountTrust)
    $lines = @("services:")
    foreach ($service in @(
        @{ Name = "account-probe"; Container = $Containers.Account; Secrets = $accountSecrets },
        @{ Name = "platform-probe"; Container = $Containers.Platform; Secrets = $platformSecrets },
        @{ Name = "consumer-probe"; Container = $Containers.Consumer; Secrets = $consumerSecrets }
    )) {
        $lines += "  $($service.Name):"
        $lines += "    image: $Image"
        $lines += "    container_name: $($service.Container)"
        $lines += '    command: ["sleep", "3600"]'
        $lines += "    secrets:"
        foreach ($secret in $service.Secrets) {
            $lines += "      - $secret"
        }
    }
    $lines += "secrets:"
    foreach ($secret in $Secrets.Values) {
        $lines += "  ${secret}:"
        $lines += "    external: true"
    }
    [System.IO.File]::WriteAllLines($Path, $lines, [System.Text.UTF8Encoding]::new($false))
}

function Invoke-RotationRecreate {
    param(
        [Parameter(Mandatory)][string]$PodmanCompose,
        [Parameter(Mandatory)][string]$Project,
        [Parameter(Mandatory)][string]$ComposeFile
    )
    & $PodmanCompose -p $Project -f $ComposeFile up -d --force-recreate *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Could not force-recreate rotation rehearsal consumers through podman-compose"
    }
}

function Get-RotationMountedSHA256 {
    param(
        [Parameter(Mandatory)][string]$Container,
        [Parameter(Mandatory)][string]$Target
    )
    $output = ((@(& podman exec $Container sha256sum "/run/secrets/$Target")) -join "`n").Trim()
    if ($LASTEXITCODE -ne 0 -or $output -notmatch '^([0-9a-f]{64})\s') {
        throw "Could not hash mounted secret $Target in $Container"
    }
    return $Matches[1]
}

function Assert-RotationMountedSecrets {
    param(
        [Parameter(Mandatory)][string]$Container,
        [Parameter(Mandatory)][hashtable]$Expected
    )
    foreach ($target in $Expected.Keys) {
        $actualHash = Get-RotationMountedSHA256 -Container $Container -Target $target
        $expectedHash = (Get-FileHash -LiteralPath $Expected[$target] -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actualHash -ne $expectedHash) {
            throw "Mounted secret $target in $Container does not match the expected rotation stage"
        }
    }
}

function Assert-NoRotationSecretDisclosure {
    param(
        [Parameter(Mandatory)][string[]]$Containers,
        [Parameter(Mandatory)][string[]]$SensitiveNeedles
    )
    $diagnostics = (@(& podman inspect @Containers) -join "`n")
    if ($LASTEXITCODE -ne 0) {
        throw "Could not inspect rehearsal containers"
    }
    foreach ($container in $Containers) {
        $diagnostics += "`n" + (@(& podman logs $container) -join "`n")
        if ($LASTEXITCODE -ne 0) {
            throw "Could not read rehearsal container logs"
        }
    }
    foreach ($needle in $SensitiveNeedles) {
        if ($needle.Length -ge 16 -and $diagnostics.Contains($needle, [StringComparison]::Ordinal)) {
            throw "A secret payload was exposed through container inspect, environment, or logs"
        }
    }
}

Export-ModuleMember -Function New-OrReplaceRotationSecret, New-RotationComposeFile, Invoke-RotationRecreate, Assert-RotationMountedSecrets, Assert-NoRotationSecretDisclosure
