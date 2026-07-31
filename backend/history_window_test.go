package backend

import (
	"errors"
	"testing"
	"time"
)

// Jendela sync BUKAN konstanta; ia diturunkan dari watermark. Tiga kasus yang
// menentukan apakah user kehilangan data secara diam-diam.
func TestHistorySyncWindow(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)

	t.Run("belum pernah sync menarik SELURUH riwayat", func(t *testing.T) {
		from, full := historySyncWindow("", now)
		if !full {
			t.Fatal("import pertama harus ditandai full import")
		}
		if !from.Equal(mt5HistoryEpoch) {
			t.Fatalf("import pertama harus dari epoch, bukan %s", from)
		}
	})

	t.Run("sudah pernah sync menarik sejak watermark dengan overlap", func(t *testing.T) {
		last := now.Add(-10 * 24 * time.Hour)
		from, full := historySyncWindow(last.Format(time.RFC3339), now)
		if full {
			t.Fatal("sync lanjutan tidak boleh dianggap import pertama")
		}
		want := last.Add(-syncOverlap)
		if !from.Equal(want) {
			t.Fatalf("from = %s, mau %s (watermark - overlap)", from, want)
		}
	})

	// Watermark rusak harus gagal ke arah AMAN: tarik semuanya. Menganggapnya
	// "baru saja sync" akan menelan riwayat tanpa jejak.
	t.Run("watermark rusak jatuh ke import penuh", func(t *testing.T) {
		from, full := historySyncWindow("bukan-tanggal", now)
		if !full || !from.Equal(mt5HistoryEpoch) {
			t.Fatalf("watermark rusak harus memicu import penuh, dapat from=%s full=%v", from, full)
		}
	})
}

// INVARIAN INTI: watermark hanya boleh maju setelah data benar-benar tersimpan.
// Kalau ia maju saat fetch gagal, rentang itu tidak akan pernah ditarik lagi dan
// trade di dalamnya hilang selamanya — persis kelas bug yang ditambal konstanta
// 30 hari di app.go.
func TestSyncHistoryFromMT5_WatermarkOnlyAdvancesOnSuccess(t *testing.T) {
	old := fetchHistoryBetweenFn
	defer func() { fetchHistoryBetweenFn = old }()

	SaveSetting(lastHistorySyncKey, "")

	fetchHistoryBetweenFn = func(from, to time.Time) ([]Trade, string, error) {
		return nil, "", errors.New("MT5 tertutup")
	}
	if _, err := SyncHistoryFromMT5(); err == nil {
		t.Fatal("fetch gagal harus mengembalikan error")
	}
	if got := GetSetting(lastHistorySyncKey); got != "" {
		t.Fatalf("watermark maju padahal fetch gagal: %q", got)
	}

	fetchHistoryBetweenFn = func(from, to time.Time) ([]Trade, string, error) {
		return []Trade{{Ticket: "hw-1", Symbol: "XAUUSD", Type: "Buy", Volume: 1, CloseTime: to}}, "Real", nil
	}
	res, err := SyncHistoryFromMT5()
	if err != nil {
		t.Fatalf("sync sukses tak terduga gagal: %v", err)
	}
	if !res.FullImport {
		t.Fatal("watermark masih kosong, harusnya ditandai import penuh")
	}
	if GetSetting(lastHistorySyncKey) == "" {
		t.Fatal("watermark tidak maju setelah sync sukses")
	}
}
