package backend

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"log"
)

// Kunci setting SQLite lokal tempat identitas device disimpan.
const (
	deviceKeySettingKey        = "device_private_key_sealed" // blob DPAPI (base64), provider software
	deviceKeyProtectionSetting = "device_key_protection"     // "tpm" | "software"
	deviceIDSettingKey         = "device_id"                 // uuid dari server
)

// KeyProvider adalah identitas kriptografis device ini.
//
// Private key TIDAK PERNAH melewati interface ini — hanya public key dan operasi
// penandatanganan. Itulah yang membuat provider TPM (task berikutnya) bisa
// menggantikan yang software tanpa mengubah satu baris pun di jalur sync: pada
// TPM, private key memang tidak bisa dibaca, bahkan oleh kita.
//
// Yang dibeli TPM: mencegah KLONING device. Yang TIDAK dibeli: mencegah pemilik
// mesin menandatangani kebohongan — ia cukup memanggil API penandatanganan ini
// dengan data karangan, dan TPM akan patuh menandatanganinya.
type KeyProvider interface {
	// PublicKeyBase64 mengembalikan base64 dari DER SPKI (ECDSA P-256).
	PublicKeyBase64() (string, error)
	// SignDigest menandatangani digest SHA-256 (32 byte), mengembalikan ECDSA ASN.1 DER.
	SignDigest(digest []byte) ([]byte, error)
	// KeyProtection: "tpm" atau "software".
	KeyProtection() string
}

// GetKeyProvider mengembalikan identitas device, memilih perlindungan terkuat
// yang tersedia: TPM bila ada, DPAPI bila tidak.
//
// Ketiadaan TPM TIDAK BOLEH menggagalkan sync — banyak mesin trader adalah VPS
// tanpa TPM. Ia turun ke software, dan server mencatat perbedaannya.
func GetKeyProvider() (KeyProvider, error) {
	if p, err := loadOrCreateTPMKey(); err == nil {
		return p, nil
	} else if !errors.Is(err, errTPMUnavailable) {
		log.Printf("device_key: TPM ada tapi tidak bisa dipakai (%v); memakai key software", err)
	}
	return loadOrCreateSoftwareKey()
}

// parsePublicKeyBase64 adalah kebalikan dari PublicKeyBase64.
func parsePublicKeyBase64(pubB64 string) (*ecdsa.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	pub, ok := parsed.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, errors.New("public key bukan ECDSA P-256")
	}
	return pub, nil
}

// encodePublicKey: *ecdsa.PublicKey → base64(DER SPKI).
func encodePublicKey(pub *ecdsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}
