package backend

import (
	"bytes"
	"errors"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// CloudAPIURL adalah base URL API cloud. Variabel (bukan konstanta) supaya test
// bisa mengarahkannya ke httptest server.
var CloudAPIURL = "http://localhost:8080/api/v1"

// SetCloudAPIURL hanya untuk test dan konfigurasi lingkungan.
func SetCloudAPIURL(u string) { CloudAPIURL = u }

// CloudError adalah objek error terstruktur pada envelope metahub-api terbaru.
// Sejak backend memakai pkg/response, field "error" bukan lagi string melainkan
// objek {code, message, details}. Client HARUS mem-parse bentuk ini, kalau tidak
// pesan error dari server (mis. "email already used", "session expired") hilang.
type CloudError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Text mengembalikan pesan yang aman ditampilkan ke user. Nil-safe supaya
// pemanggil bisa langsung memakai resp.Error.Text() tanpa cek nil manual.
func (e *CloudError) Text() string {
	if e == nil {
		return ""
	}
	if e.Message != "" && e.Code != "" {
		return fmt.Sprintf("%s (%s)", e.Message, e.Code)
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// codeMT5AccountClaimed adalah kode error backend saat akun MT5 (login+server)
// ini sudah terhubung ke user MetaHub lain. Ini BUKAN kegagalan sementara —
// mengulang sync tidak akan menembusnya — jadi pesannya mengarahkan user, bukan
// menyuruh mencoba lagi.
const codeMT5AccountClaimed = "MT5_ACCOUNT_CLAIMED"

// syncRejection membangun error yang ditampilkan ke user saat server menolak sync.
// action adalah awalan konteks (mis. "sync ditolak"); status dipakai sebagai
// fallback saat server tidak mengirim pesan. Kode akun-diklaim ditangani khusus
// dengan pesan Indonesia yang jelas + arahan, tidak peduli teks server.
func syncRejection(action string, e *CloudError, status int) error {
	if e != nil && e.Code == codeMT5AccountClaimed {
		return fmt.Errorf("Akun MT5 ini sudah terhubung ke pengguna MetaHub lain. " +
			"Satu akun MT5 hanya bisa dimiliki satu pengguna. " +
			"Jika ini benar akunmu, hubungi admin.")
	}
	if e != nil && e.Code == codeDesktopNotEntitled {
		// Jawaban server ini lebih berwenang daripada apa pun di cache lokal:
		// matikan sekarang, supaya loop 5 detik berhenti menembaki server dengan
		// request yang sudah pasti ditolak.
		InvalidateEntitlement()
		return fmt.Errorf("Paket langganan Anda tidak termasuk sync ke cloud. " +
			"Jurnal lokal tetap berjalan. Tingkatkan paket untuk mengaktifkan sync.")
	}
	msg := e.Text()
	if msg == "" {
		msg = fmt.Sprintf("status %d", status)
	}
	return fmt.Errorf("%s: %s", action, msg)
}

// CloudUserProfile matches the "user" object metahub-api embeds in every login
// response (password, Google web, and the desktop Google handoff alike).
type CloudUserProfile struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Tier        string `json:"tier"`
}

// LoginResponse matches the metahub-api response envelope (pkg/response).
type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Token string           `json:"token"`
		User  CloudUserProfile `json:"user"`
	} `json:"data"`
	Error     *CloudError `json:"error"`
	RequestID string      `json:"request_id"`
}

// SyncResponse matches the metahub-api sync response envelope (pkg/response).
type SyncResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Synced  int `json:"synced"`
		Skipped int `json:"skipped"`

		// OutOfWindow & OutOfWindowTickets ada sejak server mengenal batas sync
		// per-tier. Server LAMA tidak mengirimkannya sama sekali; nilai nol yang
		// dihasilkan json.Unmarshal berarti "tidak ada yang ditolak", yang persis
		// perilaku lama — jadi desktop baru aman terhadap server lama.
		OutOfWindow        int      `json:"out_of_window"`
		OutOfWindowTickets []string `json:"out_of_window_tickets"`

		// SyncFloorAt adalah lantai AKUN yang barusan dipakai. Disimpan desktop
		// sebagai penyaring optimistik; jawaban server tetap yang berwenang.
		SyncFloorAt *time.Time `json:"sync_floor_at"`
	} `json:"data"`
	Error     *CloudError `json:"error"`
	RequestID string      `json:"request_id"`
}

// TradeSyncItem matches the metahub-api TradeSyncItem payload
type TradeSyncItem struct {
	Ticket      string    `json:"ticket"`
	PositionID  string    `json:"position_id"`
	Symbol      string    `json:"symbol"`
	Type        string    `json:"type"`
	Volume      float64   `json:"volume"`
	OpenPrice   float64   `json:"open_price"`
	ClosePrice  float64   `json:"close_price"`
	OpenTime    time.Time `json:"open_time"`
	CloseTime   time.Time `json:"close_time"`
	SL          float64   `json:"sl"`
	TP          float64   `json:"tp"`
	Commission  float64   `json:"commission"`
	Swap        float64   `json:"swap"`
	Profit      float64   `json:"profit"`
	NetProfit   float64   `json:"net_profit"`
	AccountType string    `json:"account_type"`
}

// signSyncRequest menandatangani request sync bila device sudah terdaftar.
// Kegagalan TIDAK menghentikan sync: selama Fase A server menerima request tak
// bertanda tangan (mencatatnya sebagai `unsigned`). Saat server dinyalakan ke
// Fase B, request tak bertanda tangan mulai ditolak 401 — dan pesan servernya
// yang memberi tahu user untuk memperbarui app.
func signSyncRequest(req *http.Request, body []byte) {
	provider, err := GetKeyProvider()
	if err != nil {
		log.Printf("signSyncRequest: key device tidak tersedia (%v); mengirim tanpa tanda tangan", err)
		return
	}
	if err := signRequest(req, body, provider); err != nil {
		log.Printf("signSyncRequest: %v; mengirim tanpa tanda tangan", err)
	}
}

