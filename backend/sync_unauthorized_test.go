package backend

import "testing"

// Penolakan device-signature (JWT sah, device-nya yang ditolak) TIDAK boleh
// membuat klien logout. Logout di sini menghancurkan sesi yang sah dan memicu
// loop login-ulang tiap 5 detik — persis bug "login gmail lalu jadi guest".
// Sebagai gantinya device_id lokal dibuang supaya tick berikutnya unsigned.
func TestHandleSyncUnauthorized_DeviceSignature_DoesNotLogout(t *testing.T) {
	SaveSetting("metahub_jwt", "tok")
	SaveSetting(deviceIDSettingKey, "11111111-1111-1111-1111-111111111111")

	body := []byte(`{"success":false,"error":{"code":"DEVICE_SIGNATURE_INVALID","message":"Tanda tangan device tidak sah"}}`)
	if err := handleSyncUnauthorized(body); err == nil {
		t.Fatal("harus mengembalikan error")
	}

	if GetSetting("metahub_jwt") != "tok" {
		t.Fatal("penolakan device-signature TIDAK boleh logout — sesi JWT masih sah")
	}
	if GetSetting(deviceIDSettingKey) != "" {
		t.Fatal("device_id harus dibuang agar tick berikutnya dikirim tanpa tanda tangan")
	}
}

// Sesi yang benar-benar berakhir (token tidak sah/kedaluwarsa, kode UNAUTHORIZED)
// TETAP harus logout.
func TestHandleSyncUnauthorized_SessionExpiry_LogsOut(t *testing.T) {
	SaveSetting("metahub_jwt", "tok")
	SaveSetting(deviceIDSettingKey, "11111111-1111-1111-1111-111111111111")

	body := []byte(`{"success":false,"error":{"code":"UNAUTHORIZED","message":"Session expired"}}`)
	if err := handleSyncUnauthorized(body); err == nil {
		t.Fatal("harus mengembalikan error")
	}

	if GetSetting("metahub_jwt") != "" {
		t.Fatal("sesi yang berakhir harus logout (JWT dibersihkan)")
	}
}

// Body yang tak dikenali/ kosong: fail closed ke logout. Kita tak bisa memastikan
// ini bukan sesi berakhir, dan logout adalah jawaban yang aman untuk 401 tak jelas.
func TestHandleSyncUnauthorized_UnknownBody_LogsOut(t *testing.T) {
	SaveSetting("metahub_jwt", "tok")

	if err := handleSyncUnauthorized([]byte("bukan json")); err == nil {
		t.Fatal("harus mengembalikan error")
	}
	if GetSetting("metahub_jwt") != "" {
		t.Fatal("401 tak jelas harus fail-closed ke logout")
	}
}
