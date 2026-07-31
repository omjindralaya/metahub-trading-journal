package backend

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// op adalah helper ringkas untuk membuat OpenPosition hanya dengan ticket+volume,
// karena diffOpenVolumes hanya melihat kedua field itu.
func op(ticket string, volume float64) OpenPosition {
	return OpenPosition{Ticket: ticket, Volume: volume}
}

func TestDiffOpenVolumes(t *testing.T) {
	tests := []struct {
		name    string
		current []OpenPosition
		prev    map[string]float64
		want    bool
	}{
		{
			name:    "empty prev (first tick) is never a close",
			current: []OpenPosition{op("1", 1.0), op("2", 2.0)},
			prev:    map[string]float64{},
			want:    false,
		},
		{
			name:    "no change",
			current: []OpenPosition{op("1", 1.0)},
			prev:    map[string]float64{"1": 1.0},
			want:    false,
		},
		{
			name:    "full close (ticket disappears)",
			current: []OpenPosition{},
			prev:    map[string]float64{"1": 1.0},
			want:    true,
		},
		{
			name:    "partial close (volume shrinks)",
			current: []OpenPosition{op("1", 0.5)},
			prev:    map[string]float64{"1": 1.0},
			want:    true,
		},
		{
			name:    "new open (new ticket appears)",
			current: []OpenPosition{op("1", 1.0), op("2", 2.0)},
			prev:    map[string]float64{"1": 1.0},
			want:    false,
		},
		{
			name:    "scale-in (volume increases)",
			current: []OpenPosition{op("1", 2.0)},
			prev:    map[string]float64{"1": 1.0},
			want:    false,
		},
		{
			name:    "one closes while another opens",
			current: []OpenPosition{op("2", 2.0)},
			prev:    map[string]float64{"1": 1.0},
			want:    true,
		},
		{
			name:    "float noise below epsilon is not a close",
			current: []OpenPosition{op("1", 1.0 - 1e-9)},
			prev:    map[string]float64{"1": 1.0},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := diffOpenVolumes(tt.current, tt.prev); got != tt.want {
				t.Errorf("diffOpenVolumes() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Payload sync TIDAK BOLEH lagi memuat data_hash. Kolom itu teater keamanan:
// rumusnya ada di source code, jadi penyerang bisa mengarang trade lalu menghitung
// hash-nya sendiri.
func TestSyncPayloadNoLongerCarriesDataHash(t *testing.T) {
	item := TradeSyncItem{Ticket: "1", Symbol: "XAUUSD", Type: "Buy"}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("data_hash")) {
		t.Fatalf("payload masih mengirim data_hash: %s", raw)
	}
}

// Sync yang sah membawa keempat header tanda tangan.
func TestPushClosedTradesSignsRequest(t *testing.T) {
	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.Write([]byte(`{"success":true,"data":{"synced":1,"skipped":0}}`))
	}))
	defer srv.Close()

	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	SaveSetting(deviceIDSettingKey, "33333333-3333-3333-3333-333333333333")

	if _, err := pushClosedTradesToCloud("token", []Trade{{
		Ticket: "1", Symbol: "XAUUSD", Type: "Buy", Volume: 0.1, NetProfit: 10,
	}}); err != nil {
		t.Fatal(err)
	}

	for _, h := range []string{"X-Device-ID", "X-Timestamp", "X-Nonce", "X-Signature"} {
		if headers.Get(h) == "" {
			t.Fatalf("sync dikirim tanpa header %s", h)
		}
	}
}

// Akun MT5 (login+server) sudah dimiliki user MetaHub lain: backend menolak 409
// dengan kode MT5_ACCOUNT_CLAIMED. Desktop harus memunculkan pesan yang jelas dan
// mengarahkan (bukan menyuruh retry) — ini bukan kegagalan sementara.
func TestPushClosedTradesReportsAccountClaimed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"success":false,"error":{"code":"MT5_ACCOUNT_CLAIMED","message":"Akun MT5 ini sudah terhubung ke pengguna lain."}}`))
	}))
	defer srv.Close()

	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	_, err := pushClosedTradesToCloud("token", []Trade{{Ticket: "1", Symbol: "XAUUSD", Type: "Buy"}})
	if err == nil {
		t.Fatal("akun yang sudah diklaim user lain harus menggagalkan sync")
	}
	if !strings.Contains(err.Error(), "hubungi admin") {
		t.Fatalf("pesan akun-diklaim tidak jelas: %v", err)
	}
}

func TestPushOpenPositionsReportsAccountClaimed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"success":false,"error":{"code":"MT5_ACCOUNT_CLAIMED","message":"claimed"}}`))
	}))
	defer srv.Close()

	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	_, err := pushOpenPositions("token", []OpenPosition{{Ticket: "1", Symbol: "XAUUSD", Volume: 0.1}})
	if err == nil {
		t.Fatal("posisi terbuka pada akun yang diklaim user lain harus ditolak")
	}
	if !strings.Contains(err.Error(), "hubungi admin") {
		t.Fatalf("pesan akun-diklaim tidak jelas: %v", err)
	}
}

// Device belum terdaftar: sync TETAP dikirim, tanpa tanda tangan. Selama Fase A
// server menerimanya sebagai `unsigned`. Menggagalkan sync di sini memutus setiap
// user pada rilis pertama.
func TestPushClosedTradesStillSyncsWhenDeviceUnregistered(t *testing.T) {
	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.Write([]byte(`{"success":true,"data":{"synced":1,"skipped":0}}`))
	}))
	defer srv.Close()

	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	SaveSetting(deviceIDSettingKey, "") // belum terdaftar

	if _, err := pushClosedTradesToCloud("token", []Trade{{
		Ticket: "1", Symbol: "XAUUSD", Type: "Buy",
	}}); err != nil {
		t.Fatalf("sync harus tetap jalan tanpa device terdaftar: %v", err)
	}
	if headers.Get("X-Signature") != "" {
		t.Fatal("request tak terdaftar tidak boleh membawa tanda tangan setengah jalan")
	}
}
