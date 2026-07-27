# livesync-sync Windows Setup Script
# Run: powershell -ExecutionPolicy Bypass -File setup.ps1
#
# What this does:
#   1. Installs self-signed certificate (so Defender trusts the binary more)
#   2. Decodes livesync-signed.b64 → livesync.exe
#   3. Optionally adds to startup

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  livesync-sync Windows Setup" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Step 1: Install certificate
$certB64 = Join-Path $ScriptDir "livesync.crt.b64"
$certPath = Join-Path $ScriptDir "livesync.crt"

if (Test-Path $certB64) {
    Write-Host "[1/3] Installing certificate..." -ForegroundColor Yellow
    try {
        $bytes = [System.Convert]::FromBase64String((Get-Content $certB64 -Raw))
        [System.IO.File]::WriteAllBytes($certPath, $bytes)
        
        # Install to Trusted Publisher (reduces Defender warnings)
        $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($certPath)
        $store = New-Object System.Security.Cryptography.X509Certificates.X509Store("TrustedPublisher", "LocalMachine")
        try {
            $store.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite)
            $store.Add($cert)
            Write-Host "  ✓ Certificate installed to Trusted Publisher" -ForegroundColor Green
        } catch {
            Write-Host "  ⚠ Need admin rights for certificate install: $($_.Exception.Message)" -ForegroundColor Yellow
            Write-Host "  → Run as Administrator or skip (binary still works)" -ForegroundColor Yellow
        } finally {
            $store.Close()
        }
    } catch {
        Write-Host "  ⚠ Certificate install skipped: $($_.Exception.Message)" -ForegroundColor Yellow
    }
}

# Step 2: Decode binary
$b64File = Join-Path $ScriptDir "livesync-signed.b64"
$exePath = Join-Path $ScriptDir "livesync.exe"

if (Test-Path $b64File) {
    Write-Host "[2/3] Decoding livesync.exe ..." -ForegroundColor Yellow
    try {
        $data = [System.Convert]::FromBase64String((Get-Content $b64File -Raw))
        [System.IO.File]::WriteAllBytes($exePath, $data)
        Write-Host "  ✓ livesync.exe ($($data.Length / 1MB -as [int]) MB)" -ForegroundColor Green
    } catch {
        Write-Host "  ✗ Decode failed: $_" -ForegroundColor Red
        exit 1
    }
} else {
    # Fallback: try direct binary
    $exeDirect = Join-Path $ScriptDir "livesync-signed.exe"
    if (Test-Path $exeDirect) {
        Copy-Item $exeDirect $exePath
    } else {
        Write-Host "  ✗ livesync-signed.b64 not found" -ForegroundColor Red
        exit 1
    }
}

# Step 3: Offer autostart
Write-Host "[3/3] Run on startup?" -ForegroundColor Yellow
$run = Read-Host "Add to startup? (y/n, default: n)"
if ($run -eq "y" -or $run -eq "Y") {
    $startupPath = [Environment]::GetFolderPath("Startup")
    $lnkPath = Join-Path $startupPath "livesync-sync.lnk"
    $shell = New-Object -ComObject WScript.Shell
    $lnk = $shell.CreateShortcut($lnkPath)
    $lnk.TargetPath = $exePath
    $lnk.WorkingDirectory = $ScriptDir
    $lnk.Description = "livesync-sync file sync daemon"
    $lnk.Save()
    Write-Host "  ✓ Added to startup" -ForegroundColor Green
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Setup complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Run livesync.exe to start."
Write-Host "Settings UI: http://localhost:2324/settings"
Write-Host ""
Write-Host "If Defender still blocks, submit to:"
Write-Host "  https://www.microsoft.com/en-us/wdsi/filesubmission"
