#requires -Version 7.4

Describe "Platform registry descriptor registration" {
    BeforeEach {
        $tokenRoot = Join-Path $TestDrive $([Guid]::NewGuid().ToString("N"))
        New-Item -ItemType Directory -Path $tokenRoot | Out-Null
        $tokenFile = Join-Path $tokenRoot "admin-token.txt"
        Set-Content -LiteralPath $tokenFile -Value "temporary-admin-token" -NoNewline
        if ($IsWindows) {
            $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
            & icacls $tokenFile /inheritance:r /grant:r "${identity}:(F)" *> $null
            if ($LASTEXITCODE -ne 0) { throw "Could not restrict test token permissions" }
        } else {
            [System.IO.File]::SetUnixFileMode(
                $tokenFile,
                [System.IO.UnixFileMode]::UserRead -bor [System.IO.UnixFileMode]::UserWrite
            )
        }
        Mock Invoke-RestMethod {
            if ($Method -eq "Get") {
                return @{ data = @() }
            }
            return @{}
        }
    }

    It "accepts explicitly authorized loopback HTTP and consumes the token" {
        & (Join-Path $PSScriptRoot "register-descriptor.ps1") `
            -AccountCenterUrl "http://127.0.0.1:18080" `
            -AdminAccessTokenFile $tokenFile `
            -RuntimeEndpoint "127.0.0.1:19001" `
            -RuntimeServerName "platform-runtime.internal" `
            -AllowLoopbackHTTP

        Test-Path -LiteralPath $tokenFile | Should Be $false
        Assert-MockCalled Invoke-RestMethod -Times 2 -Exactly
    }

    It "consumes the token when the service URL policy rejects the request" {
        $output = @(& pwsh -NoProfile -File (Join-Path $PSScriptRoot "register-descriptor.ps1") `
            -AccountCenterUrl "http://account.example.test" `
            -AdminAccessTokenFile $tokenFile `
            -RuntimeEndpoint "127.0.0.1:19001" `
            -RuntimeServerName "platform-runtime.internal" `
            -AllowLoopbackHTTP 2>&1)

        $LASTEXITCODE | Should Not Be 0
        $output -join "`n" | Should Match "must use HTTPS"
        Test-Path -LiteralPath $tokenFile | Should Be $false
    }
}
