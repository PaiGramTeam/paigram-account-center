$ErrorActionPreference = "Stop"

Push-Location $PSScriptRoot
try {
    buf lint
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }

    buf generate
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }

    Get-ChildItem -Path ./gen/go -Recurse -Filter *.go | ForEach-Object {
        gofmt -w $_.FullName
        if ($LASTEXITCODE -ne 0) {
            exit $LASTEXITCODE
        }
    }

    Push-Location ./gen/go
    try {
        go mod tidy
        if ($LASTEXITCODE -ne 0) {
            exit $LASTEXITCODE
        }
    }
    finally {
        Pop-Location
    }
}
finally {
    Pop-Location
}