// codeDeviceSignatureInvalid HARUS identik dengan middleware.CodeDeviceSignatureInvalid
// di metahub-api. Kontrak lintas-modul: 401 dengan kode ini berarti JWT-nya SAH tapi
// device-nya yang ditolak — bukan sesi yang berakhir.
const codeDeviceSignatureInvalid = "DEVICE_SIGNATURE_INVALID"

// handleSyncUnauthorized memutuskan reaksi terhadap 401 pada jalur sync bertanda
// tangan, dari body responsnya. Ada DUA jenis 401 yang WAJIB dibedakan:
//
//   - Sesi berakhir (token tidak sah/kedaluwarsa; kode UNAUTHORIZED): logout benar.
//   - Device-signature ditolak (kode DEVICE_SIGNATURE_INVALID): JWT sah, device-nya
//     yang bermasalah (tak dikenal / milik user lain — mis. akun berganti di mesin
//     yang device-nya sudah diklaim user sebelumnya). Logout di sini SALAH: ia
//     menghancurkan sesi yang sah dan memicu loop login-ulang tiap 5 detik.
//     Sebagai gantinya, buang device_id lokal supaya tick berikutnya mengirim
//     UNSIGNED (diterima server di Fase A) alih-alih menandatangani sebagai user
//     yang bukan pemilik device. Registrasi ulang untuk user saat ini terjadi di
//     login berikutnya (EnsureDeviceRegistered).
func handleSyncUnauthorized(respBody []byte) error {
	var env struct {
		Error *CloudError `json:"error"`
	}
	_ = json.Unmarshal(respBody, &env)
	if env.Error != nil && env.Error.Code == codeDeviceSignatureInvalid {
		SaveSetting(deviceIDSettingKey, "")
		return fmt.Errorf("device tidak dikenali server; sync akan dikirim tanpa tanda tangan")
	}
	LogoutCloud()
	return fmt.Errorf("session expired, please login again")
}

// No LoginToCloud: /auth/login no longer exists on the server. The Google
// desktop handoff (google_login.go) is the only way in.

// applyLoginResponse persists a successful login's token + profile, registers
// this device, and refreshes entitlement. Used by the Google desktop handoff
// (google_login.go).
func applyLoginResponse(loginResp LoginResponse) {
	if err := saveMetahubJWT(loginResp.Data.Token); err != nil {
		log.Printf("applyLoginResponse: gagal menyegel token (%v)", err)
	}
	SaveSetting("metahub_username", loginResp.Data.User.Username)
	SaveSetting("metahub_email", loginResp.Data.User.Email)
	SaveSetting("metahub_display_name", loginResp.Data.User.DisplayName)
	SaveSetting("metahub_tier", loginResp.Data.User.Tier)

	// Daftarkan identitas device ini sekarang, selagi token-nya sah. Best-effort:
	// kegagalannya tidak menggagalkan login (lihat EnsureDeviceRegistered).
	EnsureDeviceRegistered()

	// Hak akses user yang BARU login — jangan biarkan cache milik user sebelumnya
	// menentukan apa yang boleh dilakukan user ini. Dibersihkan dulu, lalu diisi
	// dari server. Bila server tak terjangkau, cache tetap kosong = tidak berhak,
	// yang benar: kita belum punya bukti apa pun bahwa user ini membayar.
	ClearEntitlement()
	if err := RefreshEntitlement(); err != nil {
		log.Printf("applyLoginResponse: gagal memuat hak akses (%v)", err)
	}
}

// CloudProfile holds user info
type CloudProfile struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Tier        string `json:"tier"`
	IsConnected bool   `json:"is_connected"`
}

// GetCloudProfile returns the currently logged in user's profile
func GetCloudProfile() CloudProfile {
	return CloudProfile{
		Username:    GetSetting("metahub_username"),
		Email:       GetSetting("metahub_email"),
		DisplayName: GetSetting("metahub_display_name"),
		Tier:        GetSetting("metahub_tier"),
		IsConnected: IsCloudConnected(),
	}
}

// IsCloudConnected checks if the user is currently logged in to MetaHub Cloud
func IsCloudConnected() bool {
	token := loadMetahubJWT()
	return token != ""
}

// LogoutCloud clears the JWT token and profile data.
//
// Key device dan device_id SENGAJA dipertahankan: itu identitas MESIN, bukan
// identitas sesi. Menghapusnya melahirkan device baru tiap login dan merusak
// jejak audit.
func LogoutCloud() error {
	saveMetahubJWT("")
	SaveSetting("metahub_username", "")
	SaveSetting("metahub_email", "")
	SaveSetting("metahub_display_name", "")
	SaveSetting("metahub_tier", "")
	// Hak akses adalah milik user, bukan milik mesin: user berikutnya yang login
	// di komputer ini tidak boleh mewarisi paket milik user sebelumnya.
	ClearEntitlement()
	return nil
}

// SyncToCloud pushes ALL local MT5 trades to the MetaHub Cloud API securely.
// MANUAL only (the "Sync to Cloud" button): a 3-day MT5 refresh keeps the click
// snappy while still catching trades closed since the last full sync, then the
// entire local history is pushed so the leaderboard/stats are fully rebuilt.
func SyncToCloud() (string, error) {
	token := loadMetahubJWT()
	if token == "" {
		return "", fmt.Errorf("not logged in. Please connect to MetaHub Cloud first")
	}

	if err := syncTargetGuardForManual(); err != nil {
		return "", err
	}

	// Kegagalan refresh MT5 tidak fatal (MT5 mungkin tertutup) tapi TIDAK ditelan
	// diam-diam: tetap kirim data lokal yang ada, sambil mencatat penyebabnya.
	if res, err := SyncHistoryFromMT5(); err != nil {
		log.Printf("SyncToCloud: lewati refresh MT5 (%v)", err)
	} else {
		log.Printf("SyncToCloud: refresh lokal %d trade sejak %s", res.Trades, res.Since.Format("2006-01-02"))
	}

	// Klik manual berarti "coba lagi sungguhan". Trade yang tertahan jendela
	// dibuka supaya percobaan ini benar-benar mencakup semuanya — kalau
	// jendelanya memang masih sempit, server akan menolaknya lagi dan
	// menandainya blocked kembali. Idempoten, dan ini SATU-SATUNYA jalan bagi
	// user yang mematikan toggle backfill otomatis. Lantai lama ikut dibuang
	// supaya penyaring lokal tidak memblokir ulang sebelum server sempat lapor.
	if err := ClearCloudBlocked(); err != nil {
		log.Printf("SyncToCloud: gagal membuka trade yang tertahan (%v)", err)
	}
	SaveSyncFloor(nil)

	// HANYA yang belum tersimpan di server. Sebelumnya seluruh riwayat diunggah
	// ulang tiap klik — beban request tumbuh seiring umur akun, padahal server
	// membuang hampir semuanya sebagai duplikat.
	trades, err := GetTradesPendingCloudSync()
	if err != nil {
		return "", fmt.Errorf("failed to read local trades: %v", err)
	}
	return pushClosedTradesToCloud(token, trades)
}

