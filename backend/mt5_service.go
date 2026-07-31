package backend

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	gomt5 "github.com/mukbeast4/go-mt5"
)

// tradeModeReal adalah nilai ENUM_ACCOUNT_TRADE_MODE MT5 untuk akun uang riil.
// MT5: 0=Demo, 1=Contest, 2=Real. go-mt5 tidak menyediakan konstanta bernama,
// jadi kita definisikan di sini agar deteksi Real/Demo punya satu sumber acuan.
const tradeModeReal = 2

// accountTypeFromInfo menerjemahkan AccountInfo MT5 menjadi "Real" atau "Demo".
// Contest (1) diperlakukan sebagai "Demo" karena bukan uang sungguhan.
func accountTypeFromInfo(info *gomt5.AccountInfo) string {
	if info == nil {
		return "Unknown"
	}
	if info.TradeMode == tradeModeReal {
		return "Real"
	}
	return "Demo"
}

// MT5AccountInfo adalah ringkasan identitas akun MT5 untuk ditampilkan di UI.
type MT5AccountInfo struct {
	Login       int64   `json:"login"`
	Server      string  `json:"server"`
	Currency    string  `json:"currency"`
	Balance     float64 `json:"balance"`      // modal/equity akun, dipakai untuk hitung max drawdown %
	AccountType string  `json:"account_type"` // "Real" / "Demo"
	Connected   bool    `json:"connected"`
}

// isBalanceDeal menandai deal MT5 yang mengubah saldo TANPA berasal dari hasil
// trading: setoran/penarikan (Balance), koreksi, kredit, dan bonus. Dijumlahkan
// untuk merekonstruksi modal bersih akun (basis persentase max drawdown).
func isBalanceDeal(t gomt5.DealType) bool {
	switch t {
	case gomt5.DealTypeBalance, gomt5.DealTypeCredit, gomt5.DealTypeBonus,
		gomt5.DealTypeCorrection, gomt5.DealTypeCharge:
		return true
	}
	return false
}

