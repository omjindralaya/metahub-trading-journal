package backend

// metahubJWTSetting = kunci SQLite lokal tempat bearer token cloud disimpan.
const metahubJWTSetting = "metahub_jwt"

// saveMetahubJWT menyimpan bearer token cloud, DISEGEL sesuai platform (DPAPI di
// Windows, terikat akun OS user). Ini menutup celah: sebelumnya token ditulis
// sebagai teks polos di journal.db, padahal key device di sebelahnya sudah
// disegel DPAPI. Proses lain di akun Windows yang sama bisa membaca token polos
// itu dari file DB lalu memakainya untuk mendaftarkan device-nya sendiri
// (/devices/register hanya butuh bearer) atau menyedot data akun korban.
//
// Token kosong disimpan apa adanya = logout/hapus, bukan disegel.
func saveMetahubJWT(token string) error {
	if token == "" {
		return SaveSetting(metahubJWTSetting, "")
	}
	sealed, err := sealToken(token)
	if err != nil {
		return err
	}
	return SaveSetting(metahubJWTSetting, sealed)
}

// loadMetahubJWT membaca lalu membuka segel token. Nilai lama TANPA segel
// (instalasi pra-upgrade, atau nilai yang ditulis test) dikembalikan apa adanya,
// lalu tersegel ulang saat penyimpanan berikutnya — jadi upgrade tidak memaksa
// login ulang. Blob tersegel yang rusak diperlakukan sebagai "tidak ada token".
func loadMetahubJWT() string {
	return openToken(GetSetting(metahubJWTSetting))
}
