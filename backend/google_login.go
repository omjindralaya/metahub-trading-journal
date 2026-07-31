package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// desktopAuthClient adalah client untuk kedua panggilan login Google. Ia HARUS
// punya timeout sendiri: http.Get/http.Post telanjang memakai http.DefaultClient,
// yang tidak punya batas waktu sama sekali. Server yang menerima koneksi lalu
// diam membuat panggilan itu tidak pernah kembali — dan karena ctx hanya
// diperiksa DI ANTARA dua polling, timeout 9 menit maupun tombol "Batal" tidak
// pernah sempat terbaca. Variabel supaya test bisa memperkecilnya.
var desktopAuthClient = &http.Client{Timeout: 20 * time.Second}

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
	// ctx dibuat SEBELUM request pertama supaya pembuatan sesi pun bisa dibatalkan
	// dan ikut terkena batas waktu — bukan hanya polling sesudahnya.
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

	sessionID, authURL, err := createDesktopSession(ctx)
	if err != nil {
		return err
	}
	openBrowser(authURL)

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
			loginResp, pending, perr := pollDesktopSession(ctx, sessionID)
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
func createDesktopSession(ctx context.Context) (sessionID string, authURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, "POST", CloudAPIURL+"/auth/desktop/session", bytes.NewReader(nil))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := desktopAuthClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("connection failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
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
func pollDesktopSession(ctx context.Context, sessionID string) (loginResp *LoginResponse, pending bool, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", CloudAPIURL+"/auth/desktop/session/"+sessionID, nil)
	if err != nil {
		return nil, false, err
	}

	resp, err := desktopAuthClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("connection failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
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
