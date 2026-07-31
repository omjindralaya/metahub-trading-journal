//go:build windows

package backend

import (
	"encoding/base64"
	"strings"
)

// dpapiSealPrefix menandai nilai yang benar-benar tersegel DPAPI. Prefiks ini
// membedakannya secara TEGAS dari nilai teks-polos lama (jalur migrasi) atau
// nilai yang ditulis test, sehingga openToken tidak perlu menebak-nebak format.
const dpapiSealPrefix = "dpapi:"

// sealToken menyegel token dengan DPAPI (CryptProtectData) lalu meng-encode
// hasilnya base64 dengan prefiks penanda. Reuse helper dpapiProtect yang sudah
// dipakai untuk menyegel private key device (device_key_software_windows.go).
func sealToken(token string) (string, error) {
	sealed, err := dpapiProtect([]byte(token))
	if err != nil {
		return "", err
	}
	return dpapiSealPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// openToken membuka segel. Nilai tanpa prefiks = teks polos lama/test → kembalikan
// apa adanya (kompatibilitas migrasi). Blob tersegel yang gagal di-decode/unseal
// (mis. profil Windows berubah) → "" agar diperlakukan sebagai belum login,
// bukan token sampah yang akan ditolak server berulang kali.
func openToken(stored string) string {
	if stored == "" {
		return ""
	}
	if !strings.HasPrefix(stored, dpapiSealPrefix) {
		return stored
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, dpapiSealPrefix))
	if err != nil {
		return ""
	}
	plain, err := dpapiUnprotect(blob)
	if err != nil {
		return ""
	}
	return string(plain)
}
