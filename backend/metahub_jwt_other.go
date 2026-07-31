//go:build !windows

package backend

// Non-Windows tidak punya DPAPI. Paket backend saat ini hanya build di Windows
// (identitas device di device_key_software_windows.go / _tpm_windows.go tak punya
// padanan non-Windows), tapi stub ini menjaga jalur token tetap terkompilasi dan
// benar bila padanan itu ditambahkan kelak: token disimpan apa adanya. Bila
// platform lain benar-benar didukung nanti, ganti dengan keyring OS setempat
// (Keychain di macOS, Secret Service di Linux).
func sealToken(token string) (string, error) { return token, nil }

func openToken(stored string) string { return stored }
