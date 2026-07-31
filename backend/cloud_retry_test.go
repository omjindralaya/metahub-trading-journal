package backend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fastRetries memperkecil jeda supaya test tidak benar-benar menunggu detik.
func fastRetries(t *testing.T, chunkSize int) {
	t.Helper()
	oldSize, oldBase, oldPace := tradeSyncChunkSize, chunkRetryBaseDelay, chunkPacingDelay
	tradeSyncChunkSize, chunkRetryBaseDelay, chunkPacingDelay = chunkSize, time.Millisecond, time.Millisecond
	t.Cleanup(func() {
		tradeSyncChunkSize, chunkRetryBaseDelay, chunkPacingDelay = oldSize, oldBase, oldPace
	})
}

func captureProgress(t *testing.T) *[]SyncProgress {
	t.Helper()
	var seen []SyncProgress
	old := syncProgressFn
	SetSyncProgressHandler(func(p SyncProgress) { seen = append(seen, p) })
	t.Cleanup(func() { syncProgressFn = old })
	return &seen
}

func threeTrades() []Trade {
	closed := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	return []Trade{
		{Ticket: "rt-1", Symbol: "XAUUSD", Type: "Buy", Volume: 1, CloseTime: closed},
		{Ticket: "rt-2", Symbol: "XAUUSD", Type: "Buy", Volume: 1, CloseTime: closed},
		{Ticket: "rt-3", Symbol: "XAUUSD", Type: "Sell", Volume: 1, CloseTime: closed},
	}
}

// Rate limiter server (100/menit) bisa menolak potongan di tengah import riwayat
// penuh. 429 harus dicoba ulang, bukan menggugurkan seluruh sync.
func TestPushClosedTrades_RetriesOn429(t *testing.T) {
	fastRetries(t, 2)
	SaveSetting("account_balance", "1000")

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dua permintaan pertama ditolak, sisanya diterima.
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"success":false,"error":{"code":"RATE_LIMITED"}}`))
			return
		}
		w.Write([]byte(`{"success":true,"data":{"synced":2,"skipped":0}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	progress := captureProgress(t)

	msg, err := pushClosedTradesToCloud("tok", threeTrades())
	if err != nil {
		t.Fatalf("429 sementara seharusnya dicoba ulang, bukan gagal: %v", err)
	}
	if msg == "" {
		t.Fatal("pesan hasil sync kosong")
	}
	// 2 potongan (2+1 trade) + 2 penolakan yang dicoba ulang.
	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Fatalf("jumlah request = %d, mau 4 (2 potongan + 2 retry)", got)
	}
	if len(*progress) == 0 {
		t.Fatal("tidak ada laporan kemajuan yang dikirim ke UI")
	}
	last := (*progress)[len(*progress)-1]
	if last.Sent != last.Total || last.Total != 3 {
		t.Fatalf("laporan terakhir = %+v, mau 3/3", last)
	}
}

// Penolakan yang terus-menerus harus MENYERAH dengan pesan jelas, bukan
// mengulang selamanya dan menggantung aplikasi.
func TestPushClosedTrades_GivesUpAfterPersistent429(t *testing.T) {
	fastRetries(t, 2)
	SaveSetting("account_balance", "1000")

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"success":false,"error":{"code":"RATE_LIMITED"}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	_, err := pushClosedTradesToCloud("tok", threeTrades())
	if err == nil {
		t.Fatal("429 terus-menerus harus berakhir sebagai error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "terlalu banyak") {
		t.Fatalf("pesan error tidak menjelaskan sebabnya: %v", err)
	}
	if got := atomic.LoadInt32(&calls); int(got) != chunkRetryAttempts {
		t.Fatalf("jumlah percobaan = %d, mau berhenti di %d", got, chunkRetryAttempts)
	}
}
