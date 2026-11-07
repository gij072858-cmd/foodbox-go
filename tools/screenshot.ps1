# Regenerate the milestone screenshots with the headless UI tester.
#
# The tester renders the real views at 1280x860 and writes PNGs into
# test/screenshots, so the evidence is produced by the same code the window
# runs. Usage: pwsh -File tools/screenshot.ps1 [-Run <regex>]

param(
    [string]$Run = "TestScreenshots"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

Push-Location $root
try {
    go test ./internal/app -run $Run -count=1 -v
    if ($LASTEXITCODE -ne 0) { throw "screenshot run failed" }
}
finally {
    Pop-Location
}

Get-ChildItem (Join-Path $root "test\screenshots") -File -Filter *.png |
    Select-Object Name, Length |
    Format-Table -AutoSize
