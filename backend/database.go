package backend

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB menginisialisasi koneksi ke SQLite
func InitDB() {
	var err error
	DB, err = gorm.Open(sqlite.Open("journal.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("Gagal terhubung ke database SQLite: %v", err)
	}

	// Auto migrate schema
	err = DB.AutoMigrate(&Trade{}, &Setting{}, &SavedOpenPosition{})
	if err != nil {
		log.Fatalf("Gagal migrasi database: %v", err)
	}

	if err := migrateAccountIdentity(); err != nil {
		log.Fatalf("Gagal migrasi identitas akun: %v", err)
	}

	// Clean up duplicate IN deals (yang terlanjur masuk dengan profit 0 dan commission 0)
	DB.Where("profit = ? AND net_profit = ? AND commission = ? AND swap = ?", 0, 0, 0, 0).Delete(&Trade{})
}

// SaveTrades menyimpan atau memperbarui transaksi dari MT5 ke database
func SaveTrades(trades []Trade) error {
	if len(trades) == 0 {
		return nil
	}

	// Upsert massal: satu pernyataan per batch, bukan SELECT lalu INSERT/UPDATE
	// per trade. Import riwayat penuh puluhan ribu trade dulu berarti puluhan
	// ribu round-trip SQLite (masing-masing transaksinya sendiri); sekarang
	// tinggal beberapa pernyataan di dalam satu transaksi per batch.
	//
	// DO UPDATE hanya berjalan bila isi baris BENAR-BENAR berubah (klausa WHERE
	// di bawah). Dua akibatnya sama pentingnya: baris yang tidak berubah tidak
	// ditulis ulang percuma, dan cloud_synced_at-nya tidak ikut hangus — jadi
	// trade yang sudah pernah didorong ke server tidak dikirim ulang tanpa sebab.
	const changed = "trades.open_time <> excluded.open_time" +
		" OR trades.close_time <> excluded.close_time" +
		" OR trades.symbol <> excluded.symbol" +
		" OR trades.type <> excluded.type" +
		" OR trades.volume <> excluded.volume" +
		" OR trades.open_price <> excluded.open_price" +
		" OR trades.close_price <> excluded.close_price" +
		" OR trades.sl <> excluded.sl" +
		" OR trades.tp <> excluded.tp" +
		" OR trades.commission <> excluded.commission" +
		" OR trades.swap <> excluded.swap" +
		" OR trades.profit <> excluded.profit" +
		" OR trades.net_profit <> excluded.net_profit" +
		" OR trades.position_id <> excluded.position_id" +
		" OR trades.account_type <> excluded.account_type"

	assignments := clause.AssignmentColumns([]string{
		"open_time", "close_time", "symbol", "type", "volume", "open_price",
		"close_price", "sl", "tp", "commission", "swap", "profit", "net_profit",
		"position_id", "account_type",
	})
	// Isi berubah berarti salinan di server sudah basi: tandai perlu dikirim ulang.
	// cloud_blocked_at ikut dikosongkan — trade yang isinya berubah adalah data
	// BARU, dan berhak dicoba lagi apa pun alasan penolakan sebelumnya.
	assignments = append(assignments,
		clause.Assignment{Column: clause.Column{Name: "cloud_synced_at"}, Value: nil},
		clause.Assignment{Column: clause.Column{Name: "cloud_blocked_at"}, Value: nil},
	)

	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "mt5_login"}, {Name: "mt5_server"}, {Name: "ticket"}},
		DoUpdates: assignments,
		Where:     clause.Where{Exprs: []clause.Expression{gorm.Expr(changed)}},
	}).CreateInBatches(&trades, saveTradesBatchSize).Error
}

// saveTradesBatchSize membatasi jumlah baris per pernyataan INSERT agar tidak
// menabrak batas variabel SQLite pada import riwayat penuh.
const saveTradesBatchSize = 200

// GetTradesPendingCloudSync mengembalikan trade yang BELUM berhasil didorong ke
// cloud (atau yang isinya berubah sejak dorongan terakhir). Inilah yang membuat
// sync ke server sebanding dengan jumlah data BARU, bukan dengan besarnya
// seluruh riwayat.
func GetTradesPendingCloudSync() ([]Trade, error) {
	var trades []Trade
	// cloud_blocked_at dikecualikan supaya loop 5 detik tidak menembaki server
	// dengan trade yang sudah pasti ditolak jendelanya. Trade itu tetap tersimpan
	// dan tetap belum tersinkron — ClearCloudBlocked yang mengembalikannya.
	err := DB.Where("cloud_synced_at IS NULL AND cloud_blocked_at IS NULL").
		Order("close_time asc").Find(&trades).Error
	return trades, err
}

// MarkTradesSynced menandai satu potongan trade sebagai sudah tersimpan di
// server — satu UPDATE untuk seluruh potongan, bukan satu per trade.
func MarkTradesSynced(tickets []string) error {
	if len(tickets) == 0 {
		return nil
	}
	return DB.Model(&Trade{}).
		Where("ticket IN ?", tickets).
		Update("cloud_synced_at", time.Now()).Error
}

