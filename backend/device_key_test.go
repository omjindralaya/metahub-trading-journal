package backend

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	InitDB()
	os.Exit(m.Run())
}

// Kontrak KeyProvider: apa pun implementasinya (software hari ini, TPM besok),
// ia harus menghasilkan public key P-256 dan tanda tangan ASN.1 DER yang lolos
// verifikasi standar. Test ini adalah kontrak yang sama untuk keduanya.
func assertKeyProviderContract(t *testing.T, p KeyProvider) {
	t.Helper()

	pubB64, err := p.PublicKeyBase64()
	if err != nil {
		t.Fatalf("PublicKeyBase64: %v", err)
	}
	if pubB64 == "" {
		t.Fatal("public key kosong")
	}

	pub, err := parsePublicKeyBase64(pubB64)
	if err != nil {
		t.Fatalf("public key tidak bisa diurai: %v", err)
	}
	if pub.Curve.Params().Name != "P-256" {
		t.Fatalf("kurva = %s, harus P-256", pub.Curve.Params().Name)
	}

	msg := []byte("canonical string apa pun")
	digest := sha256.Sum256(msg)

	sig, err := p.SignDigest(digest[:])
	if err != nil {
		t.Fatalf("SignDigest: %v", err)
	}
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		t.Fatal("tanda tangan tidak lolos verifikasi ECDSA ASN.1")
	}
}

func TestSoftwareKeyProviderSatisfiesContract(t *testing.T) {
	p := newTestSoftwareProvider(t)
	assertKeyProviderContract(t, p)
}

// Key HARUS persisten. Kalau regenerasi tiap boot, device_id berubah tiap
// restart dan jejak audit jadi tak berguna.
func TestSoftwareKeyIsStableAcrossReloads(t *testing.T) {
	first := newTestSoftwareProvider(t)
	pub1, err := first.PublicKeyBase64()
	if err != nil {
		t.Fatal(err)
	}

	second := newTestSoftwareProvider(t) // memuat ulang dari setting yang sama
	pub2, err := second.PublicKeyBase64()
	if err != nil {
		t.Fatal(err)
	}

	if pub1 != pub2 {
		t.Fatal("public key berubah antar reload — device_id akan berganti tiap restart")
	}
}

func TestSoftwareKeyProtectionLabel(t *testing.T) {
	if got := newTestSoftwareProvider(t).KeyProtection(); got != "software" {
		t.Fatalf("KeyProtection = %q, harus \"software\"", got)
	}
}

// Blob DPAPI tidak boleh menyimpan private key dalam bentuk telanjang.
func TestSealedBlobIsNotPlaintextKey(t *testing.T) {
	p := newTestSoftwareProvider(t)
	sealed := GetSetting(deviceKeySettingKey)
	if sealed == "" {
		t.Fatal("key tersegel tidak tersimpan")
	}
	raw, err := p.(*softwareKeyProvider).marshalPrivateForTest()
	if err != nil {
		t.Fatal(err)
	}
	if sealed == raw {
		t.Fatal("private key tersimpan telanjang — DPAPI tidak dipakai")
	}
}

func newTestSoftwareProvider(t *testing.T) KeyProvider {
	t.Helper()
	p, err := loadOrCreateSoftwareKey()
	if err != nil {
		t.Fatalf("loadOrCreateSoftwareKey: %v", err)
	}
	return p
}
