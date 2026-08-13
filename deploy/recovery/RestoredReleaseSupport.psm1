Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Import-Module (Join-Path $PSScriptRoot "../OperationalSecurity.psm1") -Force

function Assert-RecoveryCommand {
    param([Parameter(Mandatory)][string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name is required"
    }
}

function Assert-RecoveryInstanceName {
    param([Parameter(Mandatory)][string]$Name)
    if ($Name -notmatch '^[a-z0-9][a-z0-9-]{0,62}$') {
        throw "Invalid instance name: $Name"
    }
}

function Assert-RecoveryImageReference {
    param([Parameter(Mandatory)][string]$Reference)
    if ($Reference -notmatch '^\S+@sha256:[0-9a-f]{64}$') {
        throw "Recovery manifest image is not pinned by digest: $Reference"
    }
}

function Assert-RecoveryPrivateFile {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$RepositoryRoot
    )
    return Assert-OperationalPrivateFile -Path $Path -RepositoryRoot $RepositoryRoot
}

function Assert-RecoveryPathHasNoLinks {
    param([Parameter(Mandatory)][string]$Path)
    Assert-OperationalPathHasNoLinks -Path $Path
}

function Invoke-RecoveryPodmanText {
    param(
        [Parameter(Mandatory)][string[]]$Arguments,
        [Parameter(Mandatory)][string]$FailureMessage
    )
    $global:LASTEXITCODE = 0
    $output = @(& podman @Arguments)
    if ($LASTEXITCODE -ne 0) {
        throw $FailureMessage
    }
    return ($output -join "`n").Trim()
}

function Assert-RecoveryEqual {
    param(
        [Parameter(Mandatory)]$Actual,
        [Parameter(Mandatory)]$Expected,
        [Parameter(Mandatory)][string]$Message
    )
    if ([string]$Actual -cne [string]$Expected) {
        throw "$Message did not match the recovery manifest"
    }
}

function Get-RecoveryContainerMetadata {
    param([Parameter(Mandatory)][string]$Name)
    $inspect = Invoke-RecoveryPodmanText -Arguments @("inspect", $Name) -FailureMessage "Could not inspect $Name"
    $records = @($inspect | ConvertFrom-Json)
    if ($records.Count -ne 1) {
        throw "Expected one container named $Name"
    }
    $record = $records[0]
    if ($record.State.Running -ne $true -or [string]$record.State.Health.Status -ne "healthy") {
        throw "Recovered application container is not healthy: $Name"
    }
    return [ordered]@{
        reference = [string]$record.ImageName
        source_commit = [string]$record.Config.Labels.'org.opencontainers.image.revision'
        contract_baseline = [string]$record.Config.Labels.'org.paigram.contract-baseline'
        sdk_version = [string]$record.Config.Labels.'org.paigram.sdk-version'
    }
}

function Assert-RecoveryContainerMetadata {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)]$Expected
    )
    $actual = Get-RecoveryContainerMetadata -Name $Name
    Assert-RecoveryEqual -Actual $actual.reference -Expected $Expected.reference -Message "$Name image reference"
    Assert-RecoveryEqual -Actual $actual.source_commit -Expected $Expected.source_commit -Message "$Name source commit"
    Assert-RecoveryEqual -Actual $actual.contract_baseline -Expected $Expected.contract_baseline -Message "$Name contract baseline"
    Assert-RecoveryEqual -Actual $actual.sdk_version -Expected $Expected.sdk_version -Message "$Name SDK version"
}

function Assert-RecoveryContainerDeployment {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Project,
        [Parameter(Mandatory)][string]$Service,
        [Parameter(Mandatory)][string]$PostgresVolume,
        [Parameter(Mandatory)][string[]]$Networks
    )
    $inspect = Invoke-RecoveryPodmanText -Arguments @("inspect", $Name) -FailureMessage "Could not inspect $Name"
    $record = @($inspect | ConvertFrom-Json)[0]
    $labels = $record.Config.Labels
    Assert-RecoveryEqual -Actual $labels.'com.docker.compose.project' -Expected $Project -Message "$Name Compose project"
    Assert-RecoveryEqual -Actual $labels.'com.docker.compose.service' -Expected $Service -Message "$Name Compose service"
    $mounts = @($record.Mounts | Where-Object {
            $_.Type -eq "volume" -and $_.Destination -eq "/var/lib/postgresql/data"
        })
    if ($mounts.Count -ne 1 -or [string]$mounts[0].Name -cne $PostgresVolume) {
        throw "$Name does not use the expected restored PostgreSQL volume"
    }
    $actualNetworks = @($record.NetworkSettings.Networks.PSObject.Properties.Name | Sort-Object)
    $expectedNetworks = @($Networks | Sort-Object)
    if (($actualNetworks -join "`n") -cne ($expectedNetworks -join "`n")) {
        throw "$Name does not use the expected recovery networks"
    }
}