// newMT5Client menghubungkan ke terminal MT5 dengan beberapa kali percobaan.
//
// IPC named-pipe milik MT5 kadang belum menjawab MESKI terminal sudah terbuka:
// beberapa detik pertama setelah login, saat "Algo Trading" baru diaktifkan, atau
// ketika koneksi dari tick sebelumnya belum tuntas dilepas. go-mt5 hanya mencoba
// SEKALI (~500ms) lalu menyerah dengan "no responding MT5 pipe (1 candidate)",
// sehingga user terpaksa menutup-buka MT5. Retry berjeda pendek membiarkan kondisi
// transien ini pulih sendiri tanpa intervensi manual. attempts=1 = perilaku lama.
//
// Berhenti lebih awal bila ctx sudah kedaluwarsa, jadi jalur dengan timeout ketat
// (mis. tick 5 detik) tetap aman dan tidak menumpuk.
func newMT5Client(ctx context.Context, attempts int, opts ...gomt5.Option) (*gomt5.Client, error) {
	if attempts < 1 {
		attempts = 1
	}
	const gap = 600 * time.Millisecond

	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(gap):
			}
		}
		client, err := gomt5.NewClient(ctx, opts...)
		if err == nil {
			if i > 0 {
				log.Printf("Koneksi MT5 berhasil pada percobaan ke-%d", i+1)
			}
			return client, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// --- Koneksi MT5 tunggal (shared) -------------------------------------------
//
// Dulu setiap operasi membuka + menutup pipe MT5 sendiri (NewClient/Close per
// panggilan), termasuk tiap 5 detik dari auto-sync. Buka-tutup beruntun itu bisa
// bertabrakan dan membuat pipe "sibuk"/tak menjawab, sehingga user harus
// menutup-buka MT5. Sekarang seluruh aplikasi memakai SATU client yang hidup
// selama app berjalan. go-mt5 menjalankan heartbeat internal yang mendeteksi
// putus koneksi dan MENYAMBUNG ULANG otomatis (auto-reconnect, backoff 1→30s,
// tak terbatas) — jadi walau MT5 sempat ditutup lalu dibuka lagi, koneksi pulih
// sendiri tanpa perlu me-restart MT5 secara manual.
var (
	sharedMT5   *gomt5.Client
	sharedMT5Mu sync.Mutex
)

// mt5DialOpts membangun opsi client bersama: auto-discover pipe, auto-reconnect,
// dan heartbeat 5 detik sebagai pemicu deteksi putus + reconnect.
func mt5DialOpts() []gomt5.Option {
	return []gomt5.Option{
		gomt5.WithPipeName(""),
		gomt5.WithAutoReconnect(true),
		gomt5.WithHeartbeatInterval(5 * time.Second),
	}
}

// getMT5Client mengembalikan koneksi MT5 bersama, membuatnya lazily bila belum
// ada. Bila client sedang menyambung ulang, tunggu sebentar agar operasi tidak
// gagal percuma. Penting: pemanggil TIDAK boleh memanggil Close() pada hasilnya —
// koneksi ini milik bersama dan harus tetap hidup. Gunakan CloseMT5 saat shutdown.
//
// Heartbeat/reconnect dijalankan pada context.Background() agar tidak mati saat
// context per-panggilan (mis. timeout 5 detik) kedaluwarsa.
func getMT5Client(ctx context.Context) (*gomt5.Client, error) {
	sharedMT5Mu.Lock()
	c := sharedMT5
	sharedMT5Mu.Unlock()

	if c != nil {
		if c.Connected() {
			return c, nil
		}
		if c.State() == gomt5.StateReconnecting {
			// Sedang pulih: beri waktu singkat, lalu kembalikan apa adanya
			// (jangan buat client kedua saat reconnect sedang berjalan).
			waitConnected(ctx, c, 2*time.Second)
			return c, nil
		}
	}

	// Belum ada, atau sudah Disconnected permanen: (re)buat dengan double-check.
	sharedMT5Mu.Lock()
	defer sharedMT5Mu.Unlock()
	if sharedMT5 != nil {
		if sharedMT5.Connected() || sharedMT5.State() == gomt5.StateReconnecting {
			return sharedMT5, nil
		}
		sharedMT5.Close()
		sharedMT5 = nil
	}

	client, err := newMT5Client(context.Background(), 3, mt5DialOpts()...)
	if err != nil {
		return nil, err
	}
	sharedMT5 = client
	return sharedMT5, nil
}

// waitConnected menunggu hingga client berstatus Connected atau timeout habis.
func waitConnected(ctx context.Context, c *gomt5.Client, max time.Duration) bool {
	deadline := time.Now().Add(max)
	for {
		if c.Connected() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// CloseMT5 menutup koneksi MT5 bersama (dipanggil saat aplikasi shutdown).
func CloseMT5() {
	sharedMT5Mu.Lock()
	defer sharedMT5Mu.Unlock()
	if sharedMT5 != nil {
		sharedMT5.Close()
		sharedMT5 = nil
	}
}

// RefreshAccountModal menarik SELURUH riwayat deal dari MT5 dan menjumlahkan
// deal non-trading (setoran, penarikan, koreksi, bonus) untuk merekonstruksi
// modal bersih akun = total dana yang benar-benar disetor. Nilai ini dipakai
// sebagai basis persentase max drawdown, bukan saldo berjalan (yang sudah
// tercampur profit/loss). Hasilnya disimpan ke setting "account_balance".
//
// Catatan: memakai rentang tanggal sangat lebar agar setoran pertama (yang bisa
// terjadi bertahun lalu) ikut terhitung — beda dengan FetchHistoryFromMT5 yang
// hanya menarik jendela beberapa hari untuk loop sync ringan.
func RefreshAccountModal() (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := getMT5Client(ctx)
	if err != nil {
		return 0, fmt.Errorf("gagal terhubung ke MT5: %v", err)
	}

	filter := &gomt5.HistoryFilter{
		DateFrom: gomt5.FromTime(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
		DateTo:   gomt5.FromTime(time.Now().Add(24 * time.Hour)),
	}
	deals, err := client.HistoryDealsGet(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("gagal menarik riwayat deal: %v", err)
	}

	var modal float64
	for _, d := range deals {
		if isBalanceDeal(d.Type) {
			modal += d.Profit // deposit = positif, withdrawal = negatif
		}
	}

	if modal <= 0 {
		return 0, fmt.Errorf("modal hasil rekonstruksi tidak valid (%.2f); saldo balance-deal tidak ditemukan", modal)
	}

	SetAccountBalance(strconv.FormatFloat(modal, 'f', 2, 64))
	log.Printf("Modal akun direkonstruksi dari %d deal: %.2f", len(deals), modal)
	return modal, nil
}

// GetMT5AccountInfo membaca info akun langsung dari terminal MT5 untuk
// memastikan akun yang sedang login itu Real atau Demo (dan mata uangnya).
func GetMT5AccountInfo() (MT5AccountInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := getMT5Client(ctx)
	if err != nil {
		return MT5AccountInfo{}, fmt.Errorf("gagal terhubung ke MT5 (pastikan MT5 terbuka): %v", err)
	}

	info, err := client.AccountInfo(ctx)
	if err != nil {
		return MT5AccountInfo{}, fmt.Errorf("gagal membaca info akun MT5: %v", err)
	}

	accountType := accountTypeFromInfo(info)
	if info.Currency != "" {
		SaveSetting("currency", info.Currency)
	}
	SaveSetting("account_type", accountType)
	// Persist (login, server) so the cloud sync can identify which MT5 account this
	// data belongs to (multi-account support).
	SaveSetting("mt5_account", fmt.Sprintf("%d", info.Login))
	SaveSetting("mt5_server", info.Server)

	return MT5AccountInfo{
		Login:       info.Login,
		Server:      info.Server,
		Currency:    info.Currency,
		Balance:     info.Balance,
		AccountType: accountType,
		Connected:   true,
	}, nil
}

// FetchHistoryFromMT5 mengambil riwayat transaksi dari MT5 untuk N hari terakhir.
// Dipakai oleh loop auto-sync yang sengaja berjendela sempit (1-3 hari) demi
// menjaga tick tetap ringan. Untuk penarikan yang benar (import awal / catch-up
// setelah app lama tertutup) pakai FetchHistoryFromMT5Between lewat
// SyncHistoryFromMT5, yang jendelanya diturunkan dari watermark — bukan konstanta.
func FetchHistoryFromMT5(daysBack int) ([]Trade, string, error) {
	dateTo := time.Now()
	return FetchHistoryFromMT5Between(dateTo.AddDate(0, 0, -daysBack), dateTo)
}

// FetchHistoryFromMT5Between mengambil riwayat transaksi dari MT5 dalam rentang
// waktu eksplisit. Rentang lebar (import riwayat penuh) diberi timeout lebih
// longgar karena broker perlu waktu memuat deal bertahun-tahun.
func FetchHistoryFromMT5Between(dateFrom, dateTo time.Time) ([]Trade, string, error) {
	timeout := 25 * time.Second
	if dateTo.Sub(dateFrom) > 400*24*time.Hour {
		timeout = 3 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// Pakai koneksi MT5 bersama (auto-reconnect); tidak ditutup di sini.
	client, err := getMT5Client(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("gagal terhubung ke MT5 (Pastikan MT5 terbuka): %v", err)
	}

	// Filter history untuk periode tersebut
	filter := &gomt5.HistoryFilter{
		DateFrom: gomt5.FromTime(dateFrom),
		DateTo:   gomt5.FromTime(dateTo),
	}

	// Ambil history deals
	deals, err := client.HistoryDealsGet(ctx, filter)
	if err != nil {
		return nil, "", fmt.Errorf("gagal menarik data deals: %v", err)
	}

	// Diagnostik: "0 transaksi" punya beberapa sebab yang sangat berbeda —
	// jendela kesempitan, semua posisi masih terbuka (tak ada deal OUT), atau
	// terminal memang kosong. Tanpa angka mentah ini ketiganya tampak sama.
	var dealsIn, dealsOut int
	for _, d := range deals {
		if d.Entry == gomt5.DealEntryIn {
			dealsIn++
		} else {
			dealsOut++
		}
	}
	log.Printf("MT5 history %s..%s: %d deal mentah (masuk=%d, keluar=%d)",
		dateFrom.Format("2006-01-02"), dateTo.Format("2006-01-02"), len(deals), dealsIn, dealsOut)

	accountType := "Unknown"
	// Ambil Info Akun (untuk deteksi Real/Demo & mata uang)
	accountInfo, err := client.AccountInfo(ctx)
	if err != nil {
		log.Printf("Peringatan: gagal membaca info akun MT5, tipe akun tidak diketahui: %v", err)
	} else {
		accountType = accountTypeFromInfo(accountInfo)
		if accountInfo.Currency != "" {
			SaveSetting("currency", accountInfo.Currency)
		}
		SaveSetting("account_type", accountType)
		SaveSetting("mt5_account", fmt.Sprintf("%d", accountInfo.Login))
		SaveSetting("mt5_server", accountInfo.Server)
		log.Printf("Sinkronisasi berhasil! Akun: %s, Mata Uang: %s", accountType, accountInfo.Currency)
	}

	// Buat map untuk menyimpan Deal IN (Entry) berdasarkan PositionID
	// agar kita bisa mendapatkan OpenPrice dan OpenTime
	entryDeals := make(map[int64]*gomt5.Deal)
	for _, d := range deals {
		if d.Entry == gomt5.DealEntryIn {
			// Simpan referensi ke Deal IN
			entry := d // copy
			entryDeals[d.PositionID] = &entry
		}
	}

	// Identitas akun dibaca SEKALI. GetSetting adalah query SQLite; dulu dipanggil
	// empat kali per deal (dua di konstruksi Trade, dua di lookup SL/TP), sehingga
	// import riwayat penuh puluhan ribu trade berarti ratusan ribu query.
	activeLogin, activeServer := activeAccount()

	// SL/TP juga dimuat SEKALI ke map, bukan satu DB.First per deal di dalam loop.
	// Ini kontras yang dulu janggal: SaveTrades sudah berupa upsert massal,
	// sementara jalur baca di sini masih N+1.
	sltp := make(map[string]SavedOpenPosition)
	var cachedPositions []SavedOpenPosition
	if err := DB.Where("mt5_login = ? AND mt5_server = ?", activeLogin, activeServer).
		Find(&cachedPositions).Error; err != nil {
		log.Printf("Peringatan: gagal memuat cache SL/TP (%v); SL/TP akan kosong", err)
	}
	for _, c := range cachedPositions {
		sltp[c.PositionID] = c
	}

	var trades []Trade
	for _, d := range deals {
		// Abaikan Deal IN (karena kita hanya butuh Deal OUT yang merealisasikan PnL)
		if d.Entry == gomt5.DealEntryIn {
			continue
		}

		// Abaikan jika Volume 0 atau jika ini bukan DealTypeBuy / DealTypeSell
		if d.Volume == 0 || (d.Type != gomt5.DealTypeBuy && d.Type != gomt5.DealTypeSell) {
			continue
		}

		// Terjemahkan tipe transaksi (Dibalik karena ini adalah Deal OUT)
		// Jika Deal OUT adalah Sell, berarti posisinya adalah Buy
		// Jika Deal OUT adalah Buy, berarti posisinya adalah Sell
		tradeTypeStr := "Sell"
		if d.Type == gomt5.DealTypeSell {
			tradeTypeStr = "Buy"
		}

		// Kalkulasi Net Profit: Profit kotor + Commission + Swap
		netProfit := d.Profit + d.Commission + d.Swap
		posIDStr := fmt.Sprintf("%d", d.PositionID)

		// Ambil OpenPrice dan OpenTime dari Deal IN
		var openPrice float64
		openTime := d.TimeUTC().Local() // Default to close time if entry not found

		if entryDeal, exists := entryDeals[d.PositionID]; exists {
			openPrice = entryDeal.Price
			openTime = entryDeal.TimeUTC().Local()
		}

		t := Trade{
			Ticket:      fmt.Sprintf("%d", d.Ticket),
			CloseTime:   d.TimeUTC().Local(),
			OpenTime:    openTime,
			Symbol:      d.Symbol,
			Type:        tradeTypeStr,
			Volume:      d.Volume,
			OpenPrice:   openPrice,
			ClosePrice:  d.Price,
			Commission:  d.Commission,
			Swap:        d.Swap,
			Profit:      d.Profit,
			NetProfit:   netProfit,
			PositionID:  posIDStr,
			AccountType: accountType,
			MT5Login:    activeLogin,
			MT5Server:   activeServer,
		}

		// Cari SL dan TP dari cache — HANYA untuk akun yang sama, supaya posisi
		// milik akun lain dengan position_id kebetulan sama tidak menodai SL/TP.
		// Map-nya sudah disaring per akun saat dimuat di atas.
		if cachedPos, ok := sltp[posIDStr]; ok {
			t.SL = cachedPos.SL
			t.TP = cachedPos.TP
		}

		// Hindari deal non-trading (Balance deposit/withdrawal biasanya Type != Buy/Sell atau Symbol kosong)
		if t.Symbol != "" {
			trades = append(trades, t)
		}
	}

	log.Printf("Berhasil menarik %d transaksi dari MT5\n", len(trades))
	return trades, accountType, nil
}

// CountDealsBetween mengembalikan JUMLAH deal MT5 pada sebuah rentang, tanpa
// menariknya. Ini sengaja bukan len(FetchHistoryFromMT5Between(...)): perintah
// HistoryDealsTotal hanya mengirim balik sebuah angka, sementara menarik deal
// berarti payload penuh plus pemetaan Trade dan lookup SL/TP per posisi ke
// SQLite. Bedanya menentukan, karena fungsi ini dipanggil tiap 5 detik oleh
// deal probe di AutoSyncTick untuk menjawab satu pertanyaan saja: "ada yang baru?"
func CountDealsBetween(dateFrom, dateTo time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Koneksi MT5 bersama (auto-reconnect). Dipanggil tiap 5 detik; jangan ditutup.
	client, err := getMT5Client(ctx)
	if err != nil {
		return 0, fmt.Errorf("gagal terhubung ke MT5: %v", err)
	}

	total, err := client.HistoryDealsTotal(ctx, gomt5.FromTime(dateFrom), gomt5.FromTime(dateTo))
	if err != nil {
		return 0, fmt.Errorf("gagal menghitung deal: %v", err)
	}
	return total, nil
}

// FetchOpenPositions mengambil daftar posisi yang masih terbuka (floating) dari MT5
func FetchOpenPositions() ([]OpenPosition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Koneksi MT5 bersama (auto-reconnect). Dipanggil tiap 5 detik; jangan ditutup.
	client, err := getMT5Client(ctx)
	if err != nil {
		return nil, fmt.Errorf("gagal terhubung ke MT5: %v", err)
	}

	positions, err := client.PositionsGet(ctx, &gomt5.PositionFilter{})
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil posisi terbuka: %v", err)
	}

	accountType := "Unknown"
	accountInfo, err := client.AccountInfo(ctx)
	if err == nil {
		accountType = accountTypeFromInfo(accountInfo)
		SaveSetting("account_type", accountType)
		// Persist the real MT5 login + server on every open-position fetch so the
		// 5s auto-sync always sends the account identity — even if the user never
		// ran a full history sync. Without this the payload's mt5_account/server
		// are empty and the cloud silently falls back to the primary account,
		// which breaks true multi-account resolution.
		SaveSetting("mt5_account", fmt.Sprintf("%d", accountInfo.Login))
		SaveSetting("mt5_server", accountInfo.Server)
		if accountInfo.Currency != "" {
			SaveSetting("currency", accountInfo.Currency)
		}
	}

	var openPositions []OpenPosition
	for _, p := range positions {
		var typeStr string
		if p.Type == gomt5.PositionTypeBuy {
			typeStr = "Buy"
		} else {
			typeStr = "Sell"
		}

		openPositions = append(openPositions, OpenPosition{
			Ticket:       fmt.Sprintf("%d", p.Ticket),
			Symbol:       p.Symbol,
			Type:         typeStr,
			Volume:       p.Volume,
			OpenPrice:    p.PriceOpen,
			CurrentPrice: p.PriceCurrent,
			SL:           p.PriceSL,
			TP:           p.PriceTP,
			FloatingPnL:  p.Profit,
			AccountType:  accountType,
			MT5Login:     GetSetting("mt5_account"),
			MT5Server:    GetSetting("mt5_server"),
		})
	}

	// Simpan ke database lokal sebagai cache
	CacheOpenPositions(openPositions)

	return openPositions, nil
}
