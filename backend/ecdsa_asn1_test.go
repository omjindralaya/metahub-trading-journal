package backend

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"testing"
)

// rawECDSAToASN1 mengubah r‖s mentah (yang dikembalikan CNG/TPM) menjadi ASN.1
// DER (yang dituntut ecdsa.VerifyASN1 di server). Ini test paling penting di
// Task 14: kalau konversi ini salah, SETIAP tanda tangan TPM ditolak 401.
func TestRawECDSAToASN1VerifiesAgainstStdlib(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("pesan uji"))

	// Tandatangani dengan stdlib untuk mendapat r,s, lalu susun r‖s mentah seperti CNG.
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 64)
	r.FillBytes(raw[:32])
	s.FillBytes(raw[32:])

	der, err := rawECDSAToASN1(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !ecdsa.VerifyASN1(&priv.PublicKey, digest[:], der) {
		t.Fatal("DER hasil konversi tidak lolos VerifyASN1")
	}
}

// r dan s dengan high-bit set (butuh padding 0x00 di DER) harus tetop valid.
func TestRawECDSAToASN1HandlesHighBitValues(t *testing.T) {
	// r dan s yang byte pertamanya >= 0x80.
	raw := make([]byte, 64)
	for i := range raw {
		raw[i] = 0xff
	}
	der, err := rawECDSAToASN1(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Harus mengurai sebagai SEQUENCE dari dua INTEGER positif.
	var rr, ss big.Int
	if err := parseTwoASN1Ints(der, &rr, &ss); err != nil {
		t.Fatalf("DER tidak bisa diurai: %v", err)
	}
	if rr.Sign() < 0 || ss.Sign() < 0 {
		t.Fatal("integer DER tidak boleh negatif (padding 0x00 hilang)")
	}
}

func TestRawECDSAToASN1RejectsWrongLength(t *testing.T) {
	if _, err := rawECDSAToASN1(make([]byte, 63)); err == nil {
		t.Fatal("panjang ganjil/​salah harus ditolak")
	}
	if _, err := rawECDSAToASN1(nil); err == nil {
		t.Fatal("nil harus ditolak")
	}
}

// helper test: urai SEQUENCE{INTEGER,INTEGER}.
func parseTwoASN1Ints(der []byte, r, s *big.Int) error {
	return parseECDSASignatureForTest(der, r, s)
}
