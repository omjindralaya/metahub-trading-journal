package backend

import "sync"

// SyncProgress melaporkan kemajuan dorongan ke cloud sementara ia berjalan.
// Import riwayat penuh bisa memakan puluhan request; tanpa laporan ini UI hanya
// membeku tanpa penjelasan dan user menyimpulkan aplikasinya menggantung.
type SyncProgress struct {
	Sent    int    `json:"sent"`    // trade yang sudah dikonfirmasi server
	Total   int    `json:"total"`   // total trade dalam sync ini
	Message string `json:"message"` // keterangan singkat, siap tampil apa adanya
	Retry   bool   `json:"retry"`   // true saat sedang menunggu percobaan ulang
}

var (
	syncProgressMu sync.RWMutex
	syncProgressFn func(SyncProgress)
)

// SetSyncProgressHandler memasang penerima laporan kemajuan. Dipasang sekali
// oleh lapisan app (yang meneruskannya ke frontend lewat event Wails); paket
// backend sengaja tidak tahu-menahu soal Wails.
func SetSyncProgressHandler(fn func(SyncProgress)) {
	syncProgressMu.Lock()
	defer syncProgressMu.Unlock()
	syncProgressFn = fn
}

// emitSyncProgress aman dipanggil walau tidak ada handler terpasang (mis. dalam
// test atau sebelum UI siap).
func emitSyncProgress(p SyncProgress) {
	syncProgressMu.RLock()
	fn := syncProgressFn
	syncProgressMu.RUnlock()
	if fn != nil {
		fn(p)
	}
}
