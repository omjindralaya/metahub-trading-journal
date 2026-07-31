package backend

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Server menolak sebagian trade karena di luar jendela paket. Ticket yang
// ditolak TIDAK boleh ditandai tersinkron: kalau ditandai, ia tidak pernah
// dikirim lagi dan backfill setelah upgrade tidak menemukan apa pun.
func TestPushMenandaiTicketDitolakSebagaiBlocked(t *testing.T) {
	clearTestTrades(t)
	defer clearTestTrades(t)
	SaveSetting("mt5_account", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"success": true,
			"data": {
				"synced": 1,
				"skipped": 0,
				"out_of_window": 1,
				"out_of_window_tickets": ["ps-ow-lama"]
			}
		}`))
	}))
	defer srv.Close()

	old := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(old)

	SaveSetting("metahub_jwt", "token-test")
	defer SaveSetting("metahub_jwt", "")

	trades := []Trade{mustTrade(t, "ps-ow-lama", 10), mustTrade(t, "ps-ow-baru", 20)}
	if err := SaveTrades(trades); err != nil {
		t.Fatalf("simpan: %v", err)
	}

	if _, err := pushClosedTradesToCloud("token-test", trades); err != nil {
		t.Fatalf("push: %v", err)
	}

	var lama, baru Trade
	if err := DB.First(&lama, "ticket = ?", "ps-ow-lama").Error; err != nil {
		t.Fatalf("baca ps-ow-lama: %v", err)
	}
	if err := DB.First(&baru, "ticket = ?", "ps-ow-baru").Error; err != nil {
		t.Fatalf("baca ps-ow-baru: %v", err)
	}

	if lama.CloudSyncedAt != nil {
		t.Fatal("ticket yang DITOLAK ditandai tersinkron — datanya hilang selamanya")
	}
	if lama.CloudBlockedAt == nil {
		t.Fatal("ticket yang ditolak harus ditandai blocked")
	}
	if baru.CloudSyncedAt == nil {
		t.Fatal("ticket yang DITERIMA harus ditandai tersinkron")
	}
	if baru.CloudBlockedAt != nil {
		t.Fatal("ticket yang diterima tidak boleh ditandai blocked")
	}
}

// Server lama (belum di-deploy) tidak mengirim out_of_window_tickets sama
// sekali. Perilakunya harus persis seperti hari ini: semua ditandai tersinkron.
func TestPushKompatibelDenganServerTanpaFieldBaru(t *testing.T) {
	clearTestTrades(t)
	defer clearTestTrades(t)
	SaveSetting("mt5_account", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true, "data": {"synced": 1, "skipped": 0}}`))
	}))
	defer srv.Close()

	old := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(old)

	trades := []Trade{mustTrade(t, "ps-compat-1", 10)}
	if err := SaveTrades(trades); err != nil {
		t.Fatalf("simpan: %v", err)
	}
	if _, err := pushClosedTradesToCloud("token-test", trades); err != nil {
		t.Fatalf("push: %v", err)
	}

	var got Trade
	if err := DB.First(&got, "ticket = ?", "ps-compat-1").Error; err != nil {
		t.Fatalf("baca: %v", err)
	}
	if got.CloudSyncedAt == nil || got.CloudBlockedAt != nil {
		t.Fatalf("server lama: mau synced & tidak blocked, dapat synced=%v blocked=%v",
			got.CloudSyncedAt, got.CloudBlockedAt)
	}
}
