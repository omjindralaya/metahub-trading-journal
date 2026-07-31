package backend

import (
	"testing"
	"time"
)

// Trade impas (profit/komisi/swap semuanya 0) adalah trade SUNGGUHAN. Dulu
// InitDB menghapus setiap baris yang keempat kolomnya nol untuk membersihkan
// "deal IN duplikat", padahal saringan itu tidak bisa membedakan deal sampah
// dari trade yang kebetulan break-even — dan berjalan tanpa batas akun, di
// SETIAP startup. Akibatnya trade impas (dan trade manual ber-P/L 0) lenyap
// diam-diam saat aplikasi dibuka berikutnya.
//
// Deal IN sekarang sudah disaring di hulu (mt5_service.go, `d.Entry ==
// DealEntryIn` dilewati), jadi tidak ada lagi yang perlu dibersihkan di sini.
func TestInitDB_KeepsBreakEvenTrade(t *testing.T) {
	DB.Exec("DELETE FROM trades")
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")

	// Persis bentuk yang dulu terhapus: menang-kalah nol bersih.
	if err := SaveTrades([]Trade{{
		Ticket: "be1", Symbol: "XAUUSD", Type: "Buy", Volume: 1,
		OpenPrice: 2000, ClosePrice: 2000, CloseTime: time.Now(),
		Profit: 0, NetProfit: 0, Commission: 0, Swap: 0,
		MT5Login: "500", MT5Server: "BrokerA",
	}}); err != nil {
		t.Fatalf("simpan trade impas: %v", err)
	}

	InitDB() // simulasi aplikasi ditutup lalu dibuka lagi

	var count int64
	DB.Model(&Trade{}).Where("ticket = ?", "be1").Count(&count)
	if count != 1 {
		t.Fatalf("trade impas harus bertahan melewati restart, sisa %d baris", count)
	}
}

// position_id ditentukan broker, jadi dua akun di broker berbeda bisa memakai
// angka yang sama. Cache SL/TP dulu ber-primary-key position_id SAJA, sehingga
// akun kedua menimpa baris akun pertama. Pembacaannya (mt5_service.go) menyaring
// position_id + login + server, jadi setelah tertimpa lookup akun pertama tidak
// menemukan apa-apa dan trade-nya tersimpan dengan SL/TP = 0 — tanpa error di
// mana pun.
func TestCacheOpenPositions_KeepsRowPerAccount(t *testing.T) {
	DB.Exec("DELETE FROM saved_open_positions")

	CacheOpenPositions([]OpenPosition{{
		Ticket: "900", Symbol: "XAUUSD", Type: "Buy",
		SL: 1900, TP: 2100, MT5Login: "500", MT5Server: "BrokerA",
	}})
	// Akun lain, position_id kebetulan sama.
	CacheOpenPositions([]OpenPosition{{
		Ticket: "900", Symbol: "EURUSD", Type: "Sell",
		SL: 1.05, TP: 1.01, MT5Login: "999", MT5Server: "BrokerB",
	}})

	var total int64
	DB.Model(&SavedOpenPosition{}).Where("position_id = ?", "900").Count(&total)
	if total != 2 {
		t.Fatalf("position_id sama di dua akun harus jadi dua baris, dapat %d", total)
	}

	// Dan SL/TP akun pertama harus utuh — inilah yang dibaca saat posisinya ditutup.
	var a SavedOpenPosition
	if err := DB.First(&a, "position_id = ? AND mt5_login = ? AND mt5_server = ?",
		"900", "500", "BrokerA").Error; err != nil {
		t.Fatalf("baris akun pertama hilang: %v", err)
	}
	if a.SL != 1900 || a.TP != 2100 {
		t.Fatalf("SL/TP akun pertama tertimpa akun lain: SL=%v TP=%v", a.SL, a.TP)
	}
}
