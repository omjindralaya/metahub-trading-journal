package backend

import "testing"

// Trade yang ditolak server karena di luar jendela tier tidak boleh terus
// dicoba tiap 5 detik — tapi juga TIDAK boleh dianggap tersinkron, karena
// backfill saat upgrade membutuhkannya.
func TestTradeBlockedKeluarDariPending(t *testing.T) {
	clearTestTrades(t)
	defer clearTestTrades(t)

	if err := SaveTrades([]Trade{mustTrade(t, "ps-blk-1", 10), mustTrade(t, "ps-blk-2", 20)}); err != nil {
		t.Fatalf("simpan: %v", err)
	}
	if err := MarkTradesBlocked([]string{"ps-blk-1"}); err != nil {
		t.Fatalf("tandai blocked: %v", err)
	}

	pending, err := GetTradesPendingCloudSync()
	if err != nil {
		t.Fatalf("baca pending: %v", err)
	}
	if len(pending) != 1 || pending[0].Ticket != "ps-blk-2" {
		t.Fatalf("pending = %+v, mau hanya ps-blk-2", pending)
	}
}

func TestClearCloudBlockedMengembalikanKePending(t *testing.T) {
	clearTestTrades(t)
	defer clearTestTrades(t)

	if err := SaveTrades([]Trade{mustTrade(t, "ps-blk-3", 10)}); err != nil {
		t.Fatalf("simpan: %v", err)
	}
	if err := MarkTradesBlocked([]string{"ps-blk-3"}); err != nil {
		t.Fatalf("tandai blocked: %v", err)
	}
	if err := ClearCloudBlocked(); err != nil {
		t.Fatalf("bersihkan blocked: %v", err)
	}

	pending, _ := GetTradesPendingCloudSync()
	if len(pending) != 1 || pending[0].Ticket != "ps-blk-3" {
		t.Fatalf("pending = %+v, mau ps-blk-3 kembali setelah blocked dibersihkan", pending)
	}
}

// Broker memfinalisasi swap/profit belakangan. Trade yang isinya BERUBAH
// berhak dicoba lagi meski sebelumnya ditolak — jendelanya mungkin sudah
// berbeda, dan yang lebih penting: data yang berubah itu data baru.
func TestIsiBerubahMelepasBlocked(t *testing.T) {
	clearTestTrades(t)
	defer clearTestTrades(t)

	if err := SaveTrades([]Trade{mustTrade(t, "ps-blk-4", 10)}); err != nil {
		t.Fatalf("simpan: %v", err)
	}
	if err := MarkTradesBlocked([]string{"ps-blk-4"}); err != nil {
		t.Fatalf("tandai blocked: %v", err)
	}

	// Profit berubah → upsert harus mengosongkan cloud_blocked_at.
	if err := SaveTrades([]Trade{mustTrade(t, "ps-blk-4", 99)}); err != nil {
		t.Fatalf("simpan ulang: %v", err)
	}

	pending, _ := GetTradesPendingCloudSync()
	if len(pending) != 1 || pending[0].Ticket != "ps-blk-4" {
		t.Fatalf("pending = %+v, mau ps-blk-4 (isinya berubah)", pending)
	}
}
