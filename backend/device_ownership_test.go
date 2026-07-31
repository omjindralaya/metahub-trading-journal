package backend

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Bug inti "login gmail lalu jadi guest": device_id itu identitas MESIN dan
// bertahan lintas logout, jadi saat AKUN LAIN login, EnsureDeviceRegistered TIDAK
// boleh melewatkan verifikasi hanya karena device_id lokal sudah terisi. Ia harus
// tetap mendaftar ulang untuk user yang sedang login.
func TestEnsureDeviceRegistered_ReverifiesEvenWhenDeviceIDPresent(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":{"device_id":"99999999-9999-9999-9999-999999999999"}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	SaveSetting("metahub_jwt", "tok")
	// Device dari sesi user SEBELUMNYA masih tersimpan.
	SaveSetting(deviceIDSettingKey, "11111111-1111-1111-1111-111111111111")

	EnsureDeviceRegistered()

	if hits == 0 {
		t.Fatal("EnsureDeviceRegistered melewatkan verifikasi padahal device_id sudah ada — inilah akar bug ganti-akun")
	}
	if got := GetSetting(deviceIDSettingKey); got != "99999999-9999-9999-9999-999999999999" {
		t.Fatalf("device_id tidak diperbarui ke device milik user saat ini: %q", got)
	}
}

// Kunci mesin ini milik akun lain (DEVICE_KEY_CLAIMED): user saat ini tidak boleh
// menandatangani dengannya. device_id lokal dibuang → sync berjalan unsigned
// (diterima Fase A), bukan ditandatangani sebagai bukan-pemilik lalu ditolak 401.
func TestEnsureDeviceRegistered_KeyClaimedByOther_FallsBackToUnsigned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"success":false,"error":{"code":"DEVICE_KEY_CLAIMED","message":"Public key ini sudah terdaftar untuk akun lain"}}`))
	}))
	defer srv.Close()
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	defer SetCloudAPIURL(oldURL)

	SaveSetting("metahub_jwt", "tok")
	SaveSetting(deviceIDSettingKey, "11111111-1111-1111-1111-111111111111")

	EnsureDeviceRegistered()

	if got := GetSetting(deviceIDSettingKey); got != "" {
		t.Fatalf("device_id harus dibuang saat kunci diklaim akun lain (agar sync unsigned), masih: %q", got)
	}
}