function Assert-RecoveryServiceConnection {
    param(
        [Parameter(Mandatory)][string]$Container,
        [Parameter(Mandatory)][string]$Project,
        [Parameter(Mandatory)][string]$Service,
        [Parameter(Mandatory)][string[]]$Networks,
        [Parameter(Mandatory)][string]$DSNFile,
        [Parameter(Mandatory)][string]$ExpectedDatabase
    )
    $inspect = Invoke-RecoveryPodmanText -Arguments @("inspect", $Container) -FailureMessage "Could not inspect $Container"
    $record = @($inspect | ConvertFrom-Json)[0]
    $labels = $record.Config.Labels
    Assert-RecoveryEqual -Actual $labels.'com.docker.compose.project' -Expected $Project -Message "$Container Compose project"
    Assert-RecoveryEqual -Actual $labels.'com.docker.compose.service' -Expected $Service -Message "$Container Compose service"
    $actualNetworks = @($record.NetworkSettings.Networks.PSObject.Properties.Name | Sort-Object)
    $expectedNetworks = @($Networks | Sort-Object)
    if (($actualNetworks -join "`n") -cne ($expectedNetworks -join "`n")) {
        throw "$Container does not use the expected recovery networks"
    }
    $output = Invoke-RecoveryPodmanText -Arguments @(
        "exec", $Container, "/usr/local/bin/recovery-dsn-verify", $DSNFile, $ExpectedDatabase
    ) -FailureMessage "$Container database DSN does not target its restored PostgreSQL service"
    Assert-RecoveryEqual -Actual $output -Expected "postgres|5432|$ExpectedDatabase" -Message "$Container database target"
}

function Get-RecoveryLoopbackPort {
    param(
        [Parameter(Mandatory)][string]$Container,
        [Parameter(Mandatory)][string]$ContainerPort
    )
    $mapping = Invoke-RecoveryPodmanText -Arguments @("port", $Container, $ContainerPort) -FailureMessage "Could not inspect published port $ContainerPort for $Container"
    $lines = @($mapping -split "`n" | Where-Object { $_ })
    if ($lines.Count -ne 1 -or $lines[0] -notmatch '^(127\.0\.0\.1|\[::1\]):([0-9]{1,5})$') {
        throw "$Container port $ContainerPort must have exactly one loopback publication"
    }
    $port = [int]$Matches[2]
    if ($port -lt 1 -or $port -gt 65535) {
        throw "$Container has an invalid published port for $ContainerPort"
    }
    return $port
}

function Assert-RecoveryAccountHealth {
    param(
        [Parameter(Mandatory)][string]$FrontendContainer,
        [Parameter(Mandatory)][string]$Path
    )
    $raw = Invoke-RecoveryPodmanText -Arguments @(
        "exec", $FrontendContainer,
        "wget", "-q", "-O", "-", "http://account-center:8080$Path"
    ) -FailureMessage "Account health probe failed for $Path"
    $response = $raw | ConvertFrom-Json
    if ($response.code -ne 200 -or $response.data.status -ne "ok") {
        throw "Account health probe returned an unexpected response for $Path"
    }
}

function Assert-RecoveryPlatformHealth {
    param(
        [Parameter(Mandatory)][string]$Container,
        [Parameter(Mandatory)][string]$PlatformInstance,
        [Parameter(Mandatory)][string]$ServerName,
        [Parameter(Mandatory)][AllowEmptyString()][string]$Service
    )
    $arguments = @(
        "exec", $Container,
        "/usr/local/bin/platform-mihomo-healthcheck",
        "-target", "127.0.0.1:9001",
        "-root-ca", "/run/secrets/$PlatformInstance-runtime-ca",
        "-server-name", $ServerName,
        "-timeout", "5s"
    )
    if ($Service) {
        $arguments += @("-service", $Service)
    }
    Invoke-RecoveryPodmanText -Arguments $arguments -FailureMessage "Platform health probe failed for service '$Service'" | Out-Null
}

Export-ModuleMember -Function @(
    "Assert-RecoveryCommand",
    "Assert-RecoveryInstanceName",
    "Assert-RecoveryImageReference",
    "Assert-RecoveryPathHasNoLinks",
    "Assert-RecoveryPrivateFile",
    "Invoke-RecoveryPodmanText",
    "Assert-RecoveryEqual",
    "Assert-RecoveryContainerMetadata",
    "Assert-RecoveryContainerDeployment",
    "Assert-RecoveryServiceConnection",
    "Get-RecoveryLoopbackPort",
    "Assert-RecoveryAccountHealth",
    "Assert-RecoveryPlatformHealth"
)
