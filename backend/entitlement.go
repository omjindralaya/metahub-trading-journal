package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

// Masa tenggang cache hak akses. Selama server tak bisa dihubungi, jawaban
// terakhirnya tetap berlaku selama ini — supaya pelanggan yang membayar tidak
// kehilangan sync hanya karena server kita sedang mati.
//
// Setelah lewat, klien berhenti mendorong (fail closed). Ini tidak pernah
// mengunci user keluar dari APLIKASI: MT5 → jurnal lokal terus jalan.
const entitlementGracePeriod = 7 * 24 * time.Hour

// Seberapa sering hak akses disegarkan saat app berjalan.
const entitlementRefreshInterval = 6 * time.Hour

const (
	keyEntitlementCanSync       = "entitlement_can_sync"
	keyEntitlementTier          = "entitlement_tier"
	keyEntitlementEffectiveTier = "entitlement_effective_tier"
	keyEntitlementCheckedAt     = "entitlement_checked_at"
	keyEntitlementSyncWindow    = "entitlement_sync_window_days"
)

// codeDesktopNotEntitled adalah penolakan server saat paket user tidak termasuk
// sync ke cloud. Berbeda dari 403 lain: mengulang tidak akan menembusnya.
const codeDesktopNotEntitled = "DESKTOP_NOT_ENTITLED"

// Entitlement adalah jawaban server yang di-cache. Ini HANYA menentukan tampilan
// dan apakah klien repot-repot mengirim request. Gerbang yang sesungguhnya ada di
// server (middleware RequireDesktopEntitlement) — cache ini tersimpan di
// journal.db, SQLite biasa yang bisa diedit siapa pun pemilik mesinnya.
type Entitlement struct {
	CanSync       bool   `json:"can_sync_desktop"`
	Tier          string `json:"tier"`
	EffectiveTier string `json:"effective_tier"`
	// SyncWindowDays: 0 = tanpa batas. Dipakai untuk mendeteksi upgrade
	// (jendela MELEBAR → backfill) dan menyaring kiriman sebelum dikirim.
	SyncWindowDays int       `json:"sync_window_days"`
	CheckedAt      time.Time `json:"checked_at"`
}

// entitlementResponse mencocokkan envelope /me/entitlement.
type entitlementResponse struct {
	Success bool        `json:"success"`
	Data    Entitlement `json:"data"`
	Error   *CloudError `json:"error"`
}

// decideCanSync adalah fungsi murni: boleh mendorong ke cloud hanya bila jawaban
// terakhir server BERKATA boleh DAN jawaban itu belum terlalu tua. Cache kosong
// (CheckedAt nol) berarti belum ada bukti apa pun → tidak boleh.
func decideCanSync(e Entitlement, now time.Time) bool {
	if !e.CanSync || e.CheckedAt.IsZero() {
		return false
	}
	return now.Sub(e.CheckedAt) < entitlementGracePeriod
}

// CanSyncToCloud melaporkan apakah klien boleh mendorong ke cloud sekarang.
func CanSyncToCloud() bool {
	return decideCanSync(CachedEntitlement(), time.Now())
}

// CachedEntitlement membaca jawaban server terakhir dari settings lokal.
func CachedEntitlement() Entitlement {
	e := Entitlement{
		CanSync:       GetSetting(keyEntitlementCanSync) == "1",
		Tier:          GetSetting(keyEntitlementTier),
		EffectiveTier: GetSetting(keyEntitlementEffectiveTier),
	}
	if raw := GetSetting(keyEntitlementCheckedAt); raw != "" {
		if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
			e.CheckedAt = time.Unix(unix, 0)
		}
	}
	if raw := GetSetting(keyEntitlementSyncWindow); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			e.SyncWindowDays = n
		}
	}
	return e
}

// SaveEntitlement menyimpan jawaban server ke settings lokal.
func SaveEntitlement(e Entitlement) {
	canSync := "0"
	if e.CanSync {
		canSync = "1"
	}
	SaveSetting(keyEntitlementCanSync, canSync)
	SaveSetting(keyEntitlementTier, e.Tier)
	SaveSetting(keyEntitlementEffectiveTier, e.EffectiveTier)
	SaveSetting(keyEntitlementCheckedAt, strconv.FormatInt(e.CheckedAt.Unix(), 10))
	SaveSetting(keyEntitlementSyncWindow, strconv.Itoa(e.SyncWindowDays))
}

// InvalidateEntitlement mematikan hak sync di klien SEKETIKA. Dipanggil saat
// server menjawab DESKTOP_NOT_ENTITLED: jawaban itu lebih baru dan lebih berwenang
// daripada apa pun yang ada di cache, jadi masa tenggang tidak berlaku untuknya.
//
// CheckedAt sengaja diperbarui ke sekarang: ini BUKAN cache basi, ini jawaban
// segar yang kebetulan berbunyi "tidak".
func InvalidateEntitlement() {
	cached := CachedEntitlement()
	SaveEntitlement(Entitlement{
		CanSync:        false,
		Tier:           cached.Tier,
		EffectiveTier:  cached.EffectiveTier,
		SyncWindowDays: cached.SyncWindowDays,
		CheckedAt:      time.Now(),
	})
}

