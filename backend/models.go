package backend

import (
	"time"
)

// Trade mewakili satu transaksi trading yang telah selesai (closed trade).
type Trade struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Ticket      string    `json:"ticket" gorm:"index"` // unik per (login,server,ticket) — lihat migrasi composite
	OpenTime    time.Time `json:"open_time"`
	CloseTime   time.Time `json:"close_time"`
	Symbol      string    `json:"symbol"`
	Type        string    `json:"type"` // "Buy" atau "Sell"
	Volume      float64   `json:"volume"`
	OpenPrice   float64   `json:"open_price"`
	ClosePrice  float64   `json:"close_price"`
	SL          float64   `json:"sl"`
	TP          float64   `json:"tp"`
	Commission  float64   `json:"commission"`
	Swap        float64   `json:"swap"`
	Profit      float64   `json:"profit"` // Profit bersih termasuk komisi & swap (tergantung cara MT5 export)
	NetProfit   float64   `json:"net_profit"`
	PositionID  string    `json:"position_id"` // Untuk mencocokkan dengan SavedOpenPosition
	AccountType string    `json:"account_type"`

	// Identitas akun MT5 pemilik trade ini (login + server). Wajib ada agar
	// jurnal lokal tidak mencampur akun dan agar nomor tiket yang kebetulan sama
	// antar-akun tidak saling menimpa. Kosong hanya untuk baris legacy sebelum
	// migrasi (di-backfill best-effort ke akun terakhir yang aktif).
	MT5Login  string `json:"mt5_login" gorm:"index"`
	MT5Server string `json:"mt5_server" gorm:"index"`

	// CloudSyncedAt menandai trade ini sudah berhasil didorong ke MetaHub Cloud.
	// NULL = belum pernah, atau isinya berubah di MT5 sejak dorongan terakhir
	// (upsert mengosongkannya kembali). Inilah yang membuat sync ke server
	// mengirim HANYA yang belum terkirim, bukan mengunggah ulang seluruh riwayat
	// setiap kali tombol sync ditekan.
	CloudSyncedAt *time.Time `json:"cloud_synced_at" gorm:"index"`

	// CloudBlockedAt menandai trade yang DITOLAK server karena close_time-nya di
	// bawah batas jendela paket user. Sengaja dipisah dari CloudSyncedAt: yang
	// itu berarti "sudah aman di server", yang ini berarti "belum, dan mengulang
	// sekarang tidak ada gunanya".
	//
	// Menyatukan keduanya adalah kehilangan data: trade yang ditandai tersinkron
	// tidak akan pernah dikirim lagi, sehingga backfill setelah user upgrade
	// tidak menemukan apa pun untuk dikirim — user membayar dan riwayat lamanya
	// tetap kosong, tanpa error di mana pun.
	CloudBlockedAt *time.Time `json:"cloud_blocked_at" gorm:"index"`
}

// SavedOpenPosition merekam snapshot SL dan TP dari posisi yang sedang berjalan.
// Data ini digunakan untuk melengkapi Trade ketika posisinya ditutup.
//
// Primary key-nya KOMPOSIT (position_id, mt5_login, mt5_server) dengan alasan
// yang sama dengan unique index komposit pada trades: position_id ditentukan
// broker dan hanya unik di dalam satu trade server. Dengan position_id sebagai
// satu-satunya key, akun kedua menimpa baris akun pertama; lalu pencarian SL/TP
// akun pertama (yang menyaring login+server) tidak menemukan apa-apa dan trade
// tersimpan dengan SL/TP nol — tanpa error di mana pun.
type SavedOpenPosition struct {
	PositionID string  `json:"position_id" gorm:"primaryKey"`
	MT5Login   string  `json:"mt5_login" gorm:"primaryKey"`
	MT5Server  string  `json:"mt5_server" gorm:"primaryKey"`
	Ticket     string  `json:"ticket"`
	Symbol     string  `json:"symbol"`
	Type       string  `json:"type"`
	SL         float64 `json:"sl"`
	TP         float64 `json:"tp"`
}

// Setting mewakili pengaturan umum aplikasi seperti mata uang
type Setting struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value"`
}


// OpenPosition mewakili posisi yang masih terbuka (floating)
type OpenPosition struct {
	Ticket       string  `json:"ticket"`
	Symbol       string  `json:"symbol"`
	Type         string  `json:"type"` // Buy / Sell
	Volume       float64 `json:"volume"`
	OpenPrice    float64 `json:"open_price"`
	CurrentPrice float64 `json:"current_price"`
	SL           float64 `json:"sl"`
	TP           float64 `json:"tp"`
	FloatingPnL  float64 `json:"floating_pnl"`
	AccountType  string  `json:"account_type"`
	MT5Login     string  `json:"mt5_login"`
	MT5Server    string  `json:"mt5_server"`
}
