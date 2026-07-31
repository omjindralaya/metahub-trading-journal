//go:build windows

package backend

import "testing"

// Token cloud harus tersegel di journal.db, sama seperti private key device.
// Sebelumnya ia ditulis polos: proses lain di akun Windows yang sama bisa
// membacanya dari file DB dan membajak sesi. Test ini mengunci tiga hal:
// round-trip benar, nilai TERSIMPAN bukan token telanjang, dan token lama
// tanpa segel (migrasi) tetap terbaca.
func TestMetahubJWTIsSealedAtRest(t *testing.T) {
	const token = "header.payload.signature-xyz"
	if err := saveMetahubJWT(token); err != nil {
		t.Fatalf("saveMetahubJWT: %v", err)
	}

	// Round-trip lewat DPAPI mengembalikan token asli.
	if got := loadMetahubJWT(); got != token {
		t.Fatalf("loadMetahubJWT = %q, mau %q", got, token)
	}

	// Nilai MENTAH di DB tidak boleh sama dengan token — kalau sama, DPAPI tak
	// dipakai dan celah teks-polos masih terbuka.
	raw := GetSetting(metahubJWTSetting)
	if raw == token {
		t.Fatal("token tersimpan telanjang — sealToken tidak dipakai")
	}
	if raw == "" {
		t.Fatal("token tersegel tidak tersimpan")
	}
}

// Instalasi pra-upgrade menyimpan token polos (dan test lain menulis via
// SaveSetting mentah). loadMetahubJWT harus tetap membacanya apa adanya, jadi
// upgrade tidak memaksa semua user login ulang.
func TestMetahubJWTPlaintextFallback(t *testing.T) {
	SaveSetting(metahubJWTSetting, "legacy-plain-token")
	if got := loadMetahubJWT(); got != "legacy-plain-token" {
		t.Fatalf("token polos lama harus terbaca apa adanya, dapat %q", got)
	}
}

// Menyimpan token kosong = logout: disimpan apa adanya (bukan disegel) dan
// terbaca kosong. Ini menjaga jalur LogoutCloud dan handleSyncUnauthorized.
func TestMetahubJWTEmptyClears(t *testing.T) {
	if err := saveMetahubJWT("x.y.z"); err != nil {
		t.Fatal(err)
	}
	if err := saveMetahubJWT(""); err != nil {
		t.Fatal(err)
	}
	if got := GetSetting(metahubJWTSetting); got != "" {
		t.Fatalf("logout harus mengosongkan setting, dapat %q", got)
	}
	if got := loadMetahubJWT(); got != "" {
		t.Fatalf("loadMetahubJWT setelah logout harus \"\", dapat %q", got)
	}
}
