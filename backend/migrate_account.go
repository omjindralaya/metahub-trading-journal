package backend

import "log"

// migrateAccountIdentity makes the local journal account-aware. It is idempotent
// and safe to run on every startup:
//
//  1. Backfill mt5_login/mt5_server on legacy trades (rows created before this
//     column existed) with the last-known active account. Pre-migration local
//     data is already blended across accounts and cannot be un-mixed reliably;
//     stamping it with the current account is the honest best-effort, and is
//     exactly correct for the common single-account user.
//  2. Replace the GLOBAL unique index on trades.ticket with a COMPOSITE unique
//     index on (mt5_login, mt5_server, ticket). MT5 deal tickets are unique only
//     within one trade server, so the old global index let a second account's
//     ticket overwrite the first account's row.
//  3. Rebuild the ephemeral open-position cache so it carries the current schema
//     — the account columns, and the COMPOSITE primary key that keeps two
//     accounts sharing a position_id from overwriting each other's SL/TP.
//     Dropping it is harmless — AutoSyncTick re-populates it within one tick,
//     and that is also what applies the primary-key change: SQLite cannot alter
//     a primary key in place, so the rebuild IS the migration.
func migrateAccountIdentity() error {
	// (1) Backfill legacy trades.
	login := GetSetting("mt5_account")
	server := GetSetting("mt5_server")
	if login != "" && server != "" {
		if err := DB.Exec(
			"UPDATE trades SET mt5_login = ?, mt5_server = ? WHERE COALESCE(mt5_login,'') = ''",
			login, server,
		).Error; err != nil {
			return err
		}
	}

	// (2) Swap the global ticket unique index for a composite one. GORM named the
	// old `gorm:"uniqueIndex"` index `idx_trades_ticket`; dropping it is a no-op
	// if a prior run already did so.
	if err := DB.Exec("DROP INDEX IF EXISTS idx_trades_ticket").Error; err != nil {
		return err
	}
	if err := DB.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_trades_account_ticket ON trades(mt5_login, mt5_server, ticket)",
	).Error; err != nil {
		return err
	}

	// (3) Rebuild the ephemeral open-position cache with the new schema.
	if err := DB.Exec("DROP TABLE IF EXISTS saved_open_positions").Error; err != nil {
		return err
	}
	if err := DB.AutoMigrate(&SavedOpenPosition{}); err != nil {
		return err
	}

	log.Println("migrateAccountIdentity: jurnal lokal kini sadar-akun (composite ticket index aktif)")
	return nil
}
