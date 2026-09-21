[CmdletBinding()]
param([switch]$SkipInstall)

$ErrorActionPreference = 'Stop'

try {
    foreach ($tool in @('node', 'npm.cmd', 'go')) {
        if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
            throw "Required tool not found: $tool"
        }
    }

    Push-Location $PSScriptRoot
    try {
        $targetOS = & go env GOOS
        if ($LASTEXITCODE -ne 0) { throw 'Could not determine the Go target platform.' }
        $output = 'bin/modelserver'
        if ($targetOS -eq 'windows') { $output += '.exe' }

        Write-Host 'Building dashboard...'
        Push-Location (Join-Path $PSScriptRoot 'web')
        try {
            if (-not $SkipInstall) {
                & npm.cmd ci
                if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency installation failed.' }
            }
            & npm.cmd --script-shell=cmd.exe run build
            if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
        } finally {
            Pop-Location
        }

        Write-Host 'Building server...'
        New-Item -ItemType Directory -Path 'bin' -Force | Out-Null
        & go build -o $output ./cmd/modelserver
        if ($LASTEXITCODE -ne 0) { throw 'Server build failed. On Windows, stop the running server before rebuilding.' }
        Write-Host "Built $output (dashboard embedded)."
    } finally {
        Pop-Location
    }
} catch {
    Write-Error $_ -ErrorAction Continue
    exit 1
}
