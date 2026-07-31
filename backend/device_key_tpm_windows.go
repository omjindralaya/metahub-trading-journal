//go:build windows

package backend

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"errors"
	"fmt"
	"math/big"
	"syscall"
	"unsafe"
)

// errTPMUnavailable: mesin ini tidak punya TPM 2.0 yang bisa dipakai. Kondisi
// NORMAL (VPS, mesin lama) — pemanggil turun ke key software, bukan gagal.
var errTPMUnavailable = errors.New("device_key: TPM 2.0 tidak tersedia")

// tpmKeyName: nama key persisted di TPM. Tetap sama antar rilis — menggantinya =
// melahirkan device baru bagi setiap user yang sudah terdaftar.
const tpmKeyName = "MetaHubDeviceIdentity"

// platformProvider: CNG provider yang didukung TPM.
const platformProvider = "Microsoft Platform Crypto Provider"

// tpmKeyProvider: key ECDSA P-256 yang hidup DI DALAM chip TPM. Private key
// non-exportable — tidak pernah keluar dari chip. Mencegah KLONING; TIDAK
// mencegah pemilik mesin menandatangani kebohongan (ia panggil SignDigest saja).
type tpmKeyProvider struct {
	provider ncryptHandle
	key      ncryptHandle
	pub      *ecdsa.PublicKey
}

func (p *tpmKeyProvider) KeyProtection() string { return "tpm" }

func (p *tpmKeyProvider) PublicKeyBase64() (string, error) {
	return encodePublicKey(p.pub)
}

// SignDigest memanggil NCryptSignHash. CNG mengembalikan r‖s mentah; server
// menuntut ASN.1 DER — dikonversi lewat rawECDSAToASN1.
func (p *tpmKeyProvider) SignDigest(digest []byte) ([]byte, error) {
	var size uint32
	st, _, _ := procNCryptSignHash.Call(
		uintptr(p.key), 0,
		uintptr(unsafe.Pointer(&digest[0])), uintptr(len(digest)),
		0, 0,
		uintptr(unsafe.Pointer(&size)), 0,
	)
	if st != 0 {
		return nil, fmt.Errorf("NCryptSignHash (ukuran): 0x%x", st)
	}
	raw := make([]byte, size)
	st, _, _ = procNCryptSignHash.Call(
		uintptr(p.key), 0,
		uintptr(unsafe.Pointer(&digest[0])), uintptr(len(digest)),
		uintptr(unsafe.Pointer(&raw[0])), uintptr(size),
		uintptr(unsafe.Pointer(&size)), 0,
	)
	if st != 0 {
		return nil, fmt.Errorf("NCryptSignHash: 0x%x", st)
	}
	return rawECDSAToASN1(raw[:size])
}

// loadOrCreateTPMKey membuka key persisted di TPM, atau membuatnya sekali.
func loadOrCreateTPMKey() (KeyProvider, error) {
	prov, err := ncryptOpenStorageProvider(platformProvider)
	if err != nil {
		return nil, errTPMUnavailable
	}

	key, err := ncryptOpenKey(prov, tpmKeyName)
	if err != nil {
		if key, err = ncryptCreatePersistedECDSAKey(prov, tpmKeyName); err != nil {
			ncryptFreeObject(prov)
			return nil, fmt.Errorf("gagal membuat key TPM: %w", err)
		}
		_ = SaveSetting(deviceIDSettingKey, "") // key TPM baru = device baru
	}

	pub, err := ncryptExportECDSAPublicKey(key)
	if err != nil {
		ncryptFreeObject(key)
		ncryptFreeObject(prov)
		return nil, fmt.Errorf("gagal mengekspor public key TPM: %w", err)
	}

	_ = SaveSetting(deviceKeyProtectionSetting, "tpm")
	return &tpmKeyProvider{provider: prov, key: key, pub: pub}, nil
}

// --- Binding NCrypt (ncrypt.dll) ---

type ncryptHandle uintptr

var (
	ncrypt                        = syscall.NewLazyDLL("ncrypt.dll")
	procNCryptOpenStorageProvider = ncrypt.NewProc("NCryptOpenStorageProvider")
	procNCryptOpenKey             = ncrypt.NewProc("NCryptOpenKey")
	procNCryptCreatePersistedKey  = ncrypt.NewProc("NCryptCreatePersistedKey")
	procNCryptFinalizeKey         = ncrypt.NewProc("NCryptFinalizeKey")
	procNCryptExportKey           = ncrypt.NewProc("NCryptExportKey")
	procNCryptSignHash            = ncrypt.NewProc("NCryptSignHash")
	procNCryptFreeObject          = ncrypt.NewProc("NCryptFreeObject")
)

