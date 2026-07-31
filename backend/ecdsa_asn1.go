package backend

import (
	"errors"
	"math/big"

	"golang.org/x/crypto/cryptobyte"
	asn1 "golang.org/x/crypto/cryptobyte/asn1"
)

// encodeECDSASignature membungkus (r,s) menjadi SEQUENCE ASN.1 DER — bentuk yang
// dihasilkan ecdsa.SignASN1 dan yang dituntut ecdsa.VerifyASN1 di server.
func encodeECDSASignature(r, s *big.Int) ([]byte, error) {
	var b cryptobyte.Builder
	b.AddASN1(asn1.SEQUENCE, func(child *cryptobyte.Builder) {
		child.AddASN1BigInt(r)
		child.AddASN1BigInt(s)
	})
	return b.Bytes()
}

// rawECDSAToASN1 mengubah r‖s mentah (CNG/TPM) menjadi ASN.1 DER. Tanpa konversi
// ini, SETIAP tanda tangan dari device ber-TPM ditolak server sebagai tidak sah.
func rawECDSAToASN1(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw)%2 != 0 {
		return nil, errors.New("panjang tanda tangan CNG tidak wajar")
	}
	half := len(raw) / 2
	r := new(big.Int).SetBytes(raw[:half])
	s := new(big.Int).SetBytes(raw[half:])
	return encodeECDSASignature(r, s)
}

// parseECDSASignatureForTest mengurai SEQUENCE{INTEGER,INTEGER}. Dipakai test
// (dan mendokumentasikan format DER yang kita hasilkan).
func parseECDSASignatureForTest(der []byte, r, s *big.Int) error {
	input := cryptobyte.String(der)
	var inner cryptobyte.String
	if !input.ReadASN1(&inner, asn1.SEQUENCE) {
		return errors.New("bukan SEQUENCE")
	}
	if !inner.ReadASN1Integer(r) || !inner.ReadASN1Integer(s) {
		return errors.New("gagal membaca dua INTEGER")
	}
	return nil
}
