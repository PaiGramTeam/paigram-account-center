#requires -Version 7.4

Import-Module (Join-Path $PSScriptRoot "OperationalSecurity.psm1") -Force

Describe "Operational service URL policy" {
    It "accepts HTTPS without an override" {
        $uri = Resolve-OperationalServiceURL -URL "https://account.example.test"
        $uri.AbsoluteUri | Should Be "https://account.example.test/"
    }

    It "rejects loopback HTTP without an explicit override" {
        try {
            Resolve-OperationalServiceURL -URL "http://127.0.0.1:18080" | Out-Null
            throw "Expected loopback HTTP to be rejected"
        } catch {
            $_.Exception.Message | Should Match "must use HTTPS"
        }
    }

    It "accepts loopback HTTP with an explicit override" {
        $uri = Resolve-OperationalServiceURL -URL "http://127.0.0.1:18080" -AllowLoopbackHTTP
        $uri.AbsoluteUri | Should Be "http://127.0.0.1:18080/"
    }

    It "rejects remote HTTP even with the loopback override" {
        try {
            Resolve-OperationalServiceURL -URL "http://account.example.test" -AllowLoopbackHTTP | Out-Null
            throw "Expected remote HTTP to be rejected"
        } catch {
            $_.Exception.Message | Should Match "must use HTTPS"
        }
    }
}
