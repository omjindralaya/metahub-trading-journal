//go:build windows

package backend

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"syscall"
	"unsafe"
)

// softwareKeyProvider menyimpan key ECDSA P-256 di SQLite lokal, disegel DPAPI
// (CryptProtectData, terikat akun OS user).
//
// Perlindungan yang JUJUR TENTANG BATASNYA: DPAPI menghentikan pembacaan key
// oleh proses/user LAIN di mesin yang sama. Ia TIDAK menghentikan pemilik akun
// itu sendiri (ia bisa memanggil CryptUnprotectData sendiri). Karena itu server
// mencatatnya sebagai key_protection="software" — sinyal lebih lemah dari "tpm".
type softwareKeyProvider struct {
	priv *ecdsa.PrivateKey
}

func (p *softwareKeyProvider) PublicKeyBase64() (string, error) {
	return encodePublicKey(&p.priv.PublicKey)
}

func (p *softwareKeyProvider) SignDigest(digest []byte) ([]byte, error) {
	return ecdsa.SignASN1(rand.Reader, p.priv, digest)
}

func (p *softwareKeyProvider) KeyProtection() string { return "software" }

// marshalPrivateForTest hanya dipakai test (memastikan blob tersimpan bukan
// private key telanjang).
func (p *softwareKeyProvider) marshalPrivateForTest() (string, error) {
	der, err := x509.MarshalECPrivateKey(p.priv)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// loadOrCreateSoftwareKey memuat key tersegel dari SQLite, atau membuat satu.
// Key HARUS persisten: regenerasi tiap boot akan melahirkan device baru tiap restart.
func loadOrCreateSoftwareKey() (KeyProvider, error) {
	if sealed := GetSetting(deviceKeySettingKey); sealed != "" {
		blob, err := base64.StdEncoding.DecodeString(sealed)
		if err == nil {
			if der, err := dpapiUnprotect(blob); err == nil {
				if priv, err := x509.ParseECPrivateKey(der); err == nil {
					return &softwareKeyProvider{priv: priv}, nil
				}
			}
		}
		// Blob rusak / tak bisa dibuka (profil Windows berubah). Buat key baru:
		// itu memang device "baru" dari sudut pandang server.
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat key device: %w", err)
	}
	der, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	sealed, err := dpapiProtect(der)
	if err != nil {
		return nil, fmt.Errorf("gagal menyegel key device dengan DPAPI: %w", err)
	}
	if err := SaveSetting(deviceKeySettingKey, base64.StdEncoding.EncodeToString(sealed)); err != nil {
		return nil, err
	}
	if err := SaveSetting(deviceKeyProtectionSetting, "software"); err != nil {
		return nil, err
	}
	_ = SaveSetting(deviceIDSettingKey, "") // device_id lama tak cocok dengan key baru

	return &softwareKeyProvider{priv: priv}, nil
}

// --- DPAPI (crypt32.dll) ---

var (
	crypt32            = syscall.NewLazyDLL("crypt32.dll")
	kernel32DPAPI      = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtect   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotect = crypt32.NewProc("CryptUnprotectData")
	procLocalFree      = kernel32DPAPI.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(d []byte) dataBlob {
	if len(d) == 0 {
		return dataBlob{}
	}
	return dataBlob{cbData: uint32(len(d)), pbData: &d[0]}
}

func (b *dataBlob) bytes() []byte {
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

func dpapiProtect(data []byte) ([]byte, error) {
	in := newBlob(data)
	var out dataBlob
	r, _, err := procCryptProtect.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return out.bytes(), nil
}

func dpapiUnprotect(data []byte) ([]byte, error) {
	in := newBlob(data)
	var out dataBlob
	r, _, err := procCryptUnprotect.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return out.bytes(), nil
}
