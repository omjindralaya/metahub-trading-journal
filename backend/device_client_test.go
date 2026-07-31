package backend

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// Canonical string HARUS identik dengan devicesig.CanonicalString di server.
func TestCanonicalStringMatchesServerContract(t *testing.T) {
	body := []byte(`{"a":1}`)
	sum := sha256.Sum256(body)
	want := hexLower(sum[:]) + "\n1752400000\nabc\nPOST\n/api/v1/trades/sync"

	got := canonicalString(body, "1752400000", "abc", "POST", "/api/v1/trades/sync")
	if got != want {
		t.Fatalf("canonical string:\n got: %q\nwant: %q", got, want)
	}
}

func TestNewNonceIsRandomAndHex(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		n, err := newNonce()
		if err != nil {
			t.Fatal(err)
		}
		if len(n) != 32 { // 16 byte → 32 karakter hex
			t.Fatalf("panjang nonce = %d, harus 32", len(n))
		}
		if seen[n] {
			t.Fatal("nonce berulang")
		}
		seen[n] = true
	}
}

func TestSignRequestSetsAllHeaders(t *testing.T) {
	p := newTestSoftwareProvider(t)
	SaveSetting(deviceIDSettingKey, "11111111-1111-1111-1111-111111111111")

	body := []byte(`{"trades":[]}`)
	req, err := http.NewRequest("POST", "http://example.test/api/v1/trades/sync", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := signRequest(req, body, p); err != nil {
		t.Fatal(err)
	}

	for _, h := range []string{"X-Device-ID", "X-Timestamp", "X-Nonce", "X-Signature"} {
		if req.Header.Get(h) == "" {
			t.Fatalf("header %s tidak dipasang", h)
		}
	}

	pubB64, _ := p.PublicKeyBase64()
	pub, err := parsePublicKeyBase64(pubB64)
	if err != nil {
		t.Fatal(err)
	}
	canonical := canonicalString(body,
		req.Header.Get("X-Timestamp"), req.Header.Get("X-Nonce"),
		"POST", "/api/v1/trades/sync")
	digest := sha256.Sum256([]byte(canonical))
	sig, err := base64.StdEncoding.DecodeString(req.Header.Get("X-Signature"))
	if err != nil {
		t.Fatal(err)
	}
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		t.Fatal("tanda tangan tidak memverifikasi terhadap canonical string-nya sendiri")
	}
}

func TestSignRequestUsesCurrentTimestamp(t *testing.T) {
	p := newTestSoftwareProvider(t)
	SaveSetting(deviceIDSettingKey, "11111111-1111-1111-1111-111111111111")

	req, _ := http.NewRequest("POST", "http://example.test/api/v1/trades/sync", nil)
	if err := signRequest(req, []byte("{}"), p); err != nil {
		t.Fatal(err)
	}

	ts, err := strconv.ParseInt(req.Header.Get("X-Timestamp"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if delta := time.Since(time.Unix(ts, 0)); delta > 5*time.Second || delta < -5*time.Second {
		t.Fatalf("timestamp meleset %v dari waktu sekarang", delta)
	}
}

func TestSignRequestFailsWithoutDeviceID(t *testing.T) {
	p := newTestSoftwareProvider(t)
	SaveSetting(deviceIDSettingKey, "")

	req, _ := http.NewRequest("POST", "http://example.test/api/v1/trades/sync", nil)
	if err := signRequest(req, []byte("{}"), p); err == nil {
		t.Fatal("signRequest harus gagal saat device belum terdaftar")
	}
	if req.Header.Get("X-Signature") != "" {
		t.Fatal("header tanda tangan dipasang meski device belum terdaftar")
	}
}

func TestRegisterDeviceStoresReturnedID(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":{"device_id":"22222222-2222-2222-2222-222222222222"}}`))
	}))
	defer srv.Close()

	SaveSetting(deviceIDSettingKey, "")
	SaveSetting("metahub_jwt", "token-abc")

	if err := registerDeviceAt(srv.URL, "token-abc"); err != nil {
		t.Fatal(err)
	}

	if got := GetSetting(deviceIDSettingKey); got != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("device_id tersimpan = %q", got)
	}
	if gotBody["public_key"] == "" || gotBody["public_key"] == nil {
		t.Fatal("public_key tidak dikirim")
	}
	// key_protection mencerminkan apa pun yang dipilih GetKeyProvider di mesin
	// ini: "tpm" bila TPM 2.0 usable tersedia (mis. banyak mesin Windows 11),
	// "software" bila tidak. Test ini TIDAK menguji pemilihan itu sendiri
	// (lihat TestGetKeyProviderNeverHardFails) — ia hanya memastikan apa pun
	// hasilnya, nilai itu benar-benar terkirim ke server saat registrasi.
	switch gotBody["key_protection"] {
	case "tpm", "software":
	default:
		t.Fatalf("key_protection = %v, harus \"tpm\" atau \"software\"", gotBody["key_protection"])
	}
}

func TestRegisterDeviceFailureIsNotFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	SaveSetting(deviceIDSettingKey, "")
	if err := registerDeviceAt(srv.URL, "token-abc"); err == nil {
		t.Fatal("registerDeviceAt harus mengembalikan error saat server menolak")
	}
	if GetSetting(deviceIDSettingKey) != "" {
		t.Fatal("device_id tidak boleh diisi saat registrasi gagal")
	}
}