// SyncRecentClosedToCloud pushes only TODAY's closed trades to the cloud so that
// closed-trade posts appear in the social feed automatically — without waiting for
// the manual full-history "Sync to Cloud" button. This is called on a throttled
// tick by the desktop auto-loop. Aman dipanggil berulang: payload kecil (hari ini
// saja) dan backend idempoten (ON CONFLICT) + hanya broadcast ticket yang benar-
// benar baru, jadi feed tidak akan spam meski dikirim tiap beberapa detik.
func SyncRecentClosedToCloud() (string, error) {
	token := loadMetahubJWT()
	if token == "" {
		return "", fmt.Errorf("not logged in")
	}

	// Refresh jendela 1 hari dari MT5 agar posisi yang baru saja ditutup langsung
	// masuk SQLite lokal. Best-effort: bila MT5 tertutup, tetap kirim data lokal.
	if err := refreshLocalFromMT5Fn(1); err != nil {
		log.Printf("SyncRecentClosedToCloud: lewati refresh MT5 (%v)", err)
	}

	// Jendela "hari ini" diganti "yang belum tersinkron": lebih kecil dalam
	// keadaan normal (biasanya nol), dan tidak lagi menelantarkan trade yang
	// gagal terkirim kemarin — dulu trade itu tak pernah dicoba lagi sampai
	// user menekan sync manual.
	trades, err := GetTradesPendingCloudSync()
	if err != nil {
		return "", fmt.Errorf("failed to read local trades: %v", err)
	}
	return pushClosedTradesToCloud(token, trades)
}

// PushPendingClosedToCloud mendorong trade tertutup yang belum tersinkron TANPA
// menyentuh MT5 lebih dulu. Dipakai saat startup: catch-up riwayat sudah mengisi
// SQLite, yang kurang hanya dorongannya.
//
// Ini menutup celah "trade tertutup saat aplikasi mati": dulu hanya
// SyncRecentClosedToCloud yang memanggil GetTradesPendingCloudSync, dan itu cuma
// jalan ketika loop 5 detik mendeteksi close — jadi trade dari sesi sebelumnya
// menganggur di jurnal lokal sampai kebetulan ada posisi lain ditutup.
//
// Aman diulang: cloud_synced_at membuat kiriman sebanding dengan data BARU, dan
// server men-dedupe lewat hash di atas itu. Tidak ada dedupe tambahan di sini.
//
// Gerbangnya harus sama dengan loop — entitlement DAN akun target — supaya
// startup tidak jadi jalan belakang yang mendorong akun MT5 yang salah.
// Ketiadaan hak akses bukan error: aplikasi tetap sah berjalan tanpa cloud.
func PushPendingClosedToCloud() (string, error) {
	token := loadMetahubJWT()
	if token == "" {
		return "", nil
	}
	if !CanSyncToCloud() || !syncTargetPushAllowed() {
		return "", nil
	}

	trades, err := GetTradesPendingCloudSync()
	if err != nil {
		return "", fmt.Errorf("failed to read local trades: %v", err)
	}
	return pushClosedTradesToCloud(token, trades)
}