// ClearEntitlement membuang cache sepenuhnya. Dipakai saat logout: hak akses
// milik user yang tadi login, bukan milik mesin ini.
func ClearEntitlement() {
	SaveSetting(keyEntitlementCanSync, "")
	SaveSetting(keyEntitlementTier, "")
	SaveSetting(keyEntitlementEffectiveTier, "")
	SaveSetting(keyEntitlementCheckedAt, "")
	SaveSetting(keyEntitlementSyncWindow, "")
}

// EntitlementView adalah bentuk yang dibaca frontend. CanSyncNow adalah keputusan
// yang sudah jadi (cache + masa tenggang) supaya UI tidak perlu menghitung ulang
// aturan yang sama dan berisiko berbeda dari Go.
type EntitlementView struct {
	CanSyncNow    bool   `json:"can_sync_now"`
	Tier          string `json:"tier"`
	EffectiveTier string `json:"effective_tier"`
	// Expired benar bila user pernah membeli tier berbayar tapi tier yang berlaku
	// sudah turun ke free — pesannya beda dari "belum pernah beli".
	Expired bool `json:"expired"`
	// Stale benar bila kita belum bisa memastikan hak akses ke server dan masa
	// tenggang sudah habis. Sync mati, tapi karena kita tidak tahu — bukan karena
	// user tidak berhak. UI harus mengatakan itu apa adanya.
	Stale bool `json:"stale"`
}

// GetEntitlementView menerjemahkan cache jadi keputusan siap-pakai untuk UI.
func GetEntitlementView() EntitlementView {
	e := CachedEntitlement()
	now := time.Now()
	stale := e.CanSync && !e.CheckedAt.IsZero() && now.Sub(e.CheckedAt) >= entitlementGracePeriod
	return EntitlementView{
		CanSyncNow:    decideCanSync(e, now),
		Tier:          e.Tier,
		EffectiveTier: e.EffectiveTier,
		Expired:       e.EffectiveTier == "free" && e.Tier != "" && e.Tier != "free",
		Stale:         stale,
	}
}

// upgradeURL adalah halaman upgrade di web. Variabel supaya deployment bisa
// mengarahkannya tanpa rebuild.
var upgradeURL = "https://metahub.id/upgrade"

// UpgradeURL mengembalikan halaman upgrade yang dibuka di browser.
func UpgradeURL() string { return upgradeURL }

// SetUpgradeURL dipakai konfigurasi lingkungan / test.
func SetUpgradeURL(u string) { upgradeURL = u }

// Tidak ada registerURL lagi: pendaftaran bukan langkah terpisah — login Google
// pertama sekaligus membuat akun, jadi layar login tak punya tautan "daftar".

// StartEntitlementRefresher menyegarkan hak akses secara berkala selama app hidup,
// sehingga langganan yang habis atau baru dibeli tercermin tanpa restart.
// Kegagalan hanya dicatat: cache lama tetap berlaku sampai masa tenggang habis.
func StartEntitlementRefresher() {
	go func() {
		ticker := time.NewTicker(entitlementRefreshInterval)
		defer ticker.Stop()
		for range ticker.C {
			if loadMetahubJWT() == "" {
				continue
			}
			if err := RefreshEntitlement(); err != nil {
				log.Printf("entitlement refresh: %v", err)
			}
		}
	}()
}

// RefreshEntitlement menanyakan hak akses terkini ke server dan menyimpannya.
//
// Kegagalan koneksi TIDAK menghapus cache: server mati bukan alasan mencabut hak
// pelanggan yang membayar. Cache lama tetap berlaku sampai masa tenggang habis.
func RefreshEntitlement() error {
	token := loadMetahubJWT()
	if token == "" {
		return fmt.Errorf("belum login")
	}

	req, err := http.NewRequest("GET", CloudAPIURL+"/me/entitlement", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gagal menghubungi server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		// Sesi mati: hak akses tidak lagi milik siapa pun di mesin ini.
		LogoutCloud()
		return fmt.Errorf("sesi berakhir, silakan login lagi")
	}

	body, _ := io.ReadAll(resp.Body)
	var parsed entitlementResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("respons server tidak dikenali (status %d)", resp.StatusCode)
	}
	if !parsed.Success || resp.StatusCode != http.StatusOK {
		msg := parsed.Error.Text()
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		return fmt.Errorf("gagal memeriksa paket: %s", msg)
	}

	// CheckedAt selalu diambil dari jam LOKAL, bukan dari server: umur cache diukur
	// terhadap jam yang sama yang nanti membacanya, jadi selisih jam server/klien
	// tidak diam-diam memperpanjang atau memangkas masa tenggang.
	e := parsed.Data
	e.CheckedAt = time.Now()

	// Bandingkan SEBELUM cache ditimpa — sesudahnya jendela lama sudah hilang
	// dan upgrade tidak bisa dideteksi lagi.
	applyWindowChange(CachedEntitlement().SyncWindowDays, e.SyncWindowDays)

	SaveEntitlement(e)
	return nil
}
