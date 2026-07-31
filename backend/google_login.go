package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

// desktopAuthURL adalah halaman web yang menyelesaikan login Google untuk
// desktop (lihat metahub-web DesktopAuthPage). Variabel supaya deployment bisa
// mengarahkannya tanpa rebuild, sama seperti upgradeURL.
var desktopAuthURL = "https://metahub.id/desktop-auth"

// SetDesktopAuthURL dipakai konfigurasi lingkungan / test.
func SetDesktopAuthURL(u string) { desktopAuthURL = u }

// googleLoginPollInterval / googleLoginTimeout membatasi lama menunggu tab
// browser menyelesaikan login Google. Timeout mendekati tapi tetap di bawah
// desktopSessionTTL milik server (10 menit), supaya polling berhenti sendiri
// dengan pesan yang jelas alih-alih menembak sesi yang server sudah buang.
const (
	googleLoginPollInterval = 2 * time.Second
	googleLoginTimeout      = 9 * time.Minute
)

// googleLoginCancel memungkinkan tombol "Batal" di UI menghentikan polling
// yang sedang berjalan. Package-level karena hanya satu login Google bisa
// berlangsung dalam satu waktu di satu aplikasi desktop.
var (
	googleLoginMu     sync.Mutex
	googleLoginCancel context.CancelFunc
)

// CancelGoogleLogin menghentikan LoginWithGoogleDesktop yang sedang polling,
// bila ada. Aman dipanggil walau tidak ada login yang berjalan.
func CancelGoogleLogin() {
	googleLoginMu.Lock()
	defer googleLoginMu.Unlock()
	if googleLoginCancel != nil {
		googleLoginCancel()
	}
}

// LoginWithGoogleDesktop membuat sesi login di server, membuka browser lewat
// openBrowser (App.go yang menyediakannya — paket ini sengaja tidak mengimpor
// Wails runtime supaya tetap bisa diuji tanpa jendela aplikasi), lalu polling
// sampai tab browser menyelesaikannya, dibatalkan (CancelGoogleLogin), atau
// timeout.
func LoginWithGoogleDesktop(openBrowser func(url string)) error {
	sessionID, authURL, err := createDesktopSession()
	if err != nil {
		return err
	}
	openBrowser(authURL)

	ctx, cancel := context.WithTimeout(context.Background(), googleLoginTimeout)
	googleLoginMu.Lock()
	googleLoginCancel = cancel
	googleLoginMu.Unlock()
	defer func() {
		cancel()
		googleLoginMu.Lock()
		googleLoginCancel = nil
		googleLoginMu.Unlock()
	}()

	ticker := time.NewTicker(googleLoginPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return fmt.Errorf("login dibatalkan")
			}
			return fmt.Errorf("login Google kedaluwarsa, coba lagi")
		case <-ticker.C:
			loginResp, pending, perr := pollDesktopSession(sessionID)
			if perr != nil {
				return perr
			}
			if pending {
				continue
			}
			applyLoginResponse(*loginResp)
			return nil
		}
	}
}

type desktopSessionCreateResponse struct {
	Success bool `json:"success"`
	Data    struct {
		SessionID string `json:"session_id"`
	} `json:"data"`
	Error *CloudError `json:"error"`
}

// createDesktopSession minta server membuatkan sesi baru dan mengembalikan
// id-nya plus URL browser yang membawanya.
func createDesktopSession() (sessionID string, authURL string, err error) {
	resp, err := http.Post(CloudAPIURL+"/auth/desktop/session", "application/json", nil)
	if err != nil {
		return "", "", fmt.Errorf("connection failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var out desktopSessionCreateResponse
	if jerr := json.Unmarshal(body, &out); jerr != nil {
		return "", "", fmt.Errorf("respons server tidak dikenali (status %d)", resp.StatusCode)
	}
	if !out.Success || resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("gagal memulai login Google: %s", out.Error.Text())
	}

	return out.Data.SessionID, fmt.Sprintf("%s?session=%s", desktopAuthURL, out.Data.SessionID), nil
}

type desktopSessionGetResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Status string           `json:"status"`
		Token  string           `json:"token"`
		User   CloudUserProfile `json:"user"`
	} `json:"data"`
	Error *CloudError `json:"error"`
}

// pollDesktopSession mengecek status sesi sekali. pending=true berarti tab
// browser belum selesai — bukan error, pemanggil harus coba lagi nanti.
func pollDesktopSession(sessionID string) (loginResp *LoginResponse, pending bool, err error) {
	resp, err := http.Get(CloudAPIURL + "/auth/desktop/session/" + sessionID)
	if err != nil {
		return nil, false, fmt.Errorf("connection failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var out desktopSessionGetResponse
	if jerr := json.Unmarshal(body, &out); jerr != nil {
		return nil, false, fmt.Errorf("respons server tidak dikenali (status %d)", resp.StatusCode)
	}
	if !out.Success || resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("sesi login berakhir: %s", out.Error.Text())
	}
	if out.Data.Status != "completed" {
		return nil, true, nil
	}

	lr := &LoginResponse{Success: true}
	lr.Data.Token = out.Data.Token
	lr.Data.User = out.Data.User
	return lr, false, nil
}
