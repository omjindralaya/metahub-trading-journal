package backend

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// stubMT5 mengganti seam yang menyentuh MT5 supaya AutoSyncTick bisa diuji tanpa
// terminal MT5 hidup. Deal probe ikut distub dengan jawaban tetap (tidak ada deal
// baru) supaya test lama tidak mencoba menghubungi MT5 sungguhan; test yang
// memang menguji probe memakai scriptDealCounts setelahnya.
func stubMT5(t *testing.T, positions []OpenPosition, localRefreshes *int32) func() {
	t.Helper()
	oldFetch, oldRefresh, oldCount := fetchOpenPositionsFn, refreshLocalFromMT5Fn, countDealsBetweenFn
	fetchOpenPositionsFn = func() ([]OpenPosition, error) { return positions, nil }
	refreshLocalFromMT5Fn = func(days int) error {
		atomic.AddInt32(localRefreshes, 1)
		return nil
	}
	countDealsBetweenFn = func(from, to time.Time) (int, error) { return 0, nil }
	resetDealProbe()
	return func() {
		fetchOpenPositionsFn, refreshLocalFromMT5Fn = oldFetch, oldRefresh
		countDealsBetweenFn = oldCount
		resetDealProbe()
	}
}

// scriptDealCounts memberi jawaban probe berurutan per tick; jawaban terakhir
// dipakai ulang bila tick melebihi skrip.
func scriptDealCounts(counts ...int) {
	var i int32
	countDealsBetweenFn = func(from, to time.Time) (int, error) {
		n := int(atomic.AddInt32(&i, 1)) - 1
		if n >= len(counts) {
			n = len(counts) - 1
		}
		return counts[n], nil
	}
}

// Trade yang dibuka DAN ditutup di antara dua tick tidak pernah muncul di
// snapshot posisi terbuka, jadi diff volume buta terhadapnya. Inilah bug yang
// membuat scalping/SL cepat tidak pernah sampai ke cloud sampai ada close
// berikutnya. Probe jumlah deal MT5 harus menangkapnya.
func TestAutoSyncTick_SubTickTrade_DetectedByDealProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"synced":0,"skipped":0}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	var localRefreshes int32
	// Posisi terbuka SELALU kosong: trade lahir dan mati di antara dua tick.
	defer stubMT5(t, []OpenPosition{}, &localRefreshes)()
	scriptDealCounts(4, 6) // tick 1 = baseline, tick 2 = dua deal baru

	SaveSetting("metahub_jwt", "tok")
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")
	defer clearSyncTargetSettings(t)
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", EffectiveTier: "pro", CheckedAt: time.Now()})
	resetSnapshot(map[string]float64{})

	if err := AutoSyncTick(); err != nil { // baseline
		t.Fatalf("tick baseline gagal: %v", err)
	}
	if got := atomic.LoadInt32(&localRefreshes); got != 0 {
		t.Fatalf("tick baseline tidak boleh memicu sync closed (%d refresh)", got)
	}

	if err := AutoSyncTick(); err != nil {
		t.Fatalf("tick kedua gagal: %v", err)
	}
	if got := atomic.LoadInt32(&localRefreshes); got == 0 {
		t.Fatal("deal baru di MT5 tidak memicu sync closed — trade sub-tick tetap hilang")
	}
}

