package backend

import (
	"testing"
	"time"
)

func TestShouldBackfill(t *testing.T) {
	cases := []struct {
		name      string
		oldWindow int
		newWindow int
		toggleOn  bool
		want      bool
	}{
		{"melebar dari 1 ke 365 dengan toggle on", 1, 365, true, true},
		{"melebar ke tanpa batas dengan toggle on", 365, 0, true, true},
		{"melebar tapi toggle off", 1, 365, false, false},
		{"menyempit tidak pernah backfill", 365, 1, true, false},
		{"tanpa batas ke terbatas tidak pernah backfill", 0, 365, true, false},
		{"tidak berubah", 30, 30, true, false},
		{"tanpa batas tetap tanpa batas", 0, 0, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldBackfill(tc.oldWindow, tc.newWindow, tc.toggleOn)
			if got != tc.want {
				t.Fatalf("shouldBackfill(%d, %d, %v) = %v, mau %v",
					tc.oldWindow, tc.newWindow, tc.toggleOn, got, tc.want)
			}
		})
	}
}

// 0 berarti TANPA BATAS, jadi ia lebih lebar dari angka berapa pun. Perbandingan
// numerik polos akan membacanya sebagai paling sempit dan membalik seluruh
// logika upgrade — jebakan yang paling mungkin terjadi di fitur ini.
func TestWiderThan(t *testing.T) {
	cases := []struct {
		a, b int
		want bool
	}{
		{0, 365, true},  // tanpa batas lebih lebar dari setahun
		{365, 0, false}, // setahun TIDAK lebih lebar dari tanpa batas
		{0, 0, false},   // sama
		{365, 30, true},
		{30, 365, false},
		{30, 30, false},
	}
	for _, tc := range cases {
		if got := widerThan(tc.a, tc.b); got != tc.want {
			t.Fatalf("widerThan(%d, %d) = %v, mau %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestLantaiAkunDisimpanDanDibacaPerAkun(t *testing.T) {
	SaveSetting("mt5_account", "111")
	defer SaveSetting("mt5_account", "")

	SaveSyncFloor(nil) // pastikan bersih untuk akun 111
	if got := CachedSyncFloor(); !got.IsZero() {
		t.Fatalf("lantai awal = %v, mau zero (belum diketahui)", got)
	}

	floor := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	SaveSyncFloor(&floor)

	got := CachedSyncFloor()
	if !got.Equal(floor) {
		t.Fatalf("lantai = %v, mau %v", got, floor)
	}

	// Akun MT5 LAIN tidak boleh mewarisi lantai akun ini — lantai itu per-akun.
	SaveSetting("mt5_account", "222")
	if got := CachedSyncFloor(); !got.IsZero() {
		t.Fatalf("akun lain mewarisi lantai %v", got)
	}
}

func TestPartitionByFloor(t *testing.T) {
	floor := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	trades := []Trade{
		{Ticket: "lama", CloseTime: floor.AddDate(0, 0, -1)},
		{Ticket: "tepat", CloseTime: floor},
		{Ticket: "baru", CloseTime: floor.AddDate(0, 0, 1)},
	}

	sendable, blocked := partitionByFloor(trades, floor)

	if len(sendable) != 2 || sendable[0].Ticket != "tepat" || sendable[1].Ticket != "baru" {
		t.Fatalf("sendable = %+v, mau tepat & baru (tepat di lantai IKUT terkirim)", sendable)
	}
	if len(blocked) != 1 || blocked[0] != "lama" {
		t.Fatalf("blocked = %v, mau [lama]", blocked)
	}
}

// Lantai zero berarti BELUM DIKETAHUI, bukan "tolak semuanya". Salah menangani
// ini akan memblokir seluruh riwayat user pada sync pertama mereka.
func TestPartitionByFloorZeroMelewatkanSemua(t *testing.T) {
	trades := []Trade{
		{Ticket: "a", CloseTime: time.Now().AddDate(-9, 0, 0)},
		{Ticket: "b", CloseTime: time.Now()},
	}

	sendable, blocked := partitionByFloor(trades, time.Time{})

	if len(sendable) != 2 {
		t.Fatalf("sendable = %d, mau 2 — lantai belum diketahui tidak boleh menahan apa pun", len(sendable))
	}
	if len(blocked) != 0 {
		t.Fatalf("blocked = %v, mau kosong", blocked)
	}
}
