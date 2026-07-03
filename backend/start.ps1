# Jalankan backend Marketing (Meta OAuth + Ads/WA/IG) di :8086 secara permanen.
# Pakai: klik kanan → Run with PowerShell, atau dari terminal:  ./start.ps1
# Biarkan jendela ini TERBUKA selama dashboard dipakai.
$ErrorActionPreference = "Stop"
$here = $PSScriptRoot
Set-Location $here

if (-not $env:APP_PORT) { $env:APP_PORT = "8086" }

# Build sekali kalau exe belum ada / source berubah.
if (-not (Test-Path "$here\server.exe")) {
  Write-Host "Building server.exe..." -ForegroundColor Yellow
  go build -o server.exe ./cmd/server
}

Write-Host "Marketing backend listening on http://localhost:$($env:APP_PORT)" -ForegroundColor Green
& "$here\server.exe"
