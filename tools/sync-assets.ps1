# Sync the seed JSON and images from the original Flutter project into
# internal/assets, then re-pin assets.lock.json.
#
# The Flutter repository is read-only here: this script only copies out of it.
# Usage:  pwsh -File tools/sync-assets.ps1 [-Source <FoodBox repo>]

param(
    [string]$Source = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if (-not $Source) {
    # The original Flutter project as a sibling directory, unless overridden.
    $Source = Join-Path (Split-Path -Parent $root) "FoodBox"
}
$dataSrc = Join-Path $Source "app\assets\data"
$imgSrc = Join-Path $Source "app\assets\images"
$dataDst = Join-Path $root "internal\assets\data"
$imgDst = Join-Path $root "internal\assets\images"

foreach ($p in @($dataSrc, $imgSrc)) {
    if (-not (Test-Path $p)) { throw "missing source assets: $p" }
}

Write-Host "sync data:   $dataSrc -> $dataDst"
New-Item -ItemType Directory -Force -Path $dataDst | Out-Null
Get-ChildItem $dataSrc -File | Copy-Item -Destination $dataDst -Force

Write-Host "sync images: $imgSrc -> $imgDst"
New-Item -ItemType Directory -Force -Path $imgDst | Out-Null
Get-ChildItem $imgSrc -Directory | ForEach-Object {
    $dst = Join-Path $imgDst $_.Name
    New-Item -ItemType Directory -Force -Path $dst | Out-Null
    Get-ChildItem $_.FullName -File | Copy-Item -Destination $dst -Force
}

# Re-pin the hashes the CI checks.
Push-Location $root
try {
    go test ./internal/assets -run TestAssetsLock -args -update-assets-lock
    if ($LASTEXITCODE -ne 0) { throw "assets lock update failed" }
}
finally {
    Pop-Location
}

Write-Host "done. Review 'git status internal/assets' before committing."
