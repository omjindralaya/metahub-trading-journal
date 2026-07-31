package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// withHungCloudServer mengarahkan CloudAPIURL ke server yang MENERIMA koneksi
// lalu diam selamanya — kegagalan yang paling berbahaya, karena TCP-nya sehat
// dan tidak ada error yang pernah muncul dengan sendirinya.
func withHungCloudServer(t *testing.T) {
	t.Helper()
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-block
	}))
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	oldClient := desktopAuthClient
	desktopAuthClient = &http.Client{Timeout: 200 * time.Millisecond}

	t.Cleanup(func() {
		desktopAuthClient = oldClient
		SetCloudAPIURL(oldURL)
		close(block)
		srv.Close()
	})
}

// Dulu jalur login Google memakai http.Get/http.Post telanjang, yaitu
// http.DefaultClient — yang TIDAK punya timeout. Server yang menggantung
// membuat panggilan ini tidak pernah kembali, sehingga timeout 9 menit maupun
// tombol "Batal" (CancelGoogleLogin) tidak pernah sempat terbaca: keduanya
// hanya diperiksa di antara dua polling.
func TestPollDesktopSession_FailsInsteadOfHanging(t *testing.T) {
	withHungCloudServer(t)

	done := make(chan error, 1)
	go func() {
		_, _, err := pollDesktopSession(context.Background(), "sesi-apa-saja")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("server menggantung harus menghasilkan error, bukan sukses")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pollDesktopSession menggantung: HTTP client-nya tanpa timeout")
	}
}

func TestCreateDesktopSession_FailsInsteadOfHanging(t *testing.T) {
	withHungCloudServer(t)

	done := make(chan error, 1)
	go func() {
		_, _, err := createDesktopSession(context.Background())
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("server menggantung harus menghasilkan error, bukan sukses")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("createDesktopSession menggantung: HTTP client-nya tanpa timeout")
	}
}

// Membatalkan ctx harus MENGHENTIKAN request yang sedang berjalan, bukan hanya
// diperiksa setelah request itu selesai dengan sendirinya. Ini yang membuat
// tombol "Batal" terasa seketika.
func TestPollDesktopSession_HonorsContextCancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-block
	}))
	oldURL := CloudAPIURL
	SetCloudAPIURL(srv.URL)
	t.Cleanup(func() {
		SetCloudAPIURL(oldURL)
		close(block)
		srv.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := pollDesktopSession(ctx, "sesi-apa-saja")
		done <- err
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("pembatalan harus menghasilkan error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pollDesktopSession mengabaikan pembatalan ctx")
	}
}
