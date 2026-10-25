package downloader

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadFileUsesMediaTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte("video"))
	}))
	defer server.Close()

	originalTimeout := client.Timeout
	client.Timeout = time.Millisecond
	t.Cleanup(func() {
		client.Timeout = originalTimeout
	})

	path := filepath.Join(t.TempDir(), "video.mp4")
	got, err := downloadFile(server.URL, path, true)
	if err != nil {
		t.Fatalf("downloadFile returned an error despite the longer media timeout: %v", err)
	}
	if got != path {
		t.Fatalf("downloadFile path = %q, want %q", got, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("downloaded file is not present: %v", err)
	}
}

func TestDownloadFileFollowsMultipleMediaRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/redirect-1", http.StatusFound)
		case "/redirect-1":
			http.Redirect(w, r, "/redirect-2", http.StatusFound)
		case "/redirect-2":
			http.Redirect(w, r, "/media", http.StatusFound)
		case "/media":
			_, _ = w.Write([]byte("video"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "video.mp4")
	got, err := downloadFile(server.URL+"/start", path, true)
	if err != nil {
		t.Fatalf("downloadFile returned an error after multiple media redirects: %v", err)
	}
	if got != path {
		t.Fatalf("downloadFile path = %q, want %q", got, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("downloaded file is not present: %v", err)
	}
}
