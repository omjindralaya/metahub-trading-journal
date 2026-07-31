package backend

import "testing"

func TestEvaluateSyncTarget(t *testing.T) {
	cases := []struct {
		name                      string
		targetLogin, targetServer string
		activeLogin, activeServer string
		want                      syncTargetState
	}{
		{"active unknown blocks", "500", "BrokerA", "", "", syncTargetUnknown},
		{"active server unknown blocks", "500", "BrokerA", "500", "", syncTargetUnknown},
		{"no target yet adopts active", "", "", "500", "BrokerA", syncTargetAdopt},
		{"half-set target still adopts", "500", "", "500", "BrokerA", syncTargetAdopt},
		{"same login and server matches", "500", "BrokerA", "500", "BrokerA", syncTargetMatch},
		{"different login mismatches", "500", "BrokerA", "999", "BrokerA", syncTargetMismatch},
		{"same login different server mismatches", "500", "BrokerA-Demo", "500", "BrokerA-Real", syncTargetMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evaluateSyncTarget(tc.targetLogin, tc.targetServer, tc.activeLogin, tc.activeServer)
			if got != tc.want {
				t.Fatalf("evaluateSyncTarget(%q,%q,%q,%q) = %v, want %v",
					tc.targetLogin, tc.targetServer, tc.activeLogin, tc.activeServer, got, tc.want)
			}
		})
	}
}

// clearSyncTargetSettings resets the settings this guard reads so cases don't
// leak into each other (tests share one InitDB'd SQLite store via TestMain).
func clearSyncTargetSettings(t *testing.T) {
	t.Helper()
	SaveSetting(syncTargetLoginKey, "")
	SaveSetting(syncTargetServerKey, "")
	SaveSetting("mt5_account", "")
	SaveSetting("mt5_server", "")
	SetSyncTargetHandler(nil)
}

func TestSyncTargetPushAllowed_AdoptsThenBlocksOnSwitch(t *testing.T) {
	clearSyncTargetSettings(t)
	defer clearSyncTargetSettings(t)

	var events []SyncTargetEvent
	SetSyncTargetHandler(func(e SyncTargetEvent) { events = append(events, e) })

	// First sight of account 500/BrokerA: adopt as target, allow push.
	SaveSetting("mt5_account", "500")
	SaveSetting("mt5_server", "BrokerA")
	if !syncTargetPushAllowed() {
		t.Fatal("first-run adopt must allow the push")
	}
	if GetSetting(syncTargetLoginKey) != "500" || GetSetting(syncTargetServerKey) != "BrokerA" {
		t.Fatalf("target not persisted: login=%q server=%q",
			GetSetting(syncTargetLoginKey), GetSetting(syncTargetServerKey))
	}
	if len(events) != 1 || events[0].Status != "adopted" {
		t.Fatalf("expected one 'adopted' event, got %+v", events)
	}

	// Same account again: match, allow, no new event.
	if !syncTargetPushAllowed() {
		t.Fatal("matching account must allow the push")
	}
	if len(events) != 1 {
		t.Fatalf("match should not emit an event, got %+v", events)
	}

	// A different account becomes active: block, emit 'blocked' carrying both.
	SaveSetting("mt5_account", "999")
	SaveSetting("mt5_server", "BrokerA")
	if syncTargetPushAllowed() {
		t.Fatal("mismatched account MUST NOT be pushed")
	}
	last := events[len(events)-1]
	if last.Status != "blocked" || last.Login != "999" || last.TargetLogin != "500" {
		t.Fatalf("blocked event must carry active(999) vs target(500), got %+v", last)
	}
}

func TestSyncTargetPushAllowed_UnknownActiveBlocks(t *testing.T) {
	clearSyncTargetSettings(t)
	defer clearSyncTargetSettings(t)
	SaveSetting(syncTargetLoginKey, "500")
	SaveSetting(syncTargetServerKey, "BrokerA")
	// mt5_account/server left empty → active unknown → fail closed.
	if syncTargetPushAllowed() {
		t.Fatal("unknown active account must block the push (fail-closed)")
	}
}

func TestSyncToCloud_TargetMismatch_ReturnsError(t *testing.T) {
	clearSyncTargetSettings(t)
	defer clearSyncTargetSettings(t)

	SaveSetting("metahub_jwt", "tok")
	SaveSetting(syncTargetLoginKey, "500")
	SaveSetting(syncTargetServerKey, "BrokerA")
	SaveSetting("mt5_account", "999")
	SaveSetting("mt5_server", "BrokerA")

	_, err := SyncToCloud()
	if err == nil {
		t.Fatal("manual full-history sync must refuse a non-target account")
	}
}
