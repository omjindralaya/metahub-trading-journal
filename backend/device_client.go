package backend

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// AppVersion dikirim di setiap sync dan tercatat di sync_audit server. Naikkan
// saat rilis: investigator memakainya untuk memisahkan "client lama yang wajar"
// dari "sesuatu yang mengaku client".
const AppVersion = "1.3.0"

// canonicalString HARUS identik byte-per-byte dengan devicesig.CanonicalString
// di metahub-api. Kontrak lintas-bahasa: mengubah satu sisi saja membuat SELURUH
// sync yang sah ditolak 401, dan gejalanya menyerupai serangan.
//
// Bentuk: sha256hex(body) \n timestamp \n nonce \n METHOD \n path
func canonicalString(body []byte, timestamp, nonce, method, path string) string {
	sum := sha256.Sum256(body)
	return strings.Join([]string{hexLower(sum[:]), timestamp, nonce, method, path}, "\n")
}

func hexLower(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = digits[c>>4]
		out[i*2+1] = digits[c&0x0f]
	}
	return string(out)
}

// newNonce membuat 16 byte acak (32 karakter hex). Nonce berulang ditolak server
// sebagai replay — WAJIB dari crypto/rand.
func newNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hexLower(b), nil
}

// signRequest memasang keempat header tanda tangan. Gagal (bukan menandatangani
// setengah jalan) bila device belum terdaftar: request setengah-ditandatangani
// ditolak server sebagai TIDAK SAH, lebih buruk daripada unsigned.
func signRequest(req *http.Request, body []byte, provider KeyProvider) error {
	deviceID := GetSetting(deviceIDSettingKey)
	if deviceID == "" {
		return fmt.Errorf("device belum terdaftar di cloud")
	}

	nonce, err := newNonce()
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	canonical := canonicalString(body, ts, nonce, req.Method, req.URL.Path)
	digest := sha256.Sum256([]byte(canonical))
	sig, err := provider.SignDigest(digest[:])
	if err != nil {
		return fmt.Errorf("gagal menandatangani request: %w", err)
	}

	req.Header.Set("X-Device-ID", deviceID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(sig))
	req.Header.Set("X-App-Version", AppVersion)
	return nil
}

// errDeviceKeyClaimed: server menolak registrasi karena kunci mesin ini sudah
// terdaftar atas nama AKUN LAIN (kode DEVICE_KEY_CLAIMED, HTTP 409). Bukan
// kegagalan sementara — mengulang tidak menembusnya. User yang sedang login tidak
// boleh menandatangani sync dengan kunci milik orang lain; ia beralih ke unsigned.
var errDeviceKeyClaimed = fmt.Errorf("device: kunci mesin ini sudah diklaim akun lain")

// deviceRegisterResponse mengikuti envelope pkg/response.
type deviceRegisterResponse struct {
	Success bool `json:"success"`
	Data    struct {
		DeviceID string `json:"device_id"`
	} `json:"data"`
	Error *CloudError `json:"error"`
}

// EnsureDeviceRegistered memastikan device ini terdaftar UNTUK USER YANG SEDANG
// LOGIN. Dipanggil setelah login cloud berhasil. Kegagalannya TIDAK fatal: server
// lama / jaringan putus tidak boleh membuat user gagal login. Sync-nya masuk
// sebagai `unsigned` selama Fase A.
//
// Sengaja TIDAK dilewati saat device_id lokal sudah terisi: device_id itu
// identitas MESIN dan bertahan lintas logout (lihat LogoutCloud), jadi user yang
// baru login mungkin BUKAN pemilik device tersebut di server. Registrasi ulang
// idempoten bagi pemilik (mengembalikan id yang sama), memulihkan device_id basi
// (server DB direset → device dibuat ulang), dan ditolak DEVICE_KEY_CLAIMED bagi
// user lain — yang kita tangani dengan beralih ke sync tanpa tanda tangan alih-alih
// menandatangani sebagai bukan-pemilik (yang akan ditolak 401 dan dulu memicu
// logout berulang).
func EnsureDeviceRegistered() {
	token := loadMetahubJWT()
	if token == "" {
		return
	}
	if err := registerDeviceAt(CloudAPIURL, token); err != nil {
		if errors.Is(err, errDeviceKeyClaimed) {
			// Kunci mesin ini milik akun lain. Buang device_id lokal supaya sync
			// berjalan unsigned (diterima server di Fase A), bukan ditandatangani
			// sebagai bukan-pemilik.
			SaveSetting(deviceIDSettingKey, "")
			log.Printf("EnsureDeviceRegistered: device mesin ini diklaim akun lain; sync berjalan unsigned")
			return
		}
		log.Printf("EnsureDeviceRegistered: registrasi device gagal (%v); sync berjalan unsigned bila device_id belum ada", err)
	}
}

// registerDeviceAt melakukan POST /devices/register dan menyimpan device_id.
// baseURL dipisah supaya bisa diuji terhadap httptest server.
func registerDeviceAt(baseURL, token string) error {
	provider, err := GetKeyProvider()
	if err != nil {
		return fmt.Errorf("gagal menyiapkan key device: %w", err)
	}
	pubB64, err := provider.PublicKeyBase64()
	if err != nil {
		return err
	}

	hostname, _ := os.Hostname()
	payload := map[string]string{
		"public_key":     pubB64,
		"device_name":    hostname,
		"os":             runtime.GOOS,
		"app_version":    AppVersion,
		"key_protection": provider.KeyProtection(),
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", strings.TrimRight(baseURL, "/")+"/devices/register", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("gagal menghubungi server: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var out deviceRegisterResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return fmt.Errorf("respons server tidak dikenali (status %d)", resp.StatusCode)
	}
	if !out.Success || resp.StatusCode != http.StatusOK {
		// Kunci mesin ini diklaim akun lain: kasus khusus yang pemanggilnya tangani
		// dengan beralih ke unsigned, bukan sekadar mencatat kegagalan.
		if out.Error != nil && out.Error.Code == "DEVICE_KEY_CLAIMED" {
			return errDeviceKeyClaimed
		}
		msg := out.Error.Text()
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		return fmt.Errorf("registrasi device ditolak: %s", msg)
	}
	if out.Data.DeviceID == "" {
		return fmt.Errorf("server tidak mengembalikan device_id")
	}

	return SaveSetting(deviceIDSettingKey, out.Data.DeviceID)
}
