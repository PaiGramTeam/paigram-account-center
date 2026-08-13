Set-StrictMode -Version Latest

function Invoke-ImmutableComposeDeployment {
    param(
        [Parameter(Mandatory)][string]$PodmanCompose,
        [Parameter(Mandatory)][string[]]$ComposeArguments,
        [Parameter(Mandatory)][string]$FailureMessage,
        [string]$PodmanCommand = "podman",
        [string]$ProjectName = "",
        [string[]]$InfrastructureServices = @(),
        [string[]]$BootstrapServices = @()
    )
    if ($BootstrapServices.Count -gt 0) {
        if ($InfrastructureServices.Count -eq 0) {
            throw "InfrastructureServices are required when BootstrapServices are configured"
        }
        if ([string]::IsNullOrWhiteSpace($ProjectName)) {
            throw "ProjectName is required when BootstrapServices are configured"
        }
        Invoke-ComposeCommand `
            -PodmanCompose $PodmanCompose `
            -Arguments ($ComposeArguments + @("--profile", "bootstrap", "down")) `
            -FailureMessage "$FailureMessage while stopping the previous release"
        Assert-ComposeProjectStopped `
            -PodmanCommand $PodmanCommand `
            -ProjectName $ProjectName `
            -FailureMessage "$FailureMessage because the previous release is still running"
        Invoke-ComposeCommand `
            -PodmanCompose $PodmanCompose `
            -Arguments ($ComposeArguments + @("up", "--no-build", "-d", "--force-recreate", "--wait", "--wait-timeout", "120") + $InfrastructureServices) `
            -FailureMessage "$FailureMessage while starting storage"
        foreach ($service in $BootstrapServices) {
            Invoke-ComposeCommand `
                -PodmanCompose $PodmanCompose `
                -Arguments ($ComposeArguments + @("--profile", "bootstrap", "run", "--rm", "-T", "--no-deps", $service)) `
                -FailureMessage "$FailureMessage while running $service"
        }
    }

    Invoke-ComposeCommand `
        -PodmanCompose $PodmanCompose `
        -Arguments ($ComposeArguments + @("up", "--no-build", "-d", "--force-recreate")) `
        -FailureMessage $FailureMessage
}

function Assert-ComposeProjectStopped {
    param(
        [Parameter(Mandatory)][string]$PodmanCommand,
        [Parameter(Mandatory)][string]$ProjectName,
        [Parameter(Mandatory)][string]$FailureMessage
    )
    $global:LASTEXITCODE = 0
    $remainingContainers = @(& $PodmanCommand ps -aq --filter "label=io.podman.compose.project=$ProjectName")
    if ($LASTEXITCODE -ne 0) {
        throw "$FailureMessage; container inspection failed"
    }
    if ($remainingContainers.Count -gt 0) {
        throw $FailureMessage
    }
}

function Invoke-ComposeCommand {
    param(
        [Parameter(Mandatory)][string]$PodmanCompose,
        [Parameter(Mandatory)][string[]]$Arguments,
        [Parameter(Mandatory)][string]$FailureMessage
    )
    $global:LASTEXITCODE = 0
    & $PodmanCompose @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw $FailureMessage
    }
}

Export-ModuleMember -Function Invoke-ImmutableComposeDeployment
