package backend

import (
	"fmt"
	"sync"
)

// syncTargetState is the guard's verdict for the currently active MT5 account,
// given the account the user chose to sync ("target"). The push path is
// fail-closed: only syncTargetMatch (and the first-run syncTargetAdopt) allow a
// cloud push; every other state blocks it so the wrong account can never be
// synced silently.
type syncTargetState int

const (
	// syncTargetUnknown: the active account could not be read this cycle
	// (MT5 unreachable). Block — never push to a target we cannot confirm.
	syncTargetUnknown syncTargetState = iota
	// syncTargetAdopt: no target chosen yet, active account known. Adopt the
	// active account as the target (honors "sync the account I have open").
	syncTargetAdopt
	// syncTargetMatch: active account equals the chosen target. Allow push.
	syncTargetMatch
	// syncTargetMismatch: active account differs from the chosen target
	// (another terminal won pipe discovery, or the user switched accounts).
	// Block push and surface both identities to the user.
	syncTargetMismatch
)

// evaluateSyncTarget is a pure function: it decides the guard's verdict from the
// stored target and the currently active MT5 account. Both parts of an identity
// (login AND server) must be present and equal for a match.
func evaluateSyncTarget(targetLogin, targetServer, activeLogin, activeServer string) syncTargetState {
	if activeLogin == "" || activeServer == "" {
		return syncTargetUnknown
	}
	if targetLogin == "" || targetServer == "" {
		return syncTargetAdopt
	}
	if targetLogin == activeLogin && targetServer == activeServer {
		return syncTargetMatch
	}
	return syncTargetMismatch
}

// Settings keys for the user's chosen sync target. The active account uses the
// existing mt5_account/mt5_server keys that the sync payload is already built
// from, so the guard checks exactly what would be sent.
const (
	syncTargetLoginKey  = "sync_target_login"
	syncTargetServerKey = "sync_target_server"
)

// SyncTargetEvent tells the UI which account is active vs. chosen, so the choice
// is never hidden. Status is one of "adopted" (first-run pick) or "blocked"
// (active != chosen; push was refused).
type SyncTargetEvent struct {
	Status       string `json:"status"`
	Login        string `json:"login"`         // currently active MT5 login
	Server       string `json:"server"`        // currently active MT5 server
	TargetLogin  string `json:"target_login"`  // chosen login (empty on "adopted")
	TargetServer string `json:"target_server"` // chosen server (empty on "adopted")
}

var (
	syncTargetMu sync.RWMutex
	syncTargetFn func(SyncTargetEvent)
)

// SetSyncTargetHandler installs the receiver (the app layer forwards it to the
// frontend via a Wails event). Passing nil detaches it (used by tests).
func SetSyncTargetHandler(fn func(SyncTargetEvent)) {
	syncTargetMu.Lock()
	defer syncTargetMu.Unlock()
	syncTargetFn = fn
}

func emitSyncTarget(e SyncTargetEvent) {
	syncTargetMu.RLock()
	fn := syncTargetFn
	syncTargetMu.RUnlock()
	if fn != nil {
		fn(e)
	}
}

// syncTargetPushAllowed reports whether a cloud push may proceed for the
// currently active MT5 account. It adopts the active account as the target on
// first run, allows matching accounts, and blocks (fail-closed) on mismatch or
// when the active account is unknown — emitting an event on adopt/block so the
// UI can stay honest about which account is active vs. chosen.
func syncTargetPushAllowed() bool {
	targetLogin := GetSetting(syncTargetLoginKey)
	targetServer := GetSetting(syncTargetServerKey)
	activeLogin := GetSetting("mt5_account")
	activeServer := GetSetting("mt5_server")

	switch evaluateSyncTarget(targetLogin, targetServer, activeLogin, activeServer) {
	case syncTargetAdopt:
		SaveSetting(syncTargetLoginKey, activeLogin)
		SaveSetting(syncTargetServerKey, activeServer)
		emitSyncTarget(SyncTargetEvent{Status: "adopted", Login: activeLogin, Server: activeServer})
		return true
	case syncTargetMatch:
		return true
	case syncTargetMismatch:
		emitSyncTarget(SyncTargetEvent{
			Status: "blocked", Login: activeLogin, Server: activeServer,
			TargetLogin: targetLogin, TargetServer: targetServer,
		})
		return false
	default: // syncTargetUnknown
		return false
	}
}

// syncTargetGuardForManual is the user-facing guard for a manual sync: it
// returns a clear error when the active MT5 account is not the chosen target (or
// is unknown), instead of failing quietly. Unlike syncTargetPushAllowed it does
// NOT adopt on first run — first-run adoption belongs to the always-running
// auto-loop; a manual button should not silently claim a target. On the
// syncTargetAdopt state it returns nil (allow), and the auto-loop persists the
// target on its next tick.
func syncTargetGuardForManual() error {
	targetLogin := GetSetting(syncTargetLoginKey)
	targetServer := GetSetting(syncTargetServerKey)
	activeLogin := GetSetting("mt5_account")
	activeServer := GetSetting("mt5_server")
	switch evaluateSyncTarget(targetLogin, targetServer, activeLogin, activeServer) {
	case syncTargetMismatch:
		emitSyncTarget(SyncTargetEvent{
			Status: "blocked", Login: activeLogin, Server: activeServer,
			TargetLogin: targetLogin, TargetServer: targetServer,
		})
		return fmt.Errorf("Terminal aktif akun %s, tapi target sync kamu akun %s. "+
			"Ganti target ke akun aktif, atau buka akun %s di MT5 lalu coba lagi.",
			activeLogin, targetLogin, targetLogin)
	case syncTargetUnknown:
		return fmt.Errorf("Akun MT5 aktif belum terbaca. Pastikan MT5 terbuka lalu coba lagi.")
	default: // adopt or match — allow
		return nil
	}
}

// SyncTargetInfo is the chosen sync target, for display in the UI.
type SyncTargetInfo struct {
	Login  string `json:"login"`
	Server string `json:"server"`
}

// GetSyncTarget returns the chosen sync target (empty fields if none chosen yet).
func GetSyncTarget() SyncTargetInfo {
	return SyncTargetInfo{
		Login:  GetSetting(syncTargetLoginKey),
		Server: GetSetting(syncTargetServerKey),
	}
}