// Tanpa deal baru, probe tidak boleh memicu apa pun: beban closed-sync saat idle
// harus tetap nol.
func TestAutoSyncTick_NoNewDeals_DoesNotSyncClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"synced":0,"skipped":0}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	var localRefreshes int32
	defer stubMT5(t, []OpenPosition{op("1", 1.0)}, &localRefreshes)()
	scriptDealCounts(9, 9, 9)

	SaveSetting("metahub_jwt", "tok")
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")
	defer clearSyncTargetSettings(t)
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", EffectiveTier: "pro", CheckedAt: time.Now()})
	resetSnapshot(map[string]float64{"1": 1.0})

	for i := 0; i < 3; i++ {
		if err := AutoSyncTick(); err != nil {
			t.Fatalf("tick %d gagal: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&localRefreshes); got != 0 {
		t.Fatalf("probe memicu sync closed padahal tak ada deal baru (%d refresh)", got)
	}
}

// Probe gagal (MT5 tertutup) tidak boleh mengarang baseline: tick berikutnya
// harus mengambil baseline dari angka sungguhan, bukan dari nol palsu — kalau
// tidak, angka nyata pertama akan terbaca sebagai lonjakan deal.
func TestAutoSyncTick_ProbeError_DoesNotFabricateBaseline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"synced":0,"skipped":0}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	var localRefreshes int32
	defer stubMT5(t, []OpenPosition{}, &localRefreshes)()

	var calls int32
	countDealsBetweenFn = func(from, to time.Time) (int, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return 0, fmt.Errorf("MT5 tertutup")
		}
		return 12, nil // angka sungguhan pertama: ini baseline, bukan lonjakan
	}

	SaveSetting("metahub_jwt", "tok")
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")
	defer clearSyncTargetSettings(t)
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", EffectiveTier: "pro", CheckedAt: time.Now()})
	resetSnapshot(map[string]float64{})

	for i := 0; i < 2; i++ {
		if err := AutoSyncTick(); err != nil {
			t.Fatalf("probe gagal tidak boleh menggagalkan tick (%d): %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&localRefreshes); got != 0 {
		t.Fatalf("angka probe pertama setelah error dibaca sebagai deal baru (%d refresh)", got)
	}
}

// seedPendingTrade menaruh satu trade yang belum pernah tersinkron ke cloud
// (cloud_synced_at NULL) dan membersihkannya di akhir test.
func seedPendingTrade(t *testing.T, ticket string) {
	t.Helper()
	if err := SaveTrades([]Trade{mustTrade(t, ticket, 12.5)}); err != nil {
		t.Fatalf("seed trade pending: %v", err)
	}
	t.Cleanup(func() { DB.Where("ticket = ?", ticket).Delete(&Trade{}) })
}

// Trade yang tertutup saat aplikasi MATI ditarik ke SQLite oleh startup
// catch-up, tapi dulu tak pernah didorong ke cloud sampai kebetulan ada close
// berikutnya. Baseline deal probe di tick pertama sengaja tidak memicu, jadi
// celah ini harus ditutup di startup — bukan oleh loop.
func TestPushPendingClosedToCloud_SendsUnsyncedTrades(t *testing.T) {
	var cloudCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&cloudCalls, 1)
		w.Write([]byte(`{"success":true,"data":{"synced":1,"skipped":0}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	SaveSetting("metahub_jwt", "tok")
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")
	defer clearSyncTargetSettings(t)
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", EffectiveTier: "pro", CheckedAt: time.Now()})

	seedPendingTrade(t, "startup-1")

	if _, err := PushPendingClosedToCloud(); err != nil {
		t.Fatalf("push pending gagal: %v", err)
	}
	if got := atomic.LoadInt32(&cloudCalls); got == 0 {
		t.Fatal("trade yang tertutup saat app mati tidak pernah sampai ke cloud")
	}
}

// Gerbang yang sama dengan loop: paket tak berhak / akun aktif bukan target sync
// tidak boleh mendorong apa pun, meski jurnal lokalnya penuh trade pending.
func TestPushPendingClosedToCloud_RespectsPushGates(t *testing.T) {
	var cloudCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&cloudCalls, 1)
		w.Write([]byte(`{"success":true,"data":{"synced":1,"skipped":0}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	SaveSetting("metahub_jwt", "tok")
	defer clearSyncTargetSettings(t)
	seedPendingTrade(t, "gated-1")

	// (a) paket tidak berhak
	SaveEntitlement(Entitlement{CanSync: false, Tier: "free", EffectiveTier: "free", CheckedAt: time.Now()})
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")
	if _, err := PushPendingClosedToCloud(); err != nil {
		t.Fatalf("paket tak berhak tidak boleh jadi error: %v", err)
	}

	// (b) berhak, tapi terminal aktif bukan akun yang dipilih user
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", EffectiveTier: "pro", CheckedAt: time.Now()})
	SaveSetting(syncTargetLoginKey, "500")
	SaveSetting(syncTargetServerKey, "BrokerA")
	SaveSetting("mt5_account", "999")
	if _, err := PushPendingClosedToCloud(); err != nil {
		t.Fatalf("ketidakcocokan akun tidak boleh jadi error: %v", err)
	}

	if got := atomic.LoadInt32(&cloudCalls); got != 0 {
		t.Fatalf("push startup menembus gerbang entitlement/target (%d request)", got)
	}
}

func TestDealProbe_AnchorDoesNotSlide(t *testing.T) {
	var p dealProbe
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)

	from1, _ := p.window(now)
	p.observe(3)
	from2, to2 := p.window(now.Add(time.Minute))

	if !from1.Equal(from2) {
		t.Fatalf("anker jendela ikut bergeser (%v → %v): deal lama yang keluar jendela bisa menutupi deal baru", from1, from2)
	}
	if !to2.After(now.Add(time.Minute)) {
		t.Fatal("batas atas jendela tidak diberi margin ke depan; jam server broker yang mendahului jam lokal akan memotong deal terbaru")
	}
}

func TestDealProbe_Observe(t *testing.T) {
	var p dealProbe
	now := time.Now()

	p.window(now)
	if p.observe(5) {
		t.Fatal("hitungan pertama adalah baseline, bukan bukti ada deal baru")
	}
	p.window(now)
	if p.observe(5) {
		t.Fatal("hitungan tetap dilaporkan sebagai deal baru")
	}
	p.window(now)
	if !p.observe(6) {
		t.Fatal("hitungan naik tidak dilaporkan sebagai deal baru")
	}
	p.window(now)
	if p.observe(6) {
		t.Fatal("hitungan yang sudah dilaporkan terpicu dua kali")
	}
	p.window(now)
	if p.observe(2) {
		t.Fatal("hitungan turun (deal dibersihkan broker) dilaporkan sebagai deal baru")
	}
	p.window(now)
	if !p.observe(3) {
		t.Fatal("setelah turun, kenaikan berikutnya harus terdeteksi dari dasar yang baru")
	}
}

// Sesi yang hidup berhari-hari harus menganker ulang jendelanya, dan penganker-
// an ulang TIDAK boleh terbaca sebagai deal baru (hitungannya tak sebanding).
func TestDealProbe_ReanchorIsNotANewDeal(t *testing.T) {
	var p dealProbe
	now := time.Now()

	p.window(now)
	p.observe(500)

	from2, _ := p.window(now.Add(maxDealProbeWindow + time.Hour))
	if !from2.After(now) {
		t.Fatal("jendela tidak dianker ulang setelah sesi panjang; rentangnya tumbuh tanpa batas")
	}
	if p.observe(4) {
		t.Fatal("hitungan dari jendela yang baru dianker dibandingkan dengan hitungan jendela lama")
	}
}

// resetSnapshot memaksa tick berikutnya melihat prev yang kita tentukan.
func resetSnapshot(prev map[string]float64) {
	openVolumeSnapshotMu.Lock()
	openVolumeSnapshot = prev
	openVolumeSnapshotMu.Unlock()
}

// INTI FITUR INI: paket yang tidak berhak mematikan dorongan ke cloud, TAPI
// aplikasi tetap bekerja — MT5 tetap ditarik ke jurnal lokal. Yang dijual adalah
// cloud, bukan aplikasinya.
func TestAutoSyncTick_NotEntitled_KeepsLocalJournalButPushesNothing(t *testing.T) {
	var cloudCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&cloudCalls, 1)
		w.Write([]byte(`{"success":true,"data":{"synced":0,"skipped":0}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	var localRefreshes int32
	defer stubMT5(t, []OpenPosition{}, &localRefreshes)()

	SaveSetting("metahub_jwt", "tok")
	SaveEntitlement(Entitlement{CanSync: false, Tier: "free", EffectiveTier: "free", CheckedAt: time.Now()})
	resetSnapshot(map[string]float64{"1": 1.0}) // posisi hilang → close terdeteksi

	if err := AutoSyncTick(); err != nil {
		t.Fatalf("tick tidak boleh gagal hanya karena paket tak berhak: %v", err)
	}

	if got := atomic.LoadInt32(&cloudCalls); got != 0 {
		t.Fatalf("paket tak berhak tetap mendorong ke cloud (%d request)", got)
	}
	if got := atomic.LoadInt32(&localRefreshes); got == 0 {
		t.Fatal("jurnal lokal berhenti terisi dari MT5 — paket seharusnya hanya mematikan cloud, bukan aplikasinya")
	}
}

func TestAutoSyncTick_Entitled_PushesToCloud(t *testing.T) {
	var cloudCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&cloudCalls, 1)
		w.Write([]byte(`{"success":true,"data":{"synced":0,"skipped":0}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	var localRefreshes int32
	defer stubMT5(t, []OpenPosition{op("1", 1.0)}, &localRefreshes)()

	SaveSetting("metahub_jwt", "tok")
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")
	defer clearSyncTargetSettings(t)
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", EffectiveTier: "pro", CheckedAt: time.Now()})
	resetSnapshot(map[string]float64{})

	if err := AutoSyncTick(); err != nil {
		t.Fatalf("tick gagal: %v", err)
	}

	if got := atomic.LoadInt32(&cloudCalls); got == 0 {
		t.Fatal("user berhak tidak mendorong posisi terbuka ke cloud")
	}
}

// Server menolak dengan DESKTOP_NOT_ENTITLED (mis. langganan habis di tengah
// sesi). Klien harus langsung berhenti mendorong, bukan mengulang tiap 5 detik.
func TestAutoSyncTick_ServerRejection_InvalidatesCacheImmediately(t *testing.T) {
	var cloudCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&cloudCalls, 1)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"success":false,"error":{"code":"DESKTOP_NOT_ENTITLED","message":"paket tidak termasuk sync"}}`)
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	var localRefreshes int32
	defer stubMT5(t, []OpenPosition{op("1", 1.0)}, &localRefreshes)()

	SaveSetting("metahub_jwt", "tok")
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")
	defer clearSyncTargetSettings(t)
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", EffectiveTier: "pro", CheckedAt: time.Now()})
	resetSnapshot(map[string]float64{})

	_ = AutoSyncTick() // percobaan pertama: dikirim, ditolak server

	if CanSyncToCloud() {
		t.Fatal("penolakan server harus langsung mematikan cache, bukan menunggu masa tenggang habis")
	}

	before := atomic.LoadInt32(&cloudCalls)
	_ = AutoSyncTick() // tick berikutnya harus diam
	if atomic.LoadInt32(&cloudCalls) != before {
		t.Fatal("klien terus menembaki server dengan request yang pasti ditolak")
	}
}

// A different account is active than the one the user chose to sync: the auto
// loop MUST NOT push it to the cloud, but the local journal must still update.
func TestAutoSyncTick_TargetMismatch_BlocksCloudButKeepsLocalJournal(t *testing.T) {
	var cloudCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&cloudCalls, 1)
		w.Write([]byte(`{"success":true,"data":{"synced":0,"skipped":0}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	var localRefreshes int32
	defer stubMT5(t, []OpenPosition{}, &localRefreshes)()

	SaveSetting("metahub_jwt", "tok")
	SaveEntitlement(Entitlement{CanSync: true, Tier: "pro", EffectiveTier: "pro", CheckedAt: time.Now()})

	// User chose account 500, but the active terminal is account 999.
	SaveSetting(syncTargetLoginKey, "500")
	SaveSetting(syncTargetServerKey, "BrokerA")
	SaveSetting("mt5_account", "999")
	SaveSetting("mt5_server", "BrokerA")
	defer clearSyncTargetSettings(t)

	resetSnapshot(map[string]float64{"1": 1.0}) // position gone → close detected

	if err := AutoSyncTick(); err != nil {
		t.Fatalf("tick must not error on a target mismatch: %v", err)
	}
	if got := atomic.LoadInt32(&cloudCalls); got != 0 {
		t.Fatalf("wrong account was pushed to the cloud (%d request)", got)
	}
	if got := atomic.LoadInt32(&localRefreshes); got == 0 {
		t.Fatal("local journal stopped updating — mismatch should only block the cloud push")
	}
}