// pushClosedTradesToCloud memformat trade lokal (hash + tipe akun) lalu mem-POST-nya
// ke /trades/sync. Dipakai bersama oleh sync manual (full history) dan sync otomatis
// (recent closed) agar hashing/payload/penanganan respons tinggal di satu tempat.
func pushClosedTradesToCloud(token string, trades []Trade) (string, error) {
	if len(trades) == 0 {
		return "No trades to sync", nil
	}

	// Saring dari lantai yang sudah dipelajari. Ini efisiensi, BUKAN keamanan:
	// server tetap memeriksa ulang setiap trade. Yang tertahan di sini ditandai
	// blocked sekarang juga — kalau tidak, ia akan terbaca sebagai "pending"
	// lagi pada tick berikutnya dan berputar selamanya tanpa pernah terkirim.
	preBlockedCount := 0
	if sendable, preBlocked := partitionByFloor(trades, CachedSyncFloor()); len(preBlocked) > 0 {
		if err := MarkTradesBlocked(preBlocked); err != nil {
			log.Printf("pushClosedTradesToCloud: gagal menandai %d trade blocked lokal: %v", len(preBlocked), err)
		}
		preBlockedCount = len(preBlocked)
		trades = sendable
		if len(trades) == 0 {
			return fmt.Sprintf("%d transaksi lebih lama dari batas paket Anda tidak ikut tersinkron — tingkatkan paket untuk memasukkannya.", preBlockedCount), nil
		}
	}

	var syncItems []TradeSyncItem
	for _, t := range trades {
		// Ticket = deal ticket unik per (partial) close, sehingga penutupan
		// bertahap pada satu posisi tidak saling menimpa di backend.
		// PositionID dipakai backend hanya untuk mencocokkan & menutup feed Open Position.
		dealTicket := t.Ticket
		if dealTicket == "" {
			dealTicket = t.PositionID // Fallback ekstrem (harusnya tidak terjadi)
		}

		// Tidak ada hash client. Hash yang dihitung client dari data yang dikirim
		// client tidak membuktikan apa pun — rumusnya ada di source code. Keaslian
		// sekarang datang dari TANDA TANGAN DEVICE atas seluruh request (signRequest).

		// Kirim tipe akun yang sebenarnya (Real/Demo) sesuai deteksi dari MT5.
		// Backend memakai nilai ini untuk mengunci profil & mencegah campur Real/Demo,
		// dan frontend hanya mengenali casing kapital "Real"/"Demo". Nilai selain itu
		// (mis. "Unknown"/kosong) dikosongkan agar tidak salah mengunci profil user.
		acctType := t.AccountType
		if acctType != "Real" && acctType != "Demo" {
			acctType = ""
		}

		item := TradeSyncItem{
			Ticket:      dealTicket,
			PositionID:  t.PositionID,
			Symbol:      t.Symbol,
			Type:        t.Type,
			Volume:      t.Volume,
			OpenPrice:   t.OpenPrice,
			ClosePrice:  t.ClosePrice,
			OpenTime:    t.OpenTime,
			CloseTime:   t.CloseTime,
			SL:          t.SL,
			TP:          t.TP,
			Commission:  t.Commission,
			Swap:        t.Swap,
			Profit:      t.Profit,
			NetProfit:   t.NetProfit,
			AccountType: acctType,
		}
		syncItems = append(syncItems, item)
	}

	currency := GetSetting("currency")
	if currency == "" {
		currency = "USD"
	}

	// Modal akun (hasil rekonstruksi dari balance-deal MT5) — basis persentase
	// max drawdown di sisi cloud. Bila belum pernah dihitung, rekonstruksi sekali
	// di sini (best-effort). Kegagalan tidak menghentikan sync trade: server akan
	// memakai modal tersimpan sebelumnya.
	if GetAccountBalance() == "" {
		if _, err := RefreshAccountModal(); err != nil {
			log.Printf("pushClosedTradesToCloud: gagal rekonstruksi modal akun (%v)", err)
		}
	}
	balance, _ := strconv.ParseFloat(GetAccountBalance(), 64)

	// Kirim per potongan. Riwayat penuh akun lama bisa puluhan ribu trade, dan
	// satu POST raksasa berisiko timeout, 413, atau ledakan memori di kedua sisi.
	// Membatasi UKURAN payload itu sah; membatasi RENTANG TANGGAL tidak, karena
	// yang terakhir menentukan data mana yang boleh ada. Backend idempoten
	// (ON CONFLICT), jadi potongan aman dikirim ulang bila ada yang gagal.
	var totalSynced, totalSkipped int
	totalOutOfWindow := preBlockedCount
	total := len(syncItems)
	emitSyncProgress(SyncProgress{Sent: 0, Total: total, Message: fmt.Sprintf("Menyiapkan %d transaksi…", total)})

	for start := 0; start < total; start += tradeSyncChunkSize {
		end := start + tradeSyncChunkSize
		if end > total {
			end = total
		}
		chunk := syncItems[start:end]

		// Jeda antar potongan menjaga laju tetap di bawah rate limit server
		// (100 request/menit). Tanpa ini, import riwayat penuh akan menembak
		// batasnya sendiri dan memaksa retry yang sebenarnya bisa dihindari.
		if start > 0 {
			time.Sleep(chunkPacingDelay)
		}

		synced, skipped, blockedTickets, err := postChunkWithRetry(token, chunk, currency, balance, start, total)
		if err != nil {
			// Sebutkan kemajuan yang sudah tercapai: potongan yang lebih dulu
			// sukses sudah ditandai dan tidak akan terkirim ulang.
			if start > 0 {
				return "", fmt.Errorf("%v (%d trade lebih dulu tersinkron)", err, start)
			}
			return "", err
		}

		// Ticket yang DITOLAK jendela ditandai terpisah. Menandainya "tersinkron"
		// bersama yang lain akan membuatnya tidak pernah dikirim ulang — dan
		// backfill setelah user upgrade tidak akan menemukan apa pun untuk
		// dikirim. Server LAMA mengirim daftar kosong, jadi jalur ini no-op di
		// sana dan perilakunya identik dengan sebelum fitur ini ada.
		blocked := make(map[string]bool, len(blockedTickets))
		for _, tk := range blockedTickets {
			blocked[tk] = true
		}
		if len(blockedTickets) > 0 {
			if err := MarkTradesBlocked(blockedTickets); err != nil {
				log.Printf("pushClosedTradesToCloud: gagal menandai %d trade blocked: %v", len(blockedTickets), err)
			}
			totalOutOfWindow += len(blockedTickets)
		}

		// Tandai SETELAH server mengonfirmasi. Menandai lebih awal akan membuat
		// trade yang gagal terkirim dianggap selesai dan hilang dari cloud diam-diam.
		tickets := make([]string, 0, len(chunk))
		for _, it := range chunk {
			if blocked[it.Ticket] {
				continue
			}
			tickets = append(tickets, it.Ticket)
		}
		if err := MarkTradesSynced(tickets); err != nil {
			// Tidak fatal: akibat terburuknya potongan ini terkirim lagi nanti,
			// dan server idempoten. Tetap dicatat, tidak ditelan diam-diam.
			log.Printf("pushClosedTradesToCloud: gagal menandai %d trade tersinkron: %v", len(tickets), err)
		}

		totalSynced += synced
		totalSkipped += skipped

		emitSyncProgress(SyncProgress{
			Sent:    end,
			Total:   total,
			Message: fmt.Sprintf("Mengirim ke cloud… %d/%d transaksi", end, total),
		})
	}

	msg := fmt.Sprintf("Successfully synced %d new trades (%d duplicates skipped). Leaderboard updated!", totalSynced, totalSkipped)
	if totalOutOfWindow > 0 {
		// Penolakan jendela TIDAK boleh diam: dari sisi user, "12 tersinkron"
		// pada 500 trade tanpa penjelasan tidak bisa dibedakan dari aplikasi rusak.
		msg += fmt.Sprintf(" %d transaksi lebih lama dari batas paket Anda tidak ikut tersinkron — tingkatkan paket untuk memasukkannya.", totalOutOfWindow)
	}
	return msg, nil
}