// MarkTradesBlocked menandai trade yang ditolak server karena di luar jendela
// paket. Satu UPDATE untuk seluruh daftar, sepola dengan MarkTradesSynced.
func MarkTradesBlocked(tickets []string) error {
	if len(tickets) == 0 {
		return nil
	}
	return DB.Model(&Trade{}).
		Where("ticket IN ?", tickets).
		Update("cloud_blocked_at", time.Now()).Error
}

// ClearCloudBlocked mengembalikan SELURUH trade yang tertahan jendela ke
// antrean kirim. Dipanggil saat jendela paket melebar (user upgrade) dan saat
// user menekan sync manual — dua saat di mana "coba lagi" benar-benar berarti.
func ClearCloudBlocked() error {
	return DB.Model(&Trade{}).
		Where("cloud_blocked_at IS NOT NULL").
		Update("cloud_blocked_at", nil).Error
}

// ResetCloudSyncMarks memaksa seluruh riwayat lokal dianggap belum tersinkron,
// untuk kasus data di server hilang/di-reset dan perlu dibangun ulang dari
// jurnal lokal.
func ResetCloudSyncMarks() error {
	return DB.Model(&Trade{}).
		Where("cloud_synced_at IS NOT NULL").
		Update("cloud_synced_at", nil).Error
}

// SaveSetting menyimpan pengaturan ke database
func SaveSetting(key, value string) error {
	setting := Setting{Key: key, Value: value}
	return DB.Save(&setting).Error
}

// GetSetting mengambil pengaturan dari database
func GetSetting(key string) string {
	var setting Setting
	err := DB.First(&setting, "key = ?", key).Error
	if err != nil {
		return ""
	}
	return setting.Value
}

// activeAccount returns the (login, server) of the MT5 account currently active
// in the terminal, as last written by the MT5 fetch path. Empty strings mean the
// terminal has not been read yet this session; callers treat that as "unknown".
func activeAccount() (login, server string) {
	return GetSetting("mt5_account"), GetSetting("mt5_server")
}

// accountBalanceKey namespaces the reconstructed capital per MT5 account so
// switching accounts doesn't overwrite another account's drawdown basis. Falls
// back to the legacy global key when the account is unknown.
func accountBalanceKey() string {
	login, server := activeAccount()
	if login == "" || server == "" {
		return "account_balance"
	}
	return "account_balance:" + login + ":" + server
}

// GetAccountBalance / SetAccountBalance read/write the active account's balance.
func GetAccountBalance() string        { return GetSetting(accountBalanceKey()) }
func SetAccountBalance(v string) error { return SaveSetting(accountBalanceKey(), v) }

// GetTradesByPeriod mengambil history trade dari database berdasarkan periode filter
// period bisa berupa "today", "30", "60", "90", "all", atau "custom:YYYY-MM-DD:YYYY-MM-DD"
func GetTradesByPeriod(period string) ([]Trade, error) {
	var trades []Trade
	query := DB.Order("close_time desc")

	// Scope to the currently active MT5 account so the UI never blends two
	// accounts' history. If the active account is unknown (MT5 not read yet this
	// session), fall back to no filter rather than showing a blank screen — the
	// scope re-engages within one 5s tick once the terminal is read.
	if login, server := activeAccount(); login != "" && server != "" {
		query = query.Where("mt5_login = ? AND mt5_server = ?", login, server)
	}

	now := time.Now()
	var startTime time.Time

	if strings.HasPrefix(period, "custom:") {
		parts := strings.Split(period, ":")
		if len(parts) == 3 {
			// Parse dates. Adding time bounds for full day coverage
			startDate, errStart := time.Parse("2006-01-02", parts[1])
			endDate, errEnd := time.Parse("2006-01-02", parts[2])
			if errStart == nil && errEnd == nil {
				// Make endDate include the end of the day
				endDate = endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
				query = query.Where("close_time >= ? AND close_time <= ?", startDate, endDate)
			}
		}
	} else if period != "all" {
		switch period {
		case "today":
			startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		case "30":
			startTime = now.AddDate(0, 0, -30)
		case "60":
			startTime = now.AddDate(0, 0, -60)
		case "90":
			startTime = now.AddDate(0, 0, -90)
		}
		if !startTime.IsZero() {
			query = query.Where("close_time >= ?", startTime)
		}
	}

	err := query.Find(&trades).Error
	return trades, err
}

// AddManualTrade menambahkan trade secara manual ke database
func AddManualTrade(trade Trade) error {
	trade.Ticket = fmt.Sprintf("%d", time.Now().UnixNano()) // Generate fake ticket ID
	trade.MT5Login = GetSetting("mt5_account")
	trade.MT5Server = GetSetting("mt5_server")
	return DB.Create(&trade).Error
}



// CacheOpenPositions menyimpan snapshot open positions ke database
func CacheOpenPositions(positions []OpenPosition) {
	for _, p := range positions {
		// Use Ticket as PositionID for now since OpenPosition doesn't expose PositionID directly, 
		// but in MT5 for deals, PositionID usually matches the original Ticket of the open position.
		pos := SavedOpenPosition{
			PositionID: p.Ticket,
			Ticket:     p.Ticket,
			Symbol:     p.Symbol,
			Type:       p.Type,
			SL:         p.SL,
			TP:         p.TP,
			MT5Login:   p.MT5Login,
			MT5Server:  p.MT5Server,
		}
		// Save to db, updating if exists
		DB.Save(&pos)
	}
}
