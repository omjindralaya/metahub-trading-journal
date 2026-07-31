package backend

import (
	"testing"
	"time"
)

func nowForTest() time.Time { return time.Now() }

func TestSaveTrades_StampsActiveAccountAndUpsertsPerAccount(t *testing.T) {
	DB.Exec("DELETE FROM trades")
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")

	// Save the same ticket under account 500, then under account 999.
	if err := SaveTrades([]Trade{{Ticket: "77", Symbol: "XAUUSD", Type: "Buy", Volume: 1,
		MT5Login: "500", MT5Server: "BrokerA"}}); err != nil {
		t.Fatalf("save acct 500: %v", err)
	}
	if err := SaveTrades([]Trade{{Ticket: "77", Symbol: "EURUSD", Type: "Sell", Volume: 2,
		MT5Login: "999", MT5Server: "BrokerB"}}); err != nil {
		t.Fatalf("save acct 999: %v", err)
	}

	var count int64
	DB.Model(&Trade{}).Where("ticket = ?", "77").Count(&count)
	if count != 2 {
		t.Fatalf("same ticket under two accounts must be two rows, got %d", count)
	}
}

func TestGetTradesByPeriod_ShowsOnlyActiveAccount(t *testing.T) {
	DB.Exec("DELETE FROM trades")
	now := nowForTest()
	// Two accounts, one closed trade each, both within the last 30 days.
	DB.Create(&Trade{Ticket: "a1", Symbol: "XAUUSD", Type: "Buy", Volume: 1,
		CloseTime: now, MT5Login: "500", MT5Server: "BrokerA"})
	DB.Create(&Trade{Ticket: "b1", Symbol: "EURUSD", Type: "Sell", Volume: 1,
		CloseTime: now, MT5Login: "999", MT5Server: "BrokerB"})

	// Active account is 500 → only its trade shows.
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")
	got, err := GetTradesByPeriod("30")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 || got[0].Ticket != "a1" {
		t.Fatalf("expected only active account 500's trade, got %d rows: %+v", len(got), got)
	}

	// Unknown active account → fall back to no filter (show all), never a blank screen.
	SaveSetting("mt5_account", "")
	SaveSetting("mt5_server", "")
	all, _ := GetTradesByPeriod("30")
	if len(all) != 2 {
		t.Fatalf("unknown active account should show all, got %d", len(all))
	}
}
