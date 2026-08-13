#requires -Version 7.4

Describe "Restored release verification" {
    BeforeEach {
        $accountInstance = "recovery-account"
        $platformInstance = "recovery-platform"
        $platformNetwork = "recovery-platform-backplane"
        $caseName = [Guid]::NewGuid().ToString("N")
        $recoveredRoot = Join-Path $TestDrive "recovered-$caseName"
        $privateRoot = Join-Path $TestDrive "private-$caseName"
        $binRoot = Join-Path $TestDrive "bin-$caseName"
        New-Item -ItemType Directory -Path $recoveredRoot, $privateRoot, $binRoot -Force | Out-Null

        $recoveredFiles = @(
            "account-encryption-key",
            "account-service-ticket-signing-key.json",
            "platform-encryption-keyring.json",
            "account-service-ticket-public-keyring.json"
        )
        foreach ($file in $recoveredFiles) {
            Set-Content -LiteralPath (Join-Path $recoveredRoot $file) -Value "recovered-$file" -NoNewline
        }
        $tracerConfig = Join-Path $privateRoot "tracer.json"
        $evidenceFile = Join-Path $privateRoot "release-recovery-evidence.json"
        Set-Content -LiteralPath $tracerConfig -Value '{}' -NoNewline
        if ($IsWindows) {
            $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
            & icacls $tracerConfig /inheritance:r /grant:r "${identity}:(F)" *> $null
            if ($LASTEXITCODE -ne 0) {
                throw "Could not restrict the test tracer config"
            }
        } else {
            [System.IO.File]::SetUnixFileMode(
                $tracerConfig,
                [System.IO.UnixFileMode]::UserRead -bor [System.IO.UnixFileMode]::UserWrite
            )
        }

        $manifest = [ordered]@{
            format_version = 1
            account_instance = $accountInstance
            platform_instance = $platformInstance
            migrations = [ordered]@{ account = "1:false"; platform = "1:false" }
            images = [ordered]@{
                account = [ordered]@{
                    container = "source-account"
                    reference = "localhost/recovery-account@sha256:$('1' * 64)"
                    source_commit = "a" * 40
                    contract_baseline = "b" * 40
                    sdk_version = "0.1.0"
                }
                frontend = [ordered]@{
                    container = "source-frontend"
                    reference = "localhost/recovery-frontend@sha256:$('2' * 64)"
                    source_commit = "a" * 40
                    contract_baseline = "b" * 40
                    sdk_version = "0.1.0"
                }
                platform = [ordered]@{
                    container = "source-platform"
                    reference = "localhost/recovery-platform@sha256:$('3' * 64)"
                    source_commit = "a" * 40
                    contract_baseline = "b" * 40
                    sdk_version = "0.1.0"
                }
            }
        }
        $manifestPath = Join-Path $recoveredRoot "recovery-manifest.json"
        $manifest | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $manifestPath

        @'
$arguments = @($args)
$arguments -join " " | Add-Content -LiteralPath (Join-Path $env:PAI_RECOVERY_TEST_ROOT "podman-invocations.log")
$manifest = Get-Content -LiteralPath (Join-Path $env:PAI_RECOVERY_TEST_ROOT "recovery-manifest.json") -Raw | ConvertFrom-Json
if ($arguments[0] -eq "inspect") {
    $name = $arguments[1]
    $image = if ($name -eq $env:PAI_RECOVERY_ACCOUNT) {
        $manifest.images.account
    } elseif ($name -eq "$env:PAI_RECOVERY_ACCOUNT-frontend") {
        $manifest.images.frontend
    } else {
        $manifest.images.platform
    }
    $project = if ($name -like "$env:PAI_RECOVERY_ACCOUNT*") { $env:PAI_RECOVERY_ACCOUNT } else { $env:PAI_RECOVERY_PLATFORM }
    $isPostgres = $name -like "*-postgres"
    $service = if ($isPostgres) {
        "postgres"
    } elseif ($name -like "*-frontend") {
        "frontend"
    } elseif ($name -eq $env:PAI_RECOVERY_ACCOUNT) {
        "account-center"
    } else {
        "platform-mihomo"
    }
    $networkNames = if ($isPostgres) {
        if ($project -eq $env:PAI_RECOVERY_ACCOUNT) { @($project) } else { @("$project-private") }
    } elseif ($project -eq $env:PAI_RECOVERY_ACCOUNT) {
        @($project, $env:PAI_RECOVERY_PLATFORM_NETWORK)
    } else {
        @("$project-private", $env:PAI_RECOVERY_PLATFORM_NETWORK)
    }
    if (Test-Path -LiteralPath (Join-Path $env:PAI_RECOVERY_TEST_ROOT "wrong-network")) {
        $networkNames = @("unrelated-network")
    }
    $networks = [ordered]@{}
    foreach ($networkName in $networkNames) { $networks[$networkName] = [ordered]@{} }
    $mountName = if ($isPostgres) { "$project-postgres-data" } else { "" }
    if ($isPostgres -and (Test-Path -LiteralPath (Join-Path $env:PAI_RECOVERY_TEST_ROOT "wrong-volume"))) {
        $mountName = "unrelated-postgres-data"
    }
    @([ordered]@{
        State = [ordered]@{ Running = $true; Health = [ordered]@{ Status = "healthy" } }
        ImageName = $image.reference
        Config = [ordered]@{ Labels = [ordered]@{
            'org.opencontainers.image.revision' = $image.source_commit
            'org.paigram.contract-baseline' = $image.contract_baseline
            'org.paigram.sdk-version' = $image.sdk_version
            'com.docker.compose.project' = $project
            'com.docker.compose.service' = $service
        } }
        Mounts = if ($isPostgres) { @([ordered]@{ Type = "volume"; Name = $mountName; Destination = "/var/lib/postgresql/data" }) } else { @() }
        NetworkSettings = [ordered]@{ Networks = $networks }
    }) | ConvertTo-Json -Depth 6 -Compress
    return
}
if ($arguments[0] -eq "port") {
    if ($arguments[1] -eq "$env:PAI_RECOVERY_ACCOUNT-frontend") {
        "127.0.0.1:18080"
    } elseif ($arguments[1] -eq $env:PAI_RECOVERY_ACCOUNT) {
        "127.0.0.1:15051"
    } else {
        if (Test-Path -LiteralPath (Join-Path $env:PAI_RECOVERY_TEST_ROOT "remote-runtime-port")) {
            "0.0.0.0:19001"
        } else {
            "127.0.0.1:19001"
        }
    }
    return
}
if ($arguments[0] -eq "run") {
    if (($arguments -join " ") -match "platform_services") {
        "127.0.0.1:19001|platform-runtime.internal"
    } else {
        "1:false"
    }
    return
}
if ($arguments[0] -eq "exec" -and $arguments -contains "wget") {
    if (Test-Path -LiteralPath (Join-Path $env:PAI_RECOVERY_TEST_ROOT "fail-health")) {
        exit 19
    }
    '{"code":200,"data":{"status":"ok"},"message":"success"}'
    return
}
if ($arguments[0] -eq "exec" -and ($arguments -join " ") -match "database-dsn") {
    if (Test-Path -LiteralPath (Join-Path $env:PAI_RECOVERY_TEST_ROOT "wrong-dsn")) {
        exit 21
    }
    if (Test-Path -LiteralPath (Join-Path $env:PAI_RECOVERY_TEST_ROOT "query-override-dsn")) {
        exit 22
    }
    if ($arguments -contains "/run/secrets/$env:PAI_RECOVERY_ACCOUNT-database-dsn") {
        "postgres|5432|paigram"
    } else {
        "postgres|5432|platform_mihomo"
    }
    return
}
if ($arguments[0] -eq "exec" -and ($arguments -join " ") -match "platform-mihomo-healthcheck") {
    return
}
if ($arguments[0] -eq "exec" -and $arguments -contains "sha256sum") {
    $mountedPath = $arguments[-1]
    $fileName = [System.IO.Path]::GetFileName($mountedPath)
    if ($fileName -eq "$env:PAI_RECOVERY_ACCOUNT-encryption-key") {
        $fileName = "account-encryption-key"
    } elseif ($fileName -eq "$env:PAI_RECOVERY_ACCOUNT-service-ticket-signing-key") {
        $fileName = "account-service-ticket-signing-key.json"
    } elseif ($fileName -eq "$env:PAI_RECOVERY_PLATFORM-encryption-keyring") {
        $fileName = "platform-encryption-keyring.json"
    } else {
        $fileName = "account-service-ticket-public-keyring.json"
    }
    $hash = (Get-FileHash -LiteralPath (Join-Path $env:PAI_RECOVERY_TEST_ROOT $fileName) -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $mountedPath"
    return
}
throw "Unexpected podman invocation: $($arguments -join ' ')"
'@ | Set-Content -LiteralPath (Join-Path $binRoot "podman.ps1")
        @'
#!/usr/bin/env pwsh
& (Join-Path $PSScriptRoot "podman.ps1") @args
exit $LASTEXITCODE
'@ | Set-Content -LiteralPath (Join-Path $binRoot "podman")
@'
if ($args -contains "paigram_account_sdk.recovery_tracer" -and
    (Test-Path -LiteralPath (Join-Path $env:PAI_RECOVERY_TEST_ROOT "fail-tracer"))) {
    exit 17
}
Write-Output '{"status":"passed","binding_ref":"binding-ref"}'
'@ | Set-Content -LiteralPath (Join-Path $binRoot "uv.ps1")
        @'
#!/usr/bin/env pwsh
& (Join-Path $PSScriptRoot "uv.ps1") @args
exit $LASTEXITCODE
'@ | Set-Content -LiteralPath (Join-Path $binRoot "uv")
        @'
if ($args -contains "rev-parse") {
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
} elseif ($args -contains "status") {
    $allowFile = Join-Path $env:PAI_RECOVERY_TEST_ROOT "allow-dirty-verifier"
    if (Test-Path -LiteralPath $allowFile) {
        " M sdks/python/src/paigram_account_sdk/recovery_tracer.py"
    }
}
'@ | Set-Content -LiteralPath (Join-Path $binRoot "git.ps1")
        @'
#!/usr/bin/env pwsh
& (Join-Path $PSScriptRoot "git.ps1") @args
exit $LASTEXITCODE
'@ | Set-Content -LiteralPath (Join-Path $binRoot "git")
        if (-not $IsWindows) {
            foreach ($command in @("podman", "uv", "git")) {
                [System.IO.File]::SetUnixFileMode(
                    (Join-Path $binRoot $command),
                    [System.IO.UnixFileMode]::UserRead -bor
                    [System.IO.UnixFileMode]::UserWrite -bor
                    [System.IO.UnixFileMode]::UserExecute
                )
            }
        }

        $script:originalPath = $env:PATH
        $env:PATH = "$binRoot$([System.IO.Path]::PathSeparator)$env:PATH"
        $env:PAI_RECOVERY_TEST_ROOT = $recoveredRoot
        $env:PAI_RECOVERY_ACCOUNT = $accountInstance
        $env:PAI_RECOVERY_PLATFORM = $platformInstance
        $env:PAI_RECOVERY_PLATFORM_NETWORK = $platformNetwork
    }

    AfterEach {
        $env:PATH = $script:originalPath
        Remove-Item Env:PAI_RECOVERY_TEST_ROOT, Env:PAI_RECOVERY_ACCOUNT, Env:PAI_RECOVERY_PLATFORM, Env:PAI_RECOVERY_PLATFORM_NETWORK -ErrorAction SilentlyContinue
    }

    It "accepts matching healthy manifest containers and mounted recovery keys" {
        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        if ($LASTEXITCODE -ne 0) {
            throw ($output -join "`n")
        }
        $output -join "`n" | Should Match "Recovered release verification passed"
        Test-Path -LiteralPath $evidenceFile | Should Be $true
        $invocations = Get-Content -LiteralPath (Join-Path $recoveredRoot "podman-invocations.log") -Raw
        $invocations | Should Match ([regex]::Escape("wget -q -O - http://account-center:8080/livez"))
        $invocations | Should Match ([regex]::Escape("wget -q -O - http://account-center:8080/readyz"))
        $invocations | Should Match "platform-mihomo-healthcheck.*-timeout 5s(\r?\n|$)"
        $invocations | Should Match "platform-mihomo-healthcheck.*-service liveness"
    }

    It "rejects a mutable manifest image before invoking the tracer" {
        $manifest.images.account.reference = "localhost/recovery-account:latest"
        $manifest | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $manifestPath

        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "not pinned by digest"
    }

    It "rejects an uncommitted verifier or SDK checkout" {
        Set-Content -LiteralPath (Join-Path $recoveredRoot "allow-dirty-verifier") -Value "1"

        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "must match the recorded source commit without local changes"
    }

    It "rejects a recovery target that publishes the runtime listener beyond loopback" {
        Set-Content -LiteralPath (Join-Path $recoveredRoot "remote-runtime-port") -Value "1"

        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "must have exactly one loopback publication"
    }

    It "rejects an existing evidence path without replacing its prior result" {
        Set-Content -LiteralPath $evidenceFile -Value '{"tracer":"passed","prior":true}' -NoNewline

        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "must be a new path"
        Get-Content -LiteralPath $evidenceFile -Raw | Should Be '{"tracer":"passed","prior":true}'
    }

    It "does not write evidence when the restored release tracer fails" {
        Set-Content -LiteralPath (Join-Path $recoveredRoot "fail-tracer") -Value "1"

        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "Recovered release tracer failed"
        Test-Path -LiteralPath $evidenceFile | Should Be $false
    }

    It "rejects a private tracer config stored inside the repository" {
        $repositoryTracer = Join-Path (Resolve-Path (Join-Path $PSScriptRoot "../..")) "private-recovery-tracer.json"
        Set-Content -LiteralPath $repositoryTracer -Value '{}' -NoNewline
        try {
            $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
                -RecoveredSecretsDirectory $recoveredRoot `
                -TracerConfigFile $repositoryTracer `
                -EvidenceFile $evidenceFile `
                -AccountInstance $accountInstance `
                -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

            $LASTEXITCODE | Should Not Be 0
            $output -join "`n" | Should Match "must be outside the repository"
            Test-Path -LiteralPath $evidenceFile | Should Be $false
        } finally {
            Remove-Item -LiteralPath $repositoryTracer -Force
        }
    }

    It "rejects a private tracer config reached through a linked parent directory" {
        $repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
        $repositoryTracer = Join-Path $repositoryRoot "private-recovery-tracer.json"
        $linkedRepository = Join-Path $TestDrive "linked-repository-$caseName"
        Set-Content -LiteralPath $repositoryTracer -Value '{}' -NoNewline
        try {
            if ($IsWindows) {
                New-Item -ItemType Junction -Path $linkedRepository -Target $repositoryRoot | Out-Null
            } else {
                New-Item -ItemType SymbolicLink -Path $linkedRepository -Target $repositoryRoot | Out-Null
            }
            $linkedTracer = Join-Path $linkedRepository "private-recovery-tracer.json"
            $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
                -RecoveredSecretsDirectory $recoveredRoot `
                -TracerConfigFile $linkedTracer `
                -EvidenceFile $evidenceFile `
                -AccountInstance $accountInstance `
                -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

            $LASTEXITCODE | Should Not Be 0
            $output -join "`n" | Should Match "must not contain symbolic links or junctions"
            Test-Path -LiteralPath $evidenceFile | Should Be $false
        } finally {
            Remove-Item -LiteralPath $linkedRepository -Force -ErrorAction SilentlyContinue
            Remove-Item -LiteralPath $repositoryTracer -Force -ErrorAction SilentlyContinue
        }
    }

    It "rejects an evidence directory linked into recovered secrets" {
        $linkedEvidenceParent = Join-Path $TestDrive "linked-evidence-$caseName"
        try {
            if ($IsWindows) {
                New-Item -ItemType Junction -Path $linkedEvidenceParent -Target $recoveredRoot | Out-Null
            } else {
                New-Item -ItemType SymbolicLink -Path $linkedEvidenceParent -Target $recoveredRoot | Out-Null
            }
            $linkedEvidence = Join-Path $linkedEvidenceParent "release-evidence.json"
            $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
                -RecoveredSecretsDirectory $recoveredRoot `
                -TracerConfigFile $tracerConfig `
                -EvidenceFile $linkedEvidence `
                -AccountInstance $accountInstance `
                -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

            $LASTEXITCODE | Should Not Be 0
            $output -join "`n" | Should Match "must not contain symbolic links or junctions"
            Test-Path -LiteralPath $linkedEvidence | Should Be $false
        } finally {
            Remove-Item -LiteralPath $linkedEvidenceParent -Force -ErrorAction SilentlyContinue
        }
    }

    It "restores pre-existing recovery tracer environment variables" {
        $env:PAI_RECOVERY_ACCOUNT_HTTP_URL = "sentinel-http"
        $env:PAI_RECOVERY_ACCOUNT_GRPC_TARGET = "sentinel-grpc"
        try {
            & (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
                -RecoveredSecretsDirectory $recoveredRoot `
                -TracerConfigFile $tracerConfig `
                -EvidenceFile $evidenceFile `
                -AccountInstance $accountInstance `
                -PlatformInstance $platformInstance `
                -PlatformNetwork $platformNetwork *> $null

            $env:PAI_RECOVERY_ACCOUNT_HTTP_URL | Should Be "sentinel-http"
            $env:PAI_RECOVERY_ACCOUNT_GRPC_TARGET | Should Be "sentinel-grpc"
        } finally {
            Remove-Item Env:PAI_RECOVERY_ACCOUNT_HTTP_URL, Env:PAI_RECOVERY_ACCOUNT_GRPC_TARGET -ErrorAction SilentlyContinue
        }
    }

    It "does not write evidence when a health probe fails" {
        Set-Content -LiteralPath (Join-Path $recoveredRoot "fail-health") -Value "1"

        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "Account health probe failed"
        Test-Path -LiteralPath $evidenceFile | Should Be $false
    }

    It "rejects a PostgreSQL container using the wrong restored volume" {
        Set-Content -LiteralPath (Join-Path $recoveredRoot "wrong-volume") -Value "1"

        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "expected restored PostgreSQL volume"
        Test-Path -LiteralPath $evidenceFile | Should Be $false
    }

    It "rejects an application connected to the wrong PostgreSQL target" {
        Set-Content -LiteralPath (Join-Path $recoveredRoot "wrong-dsn") -Value "1"

        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "database DSN does not target its restored PostgreSQL service"
        Test-Path -LiteralPath $evidenceFile | Should Be $false
    }

    It "rejects PostgreSQL query parameters that can override the restored target" {
        Set-Content -LiteralPath (Join-Path $recoveredRoot "query-override-dsn") -Value "1"

        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "database DSN does not target its restored PostgreSQL service"
        Test-Path -LiteralPath $evidenceFile | Should Be $false
    }

    It "rejects a restored service attached to an unexpected network" {
        Set-Content -LiteralPath (Join-Path $recoveredRoot "wrong-network") -Value "1"

        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-restored-release.ps1") `
            -RecoveredSecretsDirectory $recoveredRoot `
            -TracerConfigFile $tracerConfig `
            -EvidenceFile $evidenceFile `
            -AccountInstance $accountInstance `
            -PlatformInstance $platformInstance `
            -PlatformNetwork $platformNetwork 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "expected recovery networks"
        Test-Path -LiteralPath $evidenceFile | Should Be $false
    }
}
