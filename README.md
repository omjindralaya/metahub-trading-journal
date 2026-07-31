# MetaHub Desktop — Jurnal Trading Komunitas

Aplikasi desktop (Windows) untuk mencatat dan menganalisis trade dari akun **MetaTrader 5**, dengan sinkronisasi opsional ke **MetaHub Cloud** untuk fitur komunitas (leaderboard, feed, langganan trader). Dibangun dengan [Wails](https://wails.io) (backend Go + frontend React/TypeScript).

## Fitur utama

- **Auto-sync dari MT5** - menarik riwayat transaksi dan posisi floating langsung dari terminal MT5 yang sedang login, tanpa input manual.
- **Input manual** - tambah trade secara manual untuk akun/instrumen di luar MT5.
- **Dashboard & jurnal** - ringkasan performa per periode (hari ini/30/60/90/semua), per akun MT5 (tidak dicampur antar akun/mata uang).
- **Login Google & sinkron cloud** — login sekali via browser, lalu backend mendorong trade tertutup dan posisi live ke MetaHub Cloud secara berkala.
- **Guard akun target** - memberi tahu jika akun MT5 yang sedang aktif di terminal bukan akun yang dipilih user untuk disinkron, alih-alih diam-diam salah kirim data.
- **Entitlement/langganan** - status akses (gratis/berbayar) dicek ke server saat startup dan berkala; sinkron cloud hanya jalan sesuai hak akses akun.
- **Auto-backfill** - riwayat lama yang sebelumnya tertahan batas sinkron otomatis dikirim ulang setelah upgrade paket.

## Struktur proyek

```
app.go              # Binding Go <-> frontend (Wails)
main.go             # Entry point aplikasi
backend/            # Logika inti: MT5, database SQLite, sync cloud, auth, entitlement
frontend/           # UI React + TypeScript (Vite, Tailwind)
build/              # Aset & konfigurasi packaging (ikon, installer NSIS, manifest Windows)
build-release.ps1   # Script build rilis produksi (lihat di bawah)
```

## Menjalankan secara lokal (development)

Prasyarat: Go 1.2x+, Node.js, [Wails CLI v2](https://wails.io/docs/gettingstarted/installation).

```
wails dev
```

Menjalankan dev server Vite dengan hot reload. Bisa juga dibuka lewat browser di `http://localhost:34115` untuk akses devtools dan memanggil method Go langsung.

### Kalau `go build` gagal pada clone baru

`main.go` melakukan `//go:embed all:frontend/dist`, dan bundle frontend tidak di-commit. Pada clone segar, perintah Go langsung akan berhenti dengan:

```
pattern all:frontend/dist: no matching files found
```

Bangun frontend-nya sekali dulu — `wails dev` dan `wails build` melakukannya otomatis, atau manual:

```
cd frontend && npm ci && npm run build
```

Setelah itu `go build ./...`, `go vet ./...`, dan `go test ./...` jalan seperti biasa.

### Di mana data lokal disimpan

`%APPDATA%\MetaHub\journal.db` (Windows). Lokasinya sengaja tidak relatif terhadap working directory — kalau relatif, shortcut dengan working directory berbeda akan membuka database kosong yang lain. Database dari versi lama yang masih tersimpan di sebelah executable dipindahkan otomatis sekali saat startup.

File ini memuat jurnal trading, token cloud (tersegel DPAPI), dan private key device. Jangan dibagikan.

## Build rilis

**Jangan pakai `wails build` polos untuk rilis** - API URL default-nya menunjuk ke `localhost:8080` (lihat `backend/sync_service.go`), sehingga binary hasil build akan menembak mesin development, bukan server produksi.

Gunakan script rilis, yang menyuntikkan URL produksi lewat `-ldflags -X`:

```powershell
./build-release.ps1                # build biasa
./build-release.ps1 -Installer     # + bikin installer NSIS (build/bin/*-installer.exe)
./build-release.ps1 -Obfuscate     # + obfuscate simbol via garble (butuh garble terpasang)
```

Catatan: obfuscation di sini hanya menaikkan biaya membaca binary, bukan mekanisme proteksi - hak akses (sinkron cloud) dijaga di server via signed device key, bukan di client.

Binary hasil build (`*.exe`, `build/bin/`) sengaja **tidak** disertakan di repo ini (lihat `.gitignore`) — didistribusikan lewat GitHub Releases, bukan lewat riwayat git.

## Test

```
go vet ./...
go test ./...
```

Test Go berjalan di Windows (identitas device memakai DPAPI/TPM, lihat `backend/device_key_*_windows.go`) dan memakai database sementara - tidak menyentuh `journal.db` milik Anda. CI menjalankan keduanya plus build frontend pada setiap PR (`.github/workflows/ci.yml`).

## Keamanan

Laporkan kerentanan lewat [Security Advisories](../../security/advisories/new), bukan issue publik. Baca [SECURITY.md](SECURITY.md) lebih dulu - di sana dijelaskan beberapa hal yang tampak seperti kerentanan tapi sebenarnya keputusan desain (mis. cache hak akses lokal yang memang bisa diedit, karena gerbang sesungguhnya ada di server).

## Lisensi

[MIT](LICENSE).
