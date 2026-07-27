# livesync-sync Windows 展開スクリプト
# 使い方:
#   1. livesync.b64 と同じフォルダにこのスクリプトを置く
#   2. PowerShell を開いて以下を実行:
#      powershell -ExecutionPolicy Bypass -File decode.ps1
#
# または certutil を使う場合:
#   certutil -decode livesync.b64 livesync.exe

$src = Join-Path $PSScriptRoot "livesync.b64"
$dst = Join-Path $PSScriptRoot "livesync.exe"

if (-not (Test-Path $src)) {
    Write-Host "Error: livesync.b64 not found in $PSScriptRoot" -ForegroundColor Red
    Write-Host "Please save the base64 text as livesync.b64 first." -ForegroundColor Yellow
    exit 1
}

Write-Host "Decoding livesync.b64 → livesync.exe ..." -ForegroundColor Cyan
try {
    $data = [System.Convert]::FromBase64String((Get-Content $src -Raw))
    [System.IO.File]::WriteAllBytes($dst, $data)
    Write-Host "Done! ($($data.Length) bytes written)" -ForegroundColor Green
    Write-Host "Run: .\$([System.IO.Path]::GetFileName($dst))" -ForegroundColor Yellow
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
    exit 1
}