// Parameter laju dorongan ke cloud. Variabel (bukan konstanta) supaya test bisa
// memperkecilnya dan tidak benar-benar menunggu detik.
var (
	// tradeSyncChunkSize adalah jumlah trade per request ke /trades/sync.
	tradeSyncChunkSize = 500
	// chunkPacingDelay menahan laju di bawah 100 request/menit milik server.
	chunkPacingDelay = 700 * time.Millisecond
	// chunkRetryBaseDelay adalah dasar backoff eksponensial saat ditolak.
	chunkRetryBaseDelay = 2 * time.Second
)

// chunkRetryAttempts membatasi percobaan per potongan. Harus terbatas: sync yang
// mengulang selamanya menggantung aplikasi tanpa pernah melapor.
const chunkRetryAttempts = 4

// retryableError menandai kegagalan yang layak dicoba ulang — 429 dari rate
// limiter, gangguan jaringan sesaat, atau error sisi server. Kegagalan lain
// (401, penolakan validasi) tidak akan membaik dengan diulang.
type retryableError struct {
	err   error
	after time.Duration
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

// postChunkWithRetry mengirim satu potongan, mencoba ulang dengan backoff
// eksponensial selama kegagalannya bersifat sementara.
func postChunkWithRetry(token string, chunk []TradeSyncItem, currency string, balance float64, sent, total int) (int, int, []string, error) {
	var lastErr error
	for attempt := 1; attempt <= chunkRetryAttempts; attempt++ {
		synced, skipped, blocked, err := postTradeChunk(token, chunk, currency, balance)
		if err == nil {
			return synced, skipped, blocked, nil
		}
		lastErr = err

		var retryable *retryableError
		if !errors.As(err, &retryable) {
			return 0, 0, nil, err // permanen: jangan buang waktu mengulang
		}
		if attempt == chunkRetryAttempts {
			break
		}

		// Hormati Retry-After bila server menyebutkannya; kalau tidak, mundur
		// eksponensial (2s, 4s, 8s) supaya tidak memperparah kemacetan.
		wait := retryable.after
		if wait <= 0 {
			wait = chunkRetryBaseDelay * time.Duration(1<<uint(attempt-1))
		}
		emitSyncProgress(SyncProgress{
			Sent:  sent,
			Total: total,
			Retry: true,
			Message: fmt.Sprintf("Server sedang sibuk, mencoba lagi dalam %s… (%d/%d)",
				wait.Round(time.Second), attempt, chunkRetryAttempts),
		})
		log.Printf("push chunk: percobaan %d/%d gagal (%v), menunggu %s", attempt, chunkRetryAttempts, err, wait)
		time.Sleep(wait)
	}
	return 0, 0, nil, fmt.Errorf("server menolak terlalu banyak permintaan berturut-turut setelah %d percobaan: %v", chunkRetryAttempts, lastErr)
}

// syncAccountType membaca tipe akun yang tersimpan (ditulis berbarengan dengan
// mt5_account saat terhubung ke MT5) dan menormalkannya ke "Real"/"Demo".
// Nilai lain dikosongkan: bagi server kosong berarti "tidak tahu" dan TIDAK
// akan menimpa tipe yang sudah benar — menuduh akun sebagai Demo lebih buruk
// daripada diam.
func syncAccountType() string {
	t := GetSetting("account_type")
	if t != "Real" && t != "Demo" {
		return ""
	}
	return t
}

// postTradeChunk mengirim SATU potongan trade dan mengembalikan (synced,
// skipped, ticket-yang-ditolak-karena-di-luar-jendela).
func postTradeChunk(token string, items []TradeSyncItem, currency string, balance float64) (int, int, []string, error) {
	payload := map[string]interface{}{
		"mt5_account":  GetSetting("mt5_account"),
		"mt5_server":   GetSetting("mt5_server"),
		"account_type": syncAccountType(),
		"currency":     currency,
		"balance":      balance,
		"trades":       items,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", CloudAPIURL+"/trades/sync", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	signSyncRequest(req, body)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Gangguan jaringan sesaat layak dicoba ulang.
		return 0, 0, nil, &retryableError{err: fmt.Errorf("failed to connect to server: %v", err)}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return 0, 0, nil, handleSyncUnauthorized(respBody)
	}

	// Rate limiter server: tunggu sesuai Retry-After bila disebutkan.
	if resp.StatusCode == http.StatusTooManyRequests {
		var after time.Duration
		if v, convErr := strconv.Atoi(resp.Header.Get("Retry-After")); convErr == nil && v > 0 {
			after = time.Duration(v) * time.Second
		}
		return 0, 0, nil, &retryableError{
			err:   fmt.Errorf("server membatasi laju permintaan (429)"),
			after: after,
		}
	}

	// Error sisi server biasanya sementara.
	if resp.StatusCode >= 500 {
		return 0, 0, nil, &retryableError{err: fmt.Errorf("server sedang bermasalah (status %d)", resp.StatusCode)}
	}

	var syncResp SyncResponse
	if err := json.Unmarshal(respBody, &syncResp); err != nil {
		return 0, 0, nil, fmt.Errorf("respons server tidak dikenali (status %d)", resp.StatusCode)
	}

	if !syncResp.Success || resp.StatusCode != http.StatusOK {
		return 0, 0, nil, syncRejection("sync ditolak", syncResp.Error, resp.StatusCode)
	}

	// Lantai yang dilaporkan server adalah milik akun yang BARUSAN dipakai
	// request ini. Disimpan supaya potongan berikutnya — dan sesi berikutnya —
	// bisa menyaring sebelum kirim, alih-alih membuang puluhan potongan untuk
	// ditolak semua.
	SaveSyncFloor(syncResp.Data.SyncFloorAt)

	return syncResp.Data.Synced, syncResp.Data.Skipped, syncResp.Data.OutOfWindowTickets, nil
}

// OpenPositionSyncItem matches the metahub-api OpenPositionSyncItem payload
type OpenPositionSyncItem struct {
	Ticket       string  `json:"ticket"`
	Symbol       string  `json:"symbol"`
	Type         string  `json:"type"`
	Volume       float64 `json:"volume"`
	OpenPrice    float64 `json:"open_price"`
	CurrentPrice float64 `json:"current_price"`
	SL           float64 `json:"sl"`
	TP           float64 `json:"tp"`
	FloatingPnL  float64 `json:"floating_pnl"`
	AccountType  string  `json:"account_type"`
}

// openVolumeSnapshot menyimpan {ticket: volume} dari tick terakhir, dipakai
// AutoSyncTick untuk mendeteksi penutupan (full/partial close) tanpa harus
// menembak MT5 history tiap tick. Hidup selama app berjalan; reset saat restart.
// Celah "close saat app tertutup" sudah ditutup oleh startup catch-up (30 hari)
// di app.go, jadi snapshot boleh mulai kosong.
var (
	openVolumeSnapshot   = make(map[string]float64)
	openVolumeSnapshotMu sync.Mutex
)

// Seam MT5. Semuanya variabel supaya test bisa menjalankan AutoSyncTick tanpa
// terminal MT5 hidup — pola yang sama dengan CloudAPIURL.
var (
	fetchOpenPositionsFn  = FetchOpenPositions
	refreshLocalFromMT5Fn = refreshLocalFromMT5
	countDealsBetweenFn   = CountDealsBetween
)

// --- Deal probe ---
//
// Snapshot posisi terbuka adalah STATE; penutupan posisi adalah EVENT.
// Menyimpulkan event dari selisih dua sampling state pasti kehilangan event yang
// lebih pendek dari interval sampling: posisi yang dibuka DAN ditutup di antara
// dua tick tidak pernah masuk snapshot, sehingga diffOpenVolumes buta terhadapnya
// dan trade itu tak pernah didorong ke cloud sampai ada close berikutnya. Itu
// tidak bisa diperbaiki dengan mempercepat timer, hanya diperkecil.
//
// Probe ini membaca event dari sumbernya: jumlah deal di riwayat MT5. Naiknya
// hitungan berarti ADA deal baru, berapa pun umur posisinya. Yang dipanggil
// adalah HistoryDealsTotal — RPC yang hanya mengembalikan angka, tanpa payload
// deal — jadi cukup murah untuk tiap tick.

// dealProbeLookback adalah lebar awal jendela probe. Longgar dengan sengaja:
// jam server broker bisa berbeda jauh dari jam lokal (alasan yang sama di balik
// syncOverlap), dan jendela yang terlalu sempit akan diam selamanya.
const dealProbeLookback = 48 * time.Hour

// dealProbeLookahead memberi margin ke depan supaya broker yang jamnya
// MENDAHULUI jam lokal tidak membuat deal terbaru jatuh di luar batas atas.
const dealProbeLookahead = 48 * time.Hour

// maxDealProbeWindow membatasi pertumbuhan jendela pada sesi yang hidup
// berhari-hari; setelah itu jendela dianker ulang.
const maxDealProbeWindow = 7 * 24 * time.Hour

// dealProbe melacak jumlah deal pada jendela ber-ANKER TETAP. Ankernya tidak
// ikut bergeser tiap tick karena jendela geser bisa kehilangan deal: satu deal
// lama keluar jendela pada saat satu deal baru masuk membuat hitungan tak
// berubah — persis kebutaan yang sedang kita perbaiki. Anker tetap membuat
// hitungan monoton naik, sehingga setiap kenaikan pasti berarti deal baru.
type dealProbe struct {
	anchor time.Time
	count  int
	valid  bool // false = belum punya hitungan pembanding untuk anker ini
}

// window mengembalikan rentang yang harus ditanyakan ke MT5 pada tick ini, dan
// menganker ulang bila perlu. Penganker-an ulang membatalkan hitungan lama:
// angka dari dua jendela berbeda tidak sebanding.
func (p *dealProbe) window(now time.Time) (from, to time.Time) {
	if p.anchor.IsZero() || now.Sub(p.anchor) > maxDealProbeWindow {
		p.anchor = now.Add(-dealProbeLookback)
		p.valid = false
	}
	return p.anchor, now.Add(dealProbeLookahead)
}

// observe mencatat hitungan baru dan menjawab apakah ada deal baru sejak
// pengamatan terakhir. Hitungan PERTAMA pada sebuah anker hanyalah baseline —
// bukan bukti apa pun — dan hitungan yang MENURUN (broker membersihkan riwayat
// lama) hanya menurunkan dasar pembanding, tidak memicu sync.
func (p *dealProbe) observe(count int) bool {
	if !p.valid {
		p.valid, p.count = true, count
		return false
	}
	if count == p.count {
		return false
	}
	newDeals := count > p.count
	p.count = count
	return newDeals
}

var (
	activeDealProbe   dealProbe
	activeDealProbeMu sync.Mutex
)

// resetDealProbe mengosongkan state probe. Dipakai test agar tidak saling bocor.
func resetDealProbe() {
	activeDealProbeMu.Lock()
	activeDealProbe = dealProbe{}
	activeDealProbeMu.Unlock()
}

// newDealsSinceLastTick menanyakan MT5 apakah ada deal baru. Kegagalan (MT5
// tertutup) menjawab false TANPA menyentuh state: baseline tidak boleh dikarang
// dari kegagalan, karena angka sungguhan berikutnya akan terbaca sebagai lonjakan.
func newDealsSinceLastTick() bool {
	activeDealProbeMu.Lock()
	defer activeDealProbeMu.Unlock()

	from, to := activeDealProbe.window(time.Now())
	count, err := countDealsBetweenFn(from, to)
	if err != nil {
		log.Printf("AutoSyncTick: probe deal MT5 gagal (%v)", err)
		return false
	}
	return activeDealProbe.observe(count)
}

// refreshLocalFromMT5 menarik riwayat N hari dari MT5 ke SQLite lokal. Inilah
// bagian yang TIDAK dijual: berjalan apa pun paket langganan user, karena jurnal
// lokal + MT5 adalah fitur gratis. Yang berbayar hanya dorongan ke cloud.
func refreshLocalFromMT5(days int) error {
	trades, _, err := FetchHistoryFromMT5(days)
	if err != nil {
		return err
	}
	return SaveTrades(trades)
}

// --- Jendela riwayat MT5 ---
//
// Jendela sync bukan konstanta melainkan turunan dari state. Konstanta (dulu 30
// hari) mencampur dua hal yang berbeda: batas BEBAN (loop 5 detik tak boleh
// menarik riwayat bertahun) dan batas KEBENARAN (data mana yang boleh ada).
// Yang pertama sah; yang kedua adalah kehilangan data diam-diam — akun dengan
// trade terakhir 45 hari lalu tampak kosong dan aplikasi terlihat rusak.

// mt5HistoryEpoch adalah batas bawah "seluruh riwayat", sama dengan yang sudah
// dipakai RefreshAccountModal: akun bisa dibuka bertahun lalu.
var mt5HistoryEpoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// syncOverlap menarik mundur titik awal dari watermark. Menutup dua celah:
// jam server broker yang berbeda dari UTC (gomt5.FromTime mengirim Unix UTC
// sementara filter history MT5 berbasis jam broker), dan deal yang swap/profit-
// nya baru difinalisasi setelah tercatat. Menarik ulang aman: SaveTrades upsert.
const syncOverlap = 48 * time.Hour

const lastHistorySyncKey = "last_mt5_history_sync"

// Seam MT5, sepola dengan fetchOpenPositionsFn.
var fetchHistoryBetweenFn = FetchHistoryFromMT5Between

// HistorySyncResult melaporkan APA yang dicakup sebuah sync, bukan hanya
// hasilnya. "0 transaksi" tanpa jendelanya tidak bisa dibedakan antara "tidak
// ada yang baru" dan "ada, tapi di luar jangkauan saya" — dan user berhak tahu
// yang mana.
type HistorySyncResult struct {
	Trades      int
	Since       time.Time
	FullImport  bool
	AccountType string
}

// historySyncWindow menentukan titik awal penarikan dari watermark. Watermark
// kosong ATAU rusak sama-sama berarti import penuh: gagal ke arah menarik
// terlalu banyak (idempoten, cuma lambat) alih-alih terlalu sedikit (data hilang).
func historySyncWindow(watermark string, now time.Time) (from time.Time, fullImport bool) {
	if watermark == "" {
		return mt5HistoryEpoch, true
	}
	ts, err := time.Parse(time.RFC3339, watermark)
	if err != nil {
		log.Printf("watermark sync rusak (%q), menarik riwayat penuh", watermark)
		return mt5HistoryEpoch, true
	}
	return ts.Add(-syncOverlap), false
}

// SyncHistoryFromMT5 menarik riwayat MT5 ke SQLite lokal sejak sync sukses
// terakhir — atau SELURUH riwayat bila belum pernah. Watermark hanya maju
// setelah fetch DAN simpan sukses, sehingga app yang tertutup berminggu-minggu
// (atau MT5 yang mati saat sync) tetap menyusul sendiri di kesempatan berikutnya.
func SyncHistoryFromMT5() (HistorySyncResult, error) {
	now := time.Now()
	from, fullImport := historySyncWindow(GetSetting(lastHistorySyncKey), now)
	res := HistorySyncResult{Since: from, FullImport: fullImport}

	trades, accountType, err := fetchHistoryBetweenFn(from, now)
	if err != nil {
		return res, err
	}
	res.AccountType = accountType
	if err := SaveTrades(trades); err != nil {
		return res, err
	}

	SaveSetting(lastHistorySyncKey, now.Format(time.RFC3339))
	res.Trades = len(trades)
	return res, nil
}

// AutoSyncTick adalah SATU entrypoint yang dipanggil frontend tiap 5 detik.
// Semua logika kecerdasan (kapan sync closed) ada di Go; frontend cukup timer bodoh.
//
// Alur:
//  1. Belum login → no-op.
//  2. Ambil posisi terbuka dari MT5 SEKALI. Bila error (MT5 tertutup), skip tick —
//     jangan pernah menganggap "semua posisi tertutup" (itu akan memicu sync palsu).
//  3. Push posisi terbuka ke cloud (feed live realtime) memakai posisi yang sama —
//     tidak ada fetch MT5 kedua.
//  4. Deteksi close dengan membandingkan volume vs snapshot sebelumnya.
//  5. Perbarui snapshot.
//  6. Bila diff tidak menemukan apa-apa, tanya riwayat deal MT5 (deal probe) —
//     inilah yang menangkap posisi yang dibuka DAN ditutup di antara dua tick,
//     yang tak akan pernah terlihat oleh diff snapshot.
//  7. Bila ada close terdeteksi → SyncRecentClosedToCloud (kirim yang belum
//     tersinkron, idempoten).
//
// SyncRecentClosedToCloud hanya dipanggil saat ada yang benar-benar tertutup, jadi
// beban closed-sync ke server = NOL selama tidak ada posisi ditutup.
func AutoSyncTick() error {
	token := loadMetahubJWT()
	if token == "" {
		return nil
	}

	// Hak akses menentukan apakah kita MENDORONG ke cloud — bukan apakah aplikasi
	// bekerja. Semua yang di bawah ini yang menyentuh MT5 dan SQLite lokal berjalan
	// untuk siapa pun; hanya blok push yang dilewati.
	entitled := CanSyncToCloud()

	// Satu fetch MT5 per tick (di-cache oleh FetchOpenPositions).
	positions, err := fetchOpenPositionsFn()
	if err != nil {
		// MT5 tertutup/error: lewati tick tanpa menyentuh snapshot, supaya
		// tick berikutnya tidak salah mendeteksi "hilangnya" posisi sebagai close.
		return fmt.Errorf("failed to fetch open positions: %v", err)
	}

	// Fail-closed account guard. FetchOpenPositions just refreshed
	// mt5_account/mt5_server from the live terminal, so this checks the exact
	// identity the payload will carry. Pushing requires BOTH a paid plan AND
	// that the active account is the one the user chose to sync — otherwise a
	// second open terminal (or an account switch) would silently sync the wrong
	// account. Local-journal updates below stay unconditional.
	pushAllowed := entitled && syncTargetPushAllowed()

	if pushAllowed {
		// Push posisi terbuka (feed live) — pakai positions yang sudah diambil.
		if _, err := pushOpenPositions(token, positions); err != nil {
			// Kegagalan push open tidak menghentikan deteksi close; catat saja.
			log.Printf("AutoSyncTick: push open positions gagal (%v)", err)
		}
	}

	// Deteksi close berdasarkan perubahan volume, lalu perbarui snapshot.
	openVolumeSnapshotMu.Lock()
	closeDetected := diffOpenVolumes(positions, openVolumeSnapshot)
	next := make(map[string]float64, len(positions))
	for _, p := range positions {
		next[p.Ticket] = p.Volume
	}
	openVolumeSnapshot = next
	openVolumeSnapshotMu.Unlock()

	// Diff snapshot menangkap close biasa tanpa biaya sama sekali. Yang lolos
	// darinya adalah posisi yang lahir dan mati di antara dua tick — untuk itu
	// tanya riwayat deal MT5. Probe hanya dijalankan bila diff tidak menemukan
	// apa-apa: kalau sudah pasti akan sync, satu RPC lagi cuma pemborosan.
	if !closeDetected {
		closeDetected = newDealsSinceLastTick()
	}

	if !closeDetected {
		return nil
	}

	if !pushAllowed {
		// Tidak berhak / akun bukan target: jurnal lokal TETAP diperbarui dari MT5.
		// Trade yang baru saja ditutup harus tetap muncul di aplikasi — itu bagian
		// gratisnya, dan mismatch hanya menahan dorongan ke cloud, bukan aplikasinya.
		if err := refreshLocalFromMT5Fn(1); err != nil {
			return fmt.Errorf("refresh jurnal lokal gagal: %v", err)
		}
		return nil
	}

	if _, err := SyncRecentClosedToCloud(); err != nil {
		return fmt.Errorf("sync recent closed gagal: %v", err)
	}
	return nil
}

// diffOpenVolumes adalah fungsi murni: mengembalikan true bila ADA ticket di prev
// yang menandakan penutupan pada current, yaitu ticket yang:
//   - HILANG dari current (full close), atau
//   - volumenya MENGECIL di current (partial close): curVol < prevVol - epsilon.
//
// Mengembalikan false untuk: ticket baru muncul (open baru), volume bertambah
// (scale-in), dan tanpa perubahan. Epsilon menjaga perbandingan float.
func diffOpenVolumes(current []OpenPosition, prev map[string]float64) bool {
	const epsilon = 1e-6
	curVol := make(map[string]float64, len(current))
	for _, p := range current {
		curVol[p.Ticket] = p.Volume
	}
	for ticket, prevVol := range prev {
		cur, ok := curVol[ticket]
		if !ok {
			return true // full close: ticket hilang
		}
		if cur < prevVol-epsilon {
			return true // partial close: volume mengecil
		}
	}
	return false
}

// SyncOpenPositionsToCloud pushes live floating positions to the MetaHub Cloud API.
// Tetap publik untuk pemakaian manual/independen; melakukan fetch MT5 sendiri lalu
// mendelegasikan pemformatan + POST ke pushOpenPositions.
func SyncOpenPositionsToCloud() (string, error) {
	token := loadMetahubJWT()
	if token == "" {
		return "", fmt.Errorf("not logged in")
	}

	if err := syncTargetGuardForManual(); err != nil {
		return "", err
	}

	// 1. Fetch live open positions from MT5
	positions, err := FetchOpenPositions()
	if err != nil {
		return "", fmt.Errorf("failed to fetch open positions: %v", err)
	}

	return pushOpenPositions(token, positions)
}

// pushOpenPositions memformat posisi terbuka lalu mem-POST-nya ke /trades/sync-open.
// Dipisah dari fetch MT5 supaya AutoSyncTick bisa memakai kembali posisi yang sudah
// diambil (satu fetch per tick) tanpa menembak MT5 dua kali.
func pushOpenPositions(token string, positions []OpenPosition) (string, error) {
	syncItems := make([]OpenPositionSyncItem, 0)
	for _, p := range positions {
		syncItems = append(syncItems, OpenPositionSyncItem{
			Ticket:       p.Ticket,
			Symbol:       p.Symbol,
			Type:         p.Type,
			Volume:       p.Volume,
			OpenPrice:    p.OpenPrice,
			CurrentPrice: p.CurrentPrice,
			SL:           p.SL,
			TP:           p.TP,
			FloatingPnL:  p.FloatingPnL,
			AccountType:  p.AccountType,
		})
	}

	currency := GetSetting("currency")
	if currency == "" {
		currency = "USD"
	}

	payload := map[string]interface{}{
		"mt5_account":  GetSetting("mt5_account"),
		"mt5_server":   GetSetting("mt5_server"),
		"account_type": syncAccountType(),
		"currency":     currency,
		"positions":    syncItems,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", CloudAPIURL+"/trades/sync-open", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	signSyncRequest(req, body)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to connect to server: %v", err)
	}
	defer resp.Body.Close()

	// Parse envelope agar penolakan server (mis. profil terkunci Real/Demo,
	// validasi payload) tidak ditelan diam-diam pada loop 5 detik.
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return "", handleSyncUnauthorized(respBody)
	}

	var syncResp SyncResponse
	if err := json.Unmarshal(respBody, &syncResp); err != nil {
		return "", fmt.Errorf("respons server tidak dikenali (status %d)", resp.StatusCode)
	}
	if !syncResp.Success || resp.StatusCode != http.StatusOK {
		return "", syncRejection("sync posisi terbuka ditolak", syncResp.Error, resp.StatusCode)
	}

	return fmt.Sprintf("Live positions synced (%d baru, %d dilewati)", syncResp.Data.Synced, syncResp.Data.Skipped), nil
}
