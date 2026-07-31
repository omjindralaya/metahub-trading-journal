package main

import (
	"context"
	"fmt"
	"log"
	"trading-journal/backend"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// GetCurrency mengambil pengaturan mata uang dari database
func (a *App) GetCurrency() string {
	curr := backend.GetSetting("currency")
	if curr == "" {
		return "$"
	}
	return curr
}


// AddManualTrade menambahkan trade secara manual ke database
func (a *App) AddManualTrade(trade backend.Trade) error {
	return backend.AddManualTrade(trade)
}

// FetchOpenPositions mengambil posisi yang masih floating dari MT5
func (a *App) FetchOpenPositions() ([]backend.OpenPosition, error) {
	return backend.FetchOpenPositions()
}

// GetMT5AccountInfo mengecek akun MT5 yang sedang login (Real/Demo, mata uang, server)
func (a *App) GetMT5AccountInfo() (backend.MT5AccountInfo, error) {
	return backend.GetMT5AccountInfo()
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Segarkan hak akses sekali saat app dibuka (langganan bisa saja habis atau
	// baru dibeli sejak sesi lalu), lalu berkala tiap 6 jam. Best-effort: bila
	// server tak terjangkau, cache lama tetap berlaku sampai masa tenggang habis.
	go func() {
		if backend.IsCloudConnected() {
			if err := backend.RefreshEntitlement(); err != nil {
				log.Printf("startup: gagal memuat hak akses (%v)", err)
			}
		}
	}()
	backend.StartEntitlementRefresher()

	// Teruskan laporan kemajuan sync ke UI. Paket backend sengaja tidak tahu
	// soal Wails; lapisan inilah yang menjembatani.
	backend.SetSyncProgressHandler(func(p backend.SyncProgress) {
		runtime.EventsEmit(a.ctx, "sync:progress", p)
	})

	// Teruskan status guard target-akun ke UI: saat akun MT5 aktif bukan akun yang
	// user pilih untuk di-sync, backend memancarkan 'sync:target' agar frontend bisa
	// menampilkan banner jujur (bukan menyembunyikan bahwa akun salah sedang aktif).
	backend.SetSyncTargetHandler(func(e backend.SyncTargetEvent) {
		runtime.EventsEmit(a.ctx, "sync:target", e)
	})

	// Catch-up sekali di startup: loop 5 detik hanya menarik 3 hari terakhir,
	// jadi bila app sempat tertutup ada closed trade yang tak masuk SQLite lewat
	// auto-sync. Jendelanya turun dari watermark, bukan konstanta — app yang
	// tertutup berbulan-bulan tetap menyusul utuh, dan pemasangan pertama menarik
	// seluruh riwayat. Kegagalan (mis. MT5 belum siap) dicatat, tidak menghentikan
	// app; watermark tidak maju sehingga percobaan berikutnya mengulang rentang.
	go func() {
		res, err := backend.SyncHistoryFromMT5()
		if err != nil {
			log.Printf("startup catch-up: lewati (%v)", err)
			return
		}
		log.Printf("startup catch-up: %d trade ditarik sejak %s (import penuh=%v)",
			res.Trades, res.Since.Format("2006-01-02"), res.FullImport)

		// Catch-up di atas hanya mengisi SQLite. Tanpa dorongan eksplisit di sini,
		// trade yang tertutup saat aplikasi mati baru sampai ke cloud ketika loop
		// 5 detik kebetulan mendeteksi close berikutnya — bisa berjam-jam. Aman
		// diulang tiap startup: yang dikirim hanya yang cloud_synced_at-nya kosong,
		// dan server men-dedupe lewat hash. Best-effort; gerbang entitlement dan
		// akun target ada di dalam fungsinya.
		if msg, err := backend.PushPendingClosedToCloud(); err != nil {
			log.Printf("startup: dorongan trade tertunda gagal (%v)", err)
		} else if msg != "" {
			log.Printf("startup: dorongan trade tertunda — %s", msg)
		}

		// Rekonstruksi modal akun dari balance-deal MT5 (setoran/penarikan) sekali
		// per startup, agar perubahan modal sejak sesi lalu ikut terhitung. Basis
		// persentase max drawdown di cloud. Best-effort; kegagalan hanya dicatat.
		if _, err := backend.RefreshAccountModal(); err != nil {
			log.Printf("startup: gagal rekonstruksi modal akun (%v)", err)
		}
	}()
}

// shutdown is called when the app closes. Tutup koneksi MT5 bersama supaya
// heartbeat/reconnect goroutine berhenti rapi.
func (a *App) shutdown(ctx context.Context) {
	backend.CloseMT5()
}

// FetchFromMT5 menarik riwayat MT5 ke SQLite. Tidak lagi menerima jumlah hari:
// jendelanya diturunkan dari watermark sync terakhir (seluruh riwayat bila belum
// pernah), sehingga tidak ada trade yang tertinggal hanya karena berada di luar
// konstanta yang dipilih sembarang.
func (a *App) FetchFromMT5() (string, error) {
	res, err := backend.SyncHistoryFromMT5()
	if err != nil {
		return "", fmt.Errorf("gagal mengambil data: %v", err)
	}

	if res.FullImport {
		return fmt.Sprintf("Import awal selesai: %d transaksi dari seluruh riwayat MT5\n(Akun: %s)", res.Trades, res.AccountType), nil
	}
	// Sebutkan jendelanya. Tanpa itu, "0 transaksi" tidak bisa dibedakan antara
	// "tidak ada yang baru" dan "ada, tapi di luar jangkauan sync".
	return fmt.Sprintf("Berhasil mensinkronisasi %d transaksi dari MT5\n(riwayat sejak %s, Akun: %s)",
		res.Trades, res.Since.Format("2 Jan 2006"), res.AccountType), nil
}

// GetDashboardData returns trades based on a period filter (e.g. "today", "30", "60", "90", "all")
func (a *App) GetDashboardData(period string) ([]backend.Trade, error) {
	return backend.GetTradesByPeriod(period)
}

// LoginWithGoogle opens the user's real browser to finish Google sign-in
// there (Google blocks its own sign-in inside an embedded app webview), then
// blocks until that browser tab completes it, the user cancels
// (CancelGoogleLogin), or it times out. The frontend awaits this as a normal
// promise and shows a "waiting in browser" state while it's pending.
func (a *App) LoginWithGoogle() error {
	return backend.LoginWithGoogleDesktop(func(url string) {
		runtime.BrowserOpenURL(a.ctx, url)
	})
}

// CancelGoogleLogin stops an in-flight LoginWithGoogle call.
func (a *App) CancelGoogleLogin() {
	backend.CancelGoogleLogin()
}

// No OpenRegisterPage: signing up is no longer a separate step. The first
// Google sign-in creates the account, so the login screen has nothing to link to.

// LogoutCloud logs out from MetaHub Cloud
func (a *App) LogoutCloud() error {
	return backend.LogoutCloud()
}

// IsCloudConnected checks if the user is logged in
func (a *App) IsCloudConnected() bool {
	return backend.IsCloudConnected()
}

// GetCloudProfile returns the user's cloud profile if logged in
func (a *App) GetCloudProfile() backend.CloudProfile {
	return backend.GetCloudProfile()
}

// SyncToCloud syncs local trades to MetaHub Cloud API
func (a *App) SyncToCloud() (string, error) {
	return backend.SyncToCloud()
}

// SyncOpenPositionsToCloud syncs live floating positions to MetaHub Cloud API
func (a *App) SyncOpenPositionsToCloud() (string, error) {
	return backend.SyncOpenPositionsToCloud()
}

// SyncRecentClosedToCloud syncs only today's closed trades so closed-trade posts
// appear in the social feed automatically, without the manual full-history sync.
func (a *App) SyncRecentClosedToCloud() (string, error) {
	return backend.SyncRecentClosedToCloud()
}

// AutoSyncTick is the single entrypoint the frontend calls every 5s. All cadence
// logic lives in Go: it pushes live open positions and only triggers a closed-trade
// sync when a position actually closed (volume-change detection), so idle closed-sync
// server load stays at zero.
func (a *App) AutoSyncTick() error {
	return backend.AutoSyncTick()
}

// SetSyncTarget records which MT5 account (login+server) the user chose to sync.
// Called by the UI's "switch target to the active account" action.
func (a *App) SetSyncTarget(login, server string) error {
	if err := backend.SaveSetting("sync_target_login", login); err != nil {
		return err
	}
	return backend.SaveSetting("sync_target_server", server)
}

// GetSyncTarget returns the currently chosen sync target for display.
func (a *App) GetSyncTarget() backend.SyncTargetInfo {
	return backend.GetSyncTarget()
}

// GetAutoBackfillEnabled melaporkan setelan toggle backfill ke UI. Default ON:
// setelah upgrade, riwayat lama yang sebelumnya tertahan jendela dikirim ulang
// otomatis tanpa user harus menekan sync manual.
func (a *App) GetAutoBackfillEnabled() bool {
	return backend.AutoBackfillEnabled()
}

// SetAutoBackfillEnabled menyimpan setelan toggle backfill dari UI.
func (a *App) SetAutoBackfillEnabled(on bool) error {
	return backend.SetAutoBackfillEnabled(on)
}

// GetEntitlement returns the cached subscription entitlement plus whether cloud
// sync is allowed right now (cache fresh enough AND the server said yes).
//
// This drives the UI only. The server re-checks on every write, so a user who
// edits journal.db to flip can_sync gets nothing but a sync the server refuses.
func (a *App) GetEntitlement() backend.EntitlementView {
	return backend.GetEntitlementView()
}

// RefreshEntitlement re-asks the server for the current entitlement. Called by
// the frontend after login and when the user clicks "check again" on the banner.
func (a *App) RefreshEntitlement() error {
	return backend.RefreshEntitlement()
}

// OpenUpgradePage opens the web upgrade page in the user's real browser. Payment
// lives on the web, not in the desktop app: there is no checkout to reimplement
// here, and the browser is where the user's saved cards and 3-D Secure live.
func (a *App) OpenUpgradePage() {
	runtime.BrowserOpenURL(a.ctx, backend.UpgradeURL())
}
