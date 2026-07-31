package backend

import "testing"

func TestMigrateAccountIdentity_BackfillsAndAllowsCrossAccountSameTicket(t *testing.T) {
	// Legacy row: a trade with no account identity (as if inserted pre-migration).
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")
	DB.Exec("DELETE FROM trades")
	if err := DB.Create(&Trade{Ticket: "111", Symbol: "XAUUSD", Type: "Buy", Volume: 1}).Error; err != nil {
		t.Fatalf("seed legacy trade: %v", err)
	}

	if err := migrateAccountIdentity(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Legacy row backfilled to the last-known active account.
	var got Trade
	if err := DB.First(&got, "ticket = ?", "111").Error; err != nil {
		t.Fatalf("legacy trade missing after migration: %v", err)
	}
	if got.MT5Login != "500" || got.MT5Server != "BrokerA" {
		t.Fatalf("legacy trade not backfilled: login=%q server=%q", got.MT5Login, got.MT5Server)
	}

	// The composite unique index must allow the SAME ticket under a DIFFERENT
	// account — the exact collision that used to silently overwrite data.
	err := DB.Create(&Trade{Ticket: "111", Symbol: "EURUSD", Type: "Sell", Volume: 1,
		MT5Login: "999", MT5Server: "BrokerB"}).Error
	if err != nil {
		t.Fatalf("cross-account same-ticket insert must be allowed, got: %v", err)
	}

	var count int64
	DB.Model(&Trade{}).Where("ticket = ?", "111").Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 rows sharing ticket 111 across accounts, got %d", count)
	}
}
