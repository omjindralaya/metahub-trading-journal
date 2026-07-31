<#
    Build rilis MetaHub Desktop.

    Build `wails build` polos TIDAK bisa dipakai untuk rilis: CloudAPIURL default-nya
    localhost:8080 (lihat backend/sync_service.go), jadi binary-nya menembak mesin
    pengembang, bukan server produksi. Alamat server disuntikkan di sini lewat -X.

    Soal "anti-crack": tidak ada di sini yang membuat binary tahan dibongkar, dan itu
    memang bukan tujuannya. Satu-satunya hal berharga di produk ini (sync ke cloud)
    dijaga di server lewat RequireDesktopEntitlement + tanda tangan device, bukan di
    .exe. Menambal binary agar mengira dirinya berbayar hanya menghasilkan request yang
    dijawab 403. Flag di bawah cuma membuang simbol debug dan path mesin build; itu
    menaikkan ongkos membaca binary, bukan menghentikannya.

    Catatan: file ini WAJIB tetap ASCII murni. PowerShell 5.1 membaca .ps1 sebagai ANSI,
    sehingga karakter non-ASCII (em-dash, kutip tipis) merusak parser.
#>
[CmdletBinding()]
param(
    # Alamat API produksi. Dipisah sebagai parameter supaya build staging tidak perlu
    # mengedit script (dan tidak ada yang tergoda meng-commit URL staging ke rilis).
    [string]$ApiUrl = "https://api.metahub.id/api/v1",

    # Halaman upgrade yang dibuka dari banner entitlement (backend/entitlement.go).
    [string]$UpgradeUrl = "https://metahub.id/upgrade",

    # Tidak ada RegisterUrl lagi: pendaftaran bukan langkah terpisah. Login Google
    # pertama sekaligus membuat akun, jadi layar login tak punya tautan "daftar".

    # Halaman yang menyelesaikan login Google untuk desktop (backend/google_login.go).
    [string]$DesktopAuthUrl = "https://metahub.id/desktop-auth",

    # Bikin installer NSIS sekalian.
    [switch]$Installer,

    # Obfuscate binary via garble (wails -obfuscated): acak nama simbol + enkripsi
    # string literal supaya .exe lebih sulit dibaca. Ini MENAIKKAN ongkos membongkar,
    # bukan mengamankan monetisasi (itu dijaga server, lihat catatan di header). Butuh
    # garble terpasang: go install mvdan.cc/garble@latest
    [switch]$Obfuscate,

    # Sertifikat Authenticode (.pfx). Tanpa ini binary keluar TIDAK ditandatangani:
    # SmartScreen akan menakuti pengguna Anda, dan siapa pun bisa menyisipkan malware
    # ke dalam build Anda lalu menyebarkannya sebagai "MetaHub" tanpa bisa dibedakan.
    [string]$CertPath,
    [string]$CertPassword
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

if ($ApiUrl -notmatch '^https://') {
    throw "ApiUrl harus https. JWT dan trade dikirim lewat sini. Diberikan: $ApiUrl"
}

# -X menyuntik alamat server ke variabel paket. Nama simbolnya harus persis
# <module>/<paket>.<Var>; modulnya `trading-journal` (go.mod).
# -s -w membuang tabel simbol dan info DWARF.
$ldflags = @(
    "-s", "-w",
    "-X", "trading-journal/backend.CloudAPIURL=$ApiUrl",
    "-X", "trading-journal/backend.upgradeURL=$UpgradeUrl",
    "-X", "trading-journal/backend.desktopAuthURL=$DesktopAuthUrl"
) -join " "

$wailsArgs = @(
    "build",
    "-clean",
    "-platform", "windows/amd64",
    # -trimpath membuang path absolut mesin build dari binary (E:\jurnal trade\...).
    "-trimpath",
    "-ldflags", $ldflags
)
if ($Installer) { $wailsArgs += "-nsis" }

if ($Obfuscate) {
    # garble WAJIB ada di PATH. Tanpa cek ini wails gagal di tengah dengan pesan yang
    # tidak jelas; lebih baik berhenti lebih awal dengan instruksi pasti.
    if (-not (Get-Command garble -ErrorAction SilentlyContinue)) {
        throw "garble tidak ditemukan di PATH. Install: go install mvdan.cc/garble@latest (lalu pastikan %USERPROFILE%\go\bin ada di PATH)."
    }
    # -literals mengenkripsi string literal (URL/pesan tidak lagi terbaca polos di hex).
    # -tiny membuang nama file dan nomor baris. Keduanya membuat binary lebih sulit dibaca.
    $wailsArgs += "-obfuscated"
    $wailsArgs += "-garbleargs"
    $wailsArgs += "-literals -tiny"
    Write-Host "Obfuscation: garble aktif (-literals -tiny)" -ForegroundColor Cyan
}

Write-Host "Building MetaHub -> $ApiUrl" -ForegroundColor Cyan
# wails menulis log build ke stderr. Di PowerShell 5.1, dengan $ErrorActionPreference=Stop,
# baris stderr native itu dibungkus jadi ErrorRecord dan MENGHENTIKAN script sebelum cek
# exit code di bawah jalan (build tampak "gagal" padahal sukses). Turunkan preferensi
# hanya untuk pemanggilan native ini dan stringify stderr (2>&1 | %{ "$_" }) supaya log
# tetap tampil tapi tidak diperlakukan sebagai error. Kebenaran build tetap dinilai dari
# $LASTEXITCODE (di-set exe, bukan pipeline).
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"
& wails @wailsArgs 2>&1 | ForEach-Object { "$_" }
$wailsExit = $LASTEXITCODE
$ErrorActionPreference = $prevEAP
if ($wailsExit -ne 0) { throw "wails build gagal (exit $wailsExit)" }

$exe = Join-Path $PSScriptRoot "build\bin\metahub.exe"
if (-not (Test-Path $exe)) { throw "Binary tidak ditemukan di $exe" }

# Verifikasi bahwa URL produksi benar-benar tertanam. Salah nama simbol pada -X TIDAK
# membuat linker gagal: ia diam saja dan binary keluar tetap memakai localhost. Ini
# persis cara build sebelumnya lolos sampai ke folder bin. Jadi buktikan, jangan
# percaya exit code.
$bytes = [System.IO.File]::ReadAllBytes($exe)
$ascii = [System.Text.Encoding]::ASCII.GetString($bytes)
if (-not $ascii.Contains($ApiUrl)) {
    throw "Binary tidak memuat $ApiUrl. Penyuntikan -X gagal (cek nama simbol). JANGAN dirilis."
}
# Kita TIDAK menolak rilis hanya karena byte "localhost:8080" ada di binary. Supaya
# -X bekerja, backend.CloudAPIURL WAJIB diinisialisasi ke string KONSTAN (Go: -X diam
# gagal kalau initializer memanggil fungsi). Konstanta default itu localhost:8080, dan
# linker Go 1.26 mempertahankan literal itu di rodata meski nilainya sudah di-override
# -X. Jadi byte itu sisa mati, bukan alamat aktif. Gerbang sebenarnya adalah cek di atas:
# URL produksi HANYA masuk ke binary lewat -X (frontend tidak meng-hardcode-nya, sudah
# dicek), jadi kalau cek itu lolos, nilai runtime pasti = produksi. Localhost diperingatkan
# saja, tidak memblokir.
if ($ascii.Contains("localhost:8080")) {
    Write-Warning "Binary memuat literal 'localhost:8080' (default dev sisa di rodata). Nilai runtime tetap $ApiUrl karena -X terbukti tertanam. Bukan blocker."
}
Write-Host "OK: $ApiUrl tertanam (localhost hanya literal sisa, bukan alamat aktif)." -ForegroundColor Green

if ($CertPath) {
    if (-not (Test-Path $CertPath)) { throw "Sertifikat tidak ditemukan: $CertPath" }
    $signtool = Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\signtool.exe" -ErrorAction SilentlyContinue |
        Sort-Object FullName -Descending | Select-Object -First 1
    if (-not $signtool) { throw "signtool.exe tidak ada. Install Windows SDK." }

    $targets = @($exe)
    $nsis = Get-ChildItem "$PSScriptRoot\build\bin\*installer*.exe" -ErrorAction SilentlyContinue
    if ($nsis) { $targets += $nsis.FullName }

    foreach ($t in $targets) {
        # /fd sha256 + timestamp RFC3161: tanpa timestamp, tanda tangan mati saat
        # sertifikat kedaluwarsa dan build lama Anda mendadak dianggap tak sah.
        & $signtool.FullName sign /fd sha256 /td sha256 `
            /tr "http://timestamp.digicert.com" `
            /f $CertPath /p $CertPassword $t
        if ($LASTEXITCODE -ne 0) { throw "signtool gagal untuk $t" }
    }
    Write-Host "Ditandatangani: $($targets -join ', ')" -ForegroundColor Green
} else {
    Write-Warning "TIDAK ditandatangani. SmartScreen akan memperingatkan pengguna, dan build ini bisa ditrojan orang lain tanpa terdeteksi. Beli sertifikat Authenticode, lalu jalankan ulang dengan -CertPath."
}

Write-Host "Selesai: $exe" -ForegroundColor Cyan
