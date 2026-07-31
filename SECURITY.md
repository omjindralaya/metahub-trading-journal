# Kebijakan Keamanan

## Melaporkan kerentanan

**Jangan** membuka issue publik untuk kerentanan keamanan. Laporkan lewat
[GitHub Security Advisories](../../security/advisories/new) — laporan itu hanya
terlihat oleh maintainer sampai perbaikannya siap.

Sertakan sebisanya: versi aplikasi (lihat `AppVersion` di `backend/device_client.go`),
langkah reproduksi, dan dampak yang Anda perkirakan. Kami berusaha membalas dalam
7 hari.

## Cakupan

Repositori ini hanya berisi **aplikasi desktop**. Server MetaHub Cloud ada di repositori terpisah.

## Model ancaman yang perlu diketahui sebelum melapor

Beberapa hal yang mungkin terlihat seperti kerentanan sebenarnya adalah keputusan
desain yang disengaja. Silakan tetap melapor kalau Anda tidak setuju, tapi
sebutkan konteks ini agar kita tidak berputar:

- **Cache hak akses di `journal.db` bisa diedit pemilik mesin.** Disengaja. Cache
  itu hanya menentukan tampilan dan apakah client repot-repot mengirim request.
  Gerbang yang sesungguhnya ada di server, yang memeriksa ulang setiap penulisan.
  Membalik `entitlement_can_sync` secara lokal hanya menghasilkan permintaan yang
  dijawab 403.

- **Binary bisa ditambal/dibongkar.** Disengaja, dan `build-release.ps1` menyatakan
  ini secara eksplisit. Obfuscation (opsional, via garble) menaikkan ongkos membaca
  binary, bukan mengamankan monetisasi.

- **Private key device dilindungi DPAPI, bukan disembunyikan dari pemilik akun.**
  DPAPI menghentikan proses/user LAIN di mesin yang sama membaca kunci. Ia tidak,
  dan tidak bisa, menghentikan pemilik akun Windows itu sendiri. Server mencatat
  bedanya sebagai `key_protection: "software"` versus `"tpm"`.

- **Rilis tanpa tanda tangan Authenticode.** Sudah diketahui. Bangun sendiri dari
  source bila Anda butuh rantai kepercayaan yang bisa diverifikasi.

Yang **selalu** ingin kami dengar: pengiriman data ke pihak ketiga yang tidak
disengaja, token atau kredensial yang bocor ke log/disk dalam bentuk polos,
kemampuan membaca atau menulis data akun MetaHub milik **pengguna lain**, dan
eksekusi kode lewat data yang datang dari MT5 atau dari server.
