package backend

import (
	"log"
	"strconv"
	"time"
)

// keyAutoBackfill adalah setting toggle "isi ulang riwayat lama setelah
// upgrade". Kosong = belum pernah disetel = ON, karena ini yang diharapkan user
// setelah membayar: riwayat lama muncul tanpa disuruh.
const keyAutoBackfill = "auto_backfill_on_upgrade"

// AutoBackfillEnabled melaporkan setelan toggle. Default ON.
func AutoBackfillEnabled() bool {
	return GetSetting(keyAutoBackfill) != "0"
}

// SetAutoBackfillEnabled menyimpan setelan toggle. Dipanggil dari UI Pengaturan.
func SetAutoBackfillEnabled(on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	return SaveSetting(keyAutoBackfill, v)
}

// widerThan melaporkan apakah jendela a lebih lebar dari jendela b.
//
// 0 berarti TANPA BATAS, sehingga ia lebih lebar dari angka berapa pun.
// Perbandingan numerik polos (a > b) akan membaca 0 sebagai paling SEMPIT dan
// membalik seluruh logika upgrade — Elite (0) akan terlihat seperti downgrade
// dari Pro (365). Aturannya hidup di sini, satu tempat.
func widerThan(a, b int) bool {
	if a <= 0 {
		return b > 0 // tanpa batas lebih lebar dari apa pun yang terbatas
	}
	if b <= 0 {
		return false // tidak ada yang lebih lebar dari tanpa batas
	}
	return a > b
}

// shouldBackfill memutuskan apakah trade yang tertahan jendela harus
// dikembalikan ke antrean kirim. Fungsi murni supaya keputusannya bisa dites
// tanpa DB maupun jaringan.
//
// Menyempit sengaja TIDAK melakukan apa-apa: langganan yang habis tidak
// menaikkan lantai, jadi tidak ada yang perlu ditahan ulang.
func shouldBackfill(oldWindow, newWindow int, toggleOn bool) bool {
	return toggleOn && widerThan(newWindow, oldWindow)
}

// applyWindowChange menjalankan keputusan shouldBackfill. Dipanggil
// RefreshEntitlement setelah jawaban server diterima, SEBELUM cache ditimpa.
func applyWindowChange(oldWindow, newWindow int) {
	if !shouldBackfill(oldWindow, newWindow, AutoBackfillEnabled()) {
		return
	}
	if err := ClearCloudBlocked(); err != nil {
		log.Printf("applyWindowChange: gagal membuka trade yang tertahan: %v", err)
		return
	}
	// Lantai lama WAJIB ikut dibuang. Membiarkannya berarti desktop memblokir
	// ulang trade yang barusan dibuka, memakai batas yang sudah tidak berlaku,
	// sebelum server sempat melaporkan lantai barunya.
	SaveSyncFloor(nil)
	log.Printf("applyWindowChange: jendela sync melebar (%d → %d), riwayat tertahan dikembalikan ke antrean", oldWindow, newWindow)
}

// syncFloorKey adalah kunci setting lantai untuk SATU akun MT5. Lantai bersifat
// per-akun: user dengan tiga akun MT5 punya tiga lantai berbeda, masing-masing
// terkunci saat akun itu pertama sync. Memakai satu kunci global akan membuat
// akun yang baru dihubungkan mewarisi lantai akun lain.
func syncFloorKey() string {
	return "sync_floor_at_" + GetSetting("mt5_account")
}

// CachedSyncFloor membaca lantai akun MT5 yang sedang aktif, sebagaimana
// TERAKHIR DILAPORKAN SERVER.
//
// Zero time berarti BELUM DIKETAHUI — bukan "tolak semuanya". Desktop sengaja
// tidak pernah menghitung lantai sendiri: lantai server dikunci pada sync
// pertama, sedangkan hitungan lokal selalu relatif terhadap hari ini, sehingga
// akun lama akan tertahan jauh lebih ketat daripada yang server izinkan.
func CachedSyncFloor() time.Time {
	raw := GetSetting(syncFloorKey())
	if raw == "" {
		return time.Time{}
	}
	unix, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}

// SaveSyncFloor menyimpan lantai yang dilaporkan server untuk akun MT5 aktif.
// nil (tier tanpa batas) menghapus lantai yang tersimpan.
func SaveSyncFloor(floor *time.Time) {
	if floor == nil {
		SaveSetting(syncFloorKey(), "")
		return
	}
	SaveSetting(syncFloorKey(), strconv.FormatInt(floor.Unix(), 10))
}

// partitionByFloor memisahkan trade yang layak dikirim dari yang sudah pasti
// ditolak. Fungsi murni.
//
// Lantai zero melewatkan SEMUANYA: belum tahu bukan alasan menahan data.
// Trade yang close_time-nya PERSIS di lantai ikut terkirim — server memakai
// Before(), jadi batasnya inklusif, dan kedua sisi harus sepakat.
func partitionByFloor(trades []Trade, floor time.Time) ([]Trade, []string) {
	if floor.IsZero() {
		return trades, nil
	}
	sendable := make([]Trade, 0, len(trades))
	var blocked []string
	for _, t := range trades {
		if t.CloseTime.Before(floor) {
			blocked = append(blocked, t.Ticket)
			continue
		}
		sendable = append(sendable, t)
	}
	return sendable, blocked
}
