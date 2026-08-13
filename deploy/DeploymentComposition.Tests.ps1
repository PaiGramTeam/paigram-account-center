#requires -Version 7.4

Import-Module (Join-Path $PSScriptRoot "DeploymentComposition.psm1") -Force

Describe "Immutable Compose deployment" {
    BeforeEach {
        $binRoot = Join-Path $TestDrive $([Guid]::NewGuid().ToString("N"))
        New-Item -ItemType Directory -Path $binRoot | Out-Null
        $composeCommand = Join-Path $binRoot "podman-compose.ps1"
        $podmanCommand = Join-Path $binRoot "podman.ps1"
        @'
$args -join " " | Add-Content -LiteralPath $env:PAI_DEPLOYMENT_COMPOSE_ARGS
if ($env:PAI_DEPLOYMENT_COMPOSE_FAIL -eq "1") { exit 23 }
'@ | Set-Content -LiteralPath $composeCommand
        @'
if ($env:PAI_DEPLOYMENT_REMAINING_CONTAINER -eq "1") { "container-id" }
'@ | Set-Content -LiteralPath $podmanCommand
        $env:PAI_DEPLOYMENT_COMPOSE_ARGS = Join-Path $binRoot "arguments.txt"
    }

    AfterEach {
        Remove-Item Env:PAI_DEPLOYMENT_COMPOSE_ARGS, Env:PAI_DEPLOYMENT_COMPOSE_FAIL, Env:PAI_DEPLOYMENT_REMAINING_CONTAINER -ErrorAction SilentlyContinue
    }

    It "always force-recreates immutable services" {
        Invoke-ImmutableComposeDeployment `
            -PodmanCompose $composeCommand `
            -ComposeArguments @("--env-file", ".env", "-p", "recovery", "-f", "compose.yaml") `
            -FailureMessage "deployment failed"

        (Get-Content -LiteralPath $env:PAI_DEPLOYMENT_COMPOSE_ARGS -Raw).Trim() | Should Be "--env-file .env -p recovery -f compose.yaml up --no-build -d --force-recreate"
    }

    It "fails when the Compose provider fails" {
        $env:PAI_DEPLOYMENT_COMPOSE_FAIL = "1"
        try {
            Invoke-ImmutableComposeDeployment `
                -PodmanCompose $composeCommand `
                -ComposeArguments @("-p", "recovery") `
                -FailureMessage "deployment failed"
            throw "Expected deployment failure"
        } catch {
            $_.Exception.Message | Should Be "deployment failed"
        }
    }

    It "runs bootstrap jobs serially before starting application services" {
        Invoke-ImmutableComposeDeployment `
            -PodmanCompose $composeCommand `
            -ComposeArguments @("--env-file", ".env", "-p", "recovery", "-f", "compose.yaml") `
            -FailureMessage "deployment failed" `
            -PodmanCommand $podmanCommand `
            -ProjectName "recovery" `
            -InfrastructureServices @("postgres", "redis") `
            -BootstrapServices @("migrate", "seed")

        @(Get-Content -LiteralPath $env:PAI_DEPLOYMENT_COMPOSE_ARGS) | Should Be @(
            "--env-file .env -p recovery -f compose.yaml --profile bootstrap down",
            "--env-file .env -p recovery -f compose.yaml up --no-build -d --force-recreate --wait --wait-timeout 120 postgres redis",
            "--env-file .env -p recovery -f compose.yaml --profile bootstrap run --rm -T --no-deps migrate",
            "--env-file .env -p recovery -f compose.yaml --profile bootstrap run --rm -T --no-deps seed",
            "--env-file .env -p recovery -f compose.yaml up --no-build -d --force-recreate"
        )
    }

    It "stops when a bootstrap job fails" {
        $failingCommand = Join-Path (Split-Path -Parent $composeCommand) "failing-compose.ps1"
        @'
$args -join " " | Add-Content -LiteralPath $env:PAI_DEPLOYMENT_COMPOSE_ARGS
if ($args -contains "migrate") { exit 31 }
'@ | Set-Content -LiteralPath $failingCommand

        try {
            Invoke-ImmutableComposeDeployment `
                -PodmanCompose $failingCommand `
                -ComposeArguments @("-p", "recovery") `
                -FailureMessage "deployment failed" `
                -PodmanCommand $podmanCommand `
                -ProjectName "recovery" `
                -InfrastructureServices @("postgres") `
                -BootstrapServices @("migrate")
            throw "Expected migration failure"
        } catch {
            $_.Exception.Message | Should Be "deployment failed while running migrate"
        }
        @(Get-Content -LiteralPath $env:PAI_DEPLOYMENT_COMPOSE_ARGS)[-1] | Should Match 'migrate$'
    }

    It "fails when Compose leaves project containers running" {
        $env:PAI_DEPLOYMENT_REMAINING_CONTAINER = "1"

        try {
            Invoke-ImmutableComposeDeployment `
                -PodmanCompose $composeCommand `
                -ComposeArguments @("-p", "recovery") `
                -FailureMessage "deployment failed" `
                -PodmanCommand $podmanCommand `
                -ProjectName "recovery" `
                -InfrastructureServices @("postgres") `
                -BootstrapServices @("migrate")
            throw "Expected stale container failure"
        } catch {
            $_.Exception.Message | Should Be "deployment failed because the previous release is still running"
        }
        @(Get-Content -LiteralPath $env:PAI_DEPLOYMENT_COMPOSE_ARGS) | Should Be @("-p recovery --profile bootstrap down")
    }
}