const (
	ncryptEcdsaP256Algorithm = "ECDSA_P256"
	bcryptEccPublicBlob      = "ECCPUBLICBLOB"
)

func utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}

func ncryptOpenStorageProvider(name string) (ncryptHandle, error) {
	var h ncryptHandle
	st, _, _ := procNCryptOpenStorageProvider.Call(
		uintptr(unsafe.Pointer(&h)),
		uintptr(unsafe.Pointer(utf16Ptr(name))),
		0,
	)
	if st != 0 {
		return 0, fmt.Errorf("NCryptOpenStorageProvider: 0x%x", st)
	}
	return h, nil
}

func ncryptOpenKey(prov ncryptHandle, name string) (ncryptHandle, error) {
	var h ncryptHandle
	st, _, _ := procNCryptOpenKey.Call(
		uintptr(prov),
		uintptr(unsafe.Pointer(&h)),
		uintptr(unsafe.Pointer(utf16Ptr(name))),
		0, 0,
	)
	if st != 0 {
		return 0, fmt.Errorf("NCryptOpenKey: 0x%x", st)
	}
	return h, nil
}

func ncryptCreatePersistedECDSAKey(prov ncryptHandle, name string) (ncryptHandle, error) {
	var h ncryptHandle
	st, _, _ := procNCryptCreatePersistedKey.Call(
		uintptr(prov),
		uintptr(unsafe.Pointer(&h)),
		uintptr(unsafe.Pointer(utf16Ptr(ncryptEcdsaP256Algorithm))),
		uintptr(unsafe.Pointer(utf16Ptr(name))),
		0, 0,
	)
	if st != 0 {
		return 0, fmt.Errorf("NCryptCreatePersistedKey: 0x%x", st)
	}
	st, _, _ = procNCryptFinalizeKey.Call(uintptr(h), 0)
	if st != 0 {
		ncryptFreeObject(h)
		return 0, fmt.Errorf("NCryptFinalizeKey: 0x%x", st)
	}
	return h, nil
}

// BCRYPT_ECCKEY_BLOB: header (Magic uint32, cbKey uint32), lalu X (cbKey byte),
// lalu Y (cbKey byte). P-256 → cbKey = 32.
func ncryptExportECDSAPublicKey(key ncryptHandle) (*ecdsa.PublicKey, error) {
	blobType := utf16Ptr(bcryptEccPublicBlob)

	var size uint32
	st, _, _ := procNCryptExportKey.Call(
		uintptr(key), 0,
		uintptr(unsafe.Pointer(blobType)),
		0, 0, 0,
		uintptr(unsafe.Pointer(&size)), 0,
	)
	if st != 0 {
		return nil, fmt.Errorf("NCryptExportKey (ukuran): 0x%x", st)
	}
	blob := make([]byte, size)
	st, _, _ = procNCryptExportKey.Call(
		uintptr(key), 0,
		uintptr(unsafe.Pointer(blobType)),
		0,
		uintptr(unsafe.Pointer(&blob[0])), uintptr(size),
		uintptr(unsafe.Pointer(&size)), 0,
	)
	if st != 0 {
		return nil, fmt.Errorf("NCryptExportKey: 0x%x", st)
	}
	if size < 8 {
		return nil, fmt.Errorf("blob ECC terlalu pendek: %d byte", size)
	}
	cbKey := uint32(blob[4]) | uint32(blob[5])<<8 | uint32(blob[6])<<16 | uint32(blob[7])<<24
	if cbKey != 32 || uint32(len(blob)) < 8+2*cbKey {
		return nil, fmt.Errorf("blob ECC bukan P-256 (cbKey=%d, len=%d)", cbKey, len(blob))
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(blob[8 : 8+cbKey]),
		Y:     new(big.Int).SetBytes(blob[8+cbKey : 8+2*cbKey]),
	}, nil
}

func ncryptFreeObject(h ncryptHandle) {
	if h != 0 {
		procNCryptFreeObject.Call(uintptr(h))
	}
}
