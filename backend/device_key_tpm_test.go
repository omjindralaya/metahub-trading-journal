//go:build windows

package backend

import "testing"

// Mesin tanpa TPM adalah kondisi NORMAL — test di-skip di sana, bukan gagal.
func TestTPMKeyProviderSatisfiesContract(t *testing.T) {
	p, err := loadOrCreateTPMKey()
	if err != nil {
		t.Skipf("TPM tidak tersedia di mesin ini: %v", err)
	}
	assertKeyProviderContract(t, p) // kontrak yang SAMA dengan provider software
}

func TestTPMKeyProtectionLabel(t *testing.T) {
	p, err := loadOrCreateTPMKey()
	if err != nil {
		t.Skipf("TPM tidak tersedia: %v", err)
	}
	if got := p.KeyProtection(); got != "tpm" {
		t.Fatalf("KeyProtection = %q, harus \"tpm\"", got)
	}
}

func TestTPMKeyIsStableAcrossReloads(t *testing.T) {
	first, err := loadOrCreateTPMKey()
	if err != nil {
		t.Skipf("TPM tidak tersedia: %v", err)
	}
	pub1, err := first.PublicKeyBase64()
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateTPMKey()
	if err != nil {
		t.Fatal(err)
	}
	pub2, err := second.PublicKeyBase64()
	if err != nil {
		t.Fatal(err)
	}
	if pub1 != pub2 {
		t.Fatal("key TPM berubah antar pembukaan — harus persisted, bukan ephemeral")
	}
}

// GetKeyProvider harus MEMILIH TPM bila ada, turun ke software bila tidak,
// tanpa pernah gagal keras.
func TestGetKeyProviderNeverHardFails(t *testing.T) {
	p, err := GetKeyProvider()
	if err != nil {
		t.Fatalf("GetKeyProvider tidak boleh gagal keras: %v", err)
	}
	switch p.KeyProtection() {
	case "tpm", "software":
	default:
		t.Fatalf("key_protection tak dikenal: %q", p.KeyProtection())
	}
}
