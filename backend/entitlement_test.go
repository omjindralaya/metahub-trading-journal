package backend

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDecideCanSync(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		cached Entitlement
		want   bool
	}{
		{
			name:   "cache berhak dan masih segar",
			cached: Entitlement{CanSync: true, CheckedAt: now.Add(-2 * time.Hour)},
			want:   true,
		},
		{
			name:   "cache tidak berhak",
			cached: Entitlement{CanSync: false, CheckedAt: now.Add(-2 * time.Hour)},
			want:   false,
		},
		{
			// Grace period habis dan server tak bisa dihubungi: berhenti mendorong ke
			// cloud. Server akan menolaknya juga; ini hanya berhenti lebih sopan.
			name:   "cache berhak tapi lewat masa tenggang → gagal tertutup",
			cached: Entitlement{CanSync: true, CheckedAt: now.Add(-8 * 24 * time.Hour)},
			want:   false,
		},
		{
			name:   "cache berhak tepat di batas masa tenggang masih jalan",
			cached: Entitlement{CanSync: true, CheckedAt: now.Add(-6 * 24 * time.Hour)},
			want:   true,
		},
		{
			// Belum pernah dicek (CheckedAt nol): tak ada bukti user berhak.
			name:   "cache kosong → gagal tertutup",
			cached: Entitlement{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideCanSync(tt.cached, now); got != tt.want {
				t.Errorf("decideCanSync() = %v, mau %v", got, tt.want)
			}
		})
	}
}

// Cache disimpan di journal.db — SQLite biasa yang bisa diedit user. Karena itu
// ia hanya boleh menentukan TAMPILAN. Test ini mengunci bahwa penolakan server
// langsung mematikan cache, supaya app tidak terus mendorong request yang pasti
// ditolak.
func TestEntitlementCacheRoundTripAndInvalidation(t *testing.T) {
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", EffectiveTier: "pro", CheckedAt: time.Now()})

	if got := CachedEntitlement(); !got.CanSync || got.Tier != "pro" {
		t.Fatalf("cache tidak kembali utuh: %+v", got)
	}
	if !CanSyncToCloud() {
		t.Fatal("user berhak dengan cache segar harus boleh sync")
	}

	InvalidateEntitlement()

	if CanSyncToCloud() {
		t.Fatal("setelah server menolak, cache harus mati seketika")
	}
	if CachedEntitlement().CanSync {
		t.Fatal("cache masih mengaku berhak setelah invalidasi")
	}
}

func TestRefreshEntitlementStoresServerAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/entitlement" {
			t.Errorf("endpoint salah: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("token tidak dikirim")
		}
		w.Write([]byte(`{"success":true,"data":{"tier":"pro","effective_tier":"free","can_sync_desktop":false,"checked_at":"2026-07-14T09:00:00Z"}}`))
	}))
	defer srv.Close()

	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	SaveSetting("metahub_jwt", "tok")
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", CheckedAt: time.Now()})

	if err := RefreshEntitlement(); err != nil {
		t.Fatalf("refresh gagal: %v", err)
	}

	got := CachedEntitlement()
	if got.CanSync {
		t.Fatal("langganan yang sudah kedaluwarsa di server harus mematikan sync di klien")
	}
	if got.EffectiveTier != "free" {
		t.Fatalf("effective_tier tidak tersimpan: %+v", got)
	}
}

// Server tak bisa dihubungi: JANGAN hapus cache. Selama masih dalam masa tenggang,
// user yang benar-benar membayar tetap bisa sync meski server kita sedang mati.
func TestRefreshEntitlementKeepsCacheWhenServerUnreachable(t *testing.T) {
	oldURL := CloudAPIURL
	SetCloudAPIURL("http://127.0.0.1:1") // pasti gagal konek
	defer SetCloudAPIURL(oldURL)

	SaveSetting("metahub_jwt", "tok")
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", CheckedAt: time.Now()})

	if err := RefreshEntitlement(); err == nil {
		t.Fatal("server mati harus melaporkan error, bukan ditelan diam-diam")
	}

	if !CanSyncToCloud() {
		t.Fatal("server mati tidak boleh mengunci pelanggan yang membayar keluar dari sync")
	}
}
