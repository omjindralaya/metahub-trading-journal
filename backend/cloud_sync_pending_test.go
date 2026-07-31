package backend

import (
	"testing"
	"time"
)

func clearTestTrades(t *testing.T) {
	t.Helper()
	if err := DB.Where("ticket LIKE ?", "ps-%").Delete(&Trade{}).Error; err != nil {
		t.Fatalf("bersihkan trade uji: %v", err)
	}
}

func mustTrade(t *testing.T, ticket string, netProfit float64) Trade {
	t.Helper()
	// Waktu tetap: deteksi "baris berubah" membandingkan isi, jadi trade yang
	// sama harus benar-benar identik antar pemanggilan.
	closed := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	return Trade{
		Ticket:    ticket,
		Symbol:    "XAUUSD",
		Type:      "Buy",
		Volume:    1,
		OpenTime:  closed.Add(-time.Hour),
		CloseTime: closed,
		NetProfit: netProfit,
	}
}

// Upsert massal harus tetap benar, bukan sekadar cepat: baris baru masuk, baris
// yang metriknya berubah diperbarui, dan jumlah baris tidak menggelembung.
func TestSaveTrades_BulkUpsertIsCorrect(t *testing.T) {
	clearTestTrades(t)
	defer clearTestTrades(t)

	if err := SaveTrades([]Trade{mustTrade(t, "ps-1", 10), mustTrade(t, "ps-2", 20)}); err != nil {
		t.Fatalf("simpan awal: %v", err)
	}

	// Kirim ulang dengan profit yang sudah difinalisasi broker.
	if err := SaveTrades([]Trade{mustTrade(t, "ps-1", 99), mustTrade(t, "ps-3", 30)}); err != nil {
		t.Fatalf("simpan ulang: %v", err)
	}

	var count int64
	DB.Model(&Trade{}).Where("ticket LIKE ?", "ps-%").Count(&count)
	if count != 3 {
		t.Fatalf("jumlah trade = %d, mau 3 (upsert menduplikasi baris)", count)
	}

	var got Trade
	DB.Where("ticket = ?", "ps-1").First(&got)
	if got.NetProfit != 99 {
		t.Fatalf("net_profit = %v, mau 99 (upsert tidak memperbarui metrik)", got.NetProfit)
	}
}

// INTI ITEM #1: yang sudah pernah dikirim ke server TIDAK boleh ikut terkirim
// lagi. Tanpa ini setiap klik "Sync to Cloud" mengunggah ULANG seluruh riwayat.
func TestPendingCloudSync_ExcludesAlreadyPushed(t *testing.T) {
	clearTestTrades(t)
	defer clearTestTrades(t)

	if err := SaveTrades([]Trade{mustTrade(t, "ps-1", 10), mustTrade(t, "ps-2", 20)}); err != nil {
		t.Fatalf("simpan: %v", err)
	}

	pending, err := GetTradesPendingCloudSync()
	if err != nil {
		t.Fatalf("baca pending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending awal = %d, mau 2", len(pending))
	}

	if err := MarkTradesSynced([]string{"ps-1"}); err != nil {
		t.Fatalf("tandai tersinkron: %v", err)
	}

	pending, _ = GetTradesPendingCloudSync()
	if len(pending) != 1 || pending[0].Ticket != "ps-2" {
		t.Fatalf("setelah ditandai, pending = %+v, mau hanya ps-2", pending)
	}
}

// Trade yang metriknya BERUBAH di MT5 (profit/swap difinalisasi belakangan)
// harus dikirim ulang; yang tidak berubah tidak boleh membangkitkan lalu lintas.
func TestSaveTrades_ChangedRowBecomesPendingAgain(t *testing.T) {
	clearTestTrades(t)
	defer clearTestTrades(t)

	if err := SaveTrades([]Trade{mustTrade(t, "ps-1", 10), mustTrade(t, "ps-2", 20)}); err != nil {
		t.Fatalf("simpan: %v", err)
	}
	if err := MarkTradesSynced([]string{"ps-1", "ps-2"}); err != nil {
		t.Fatalf("tandai: %v", err)
	}

	// ps-1 berubah, ps-2 identik dengan yang sudah tersimpan.
	if err := SaveTrades([]Trade{mustTrade(t, "ps-1", 77), mustTrade(t, "ps-2", 20)}); err != nil {
		t.Fatalf("simpan ulang: %v", err)
	}

	pending, err := GetTradesPendingCloudSync()
	if err != nil {
		t.Fatalf("baca pending: %v", err)
	}
	if len(pending) != 1 || pending[0].Ticket != "ps-1" {
		t.Fatalf("pending = %+v, mau hanya ps-1 (yang berubah)", pending)
	}
}
