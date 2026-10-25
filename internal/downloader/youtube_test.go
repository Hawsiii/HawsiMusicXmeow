package downloader

import (
	"ashokshau/tgmusic/internal/config"
	"ashokshau/tgmusic/internal/utils"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestBuildMeowStreamURL(t *testing.T) {
	previousURL, previousKey := config.MeowApiUrl, config.MeowApiKey
	config.MeowApiUrl = "https://music.example.test/api/"
	config.MeowApiKey = "key with symbols&spaces"
	t.Cleanup(func() {
		config.MeowApiUrl, config.MeowApiKey = previousURL, previousKey
	})

	streamURL, err := buildMeowStreamURL("video123", "audio", "128")
	if err != nil {
		t.Fatalf("buildMeowStreamURL returned an error: %v", err)
	}

	parsed, err := url.Parse(streamURL)
	if err != nil {
		t.Fatalf("returned URL is invalid: %v", err)
	}
	if parsed.Path != "/api/stream/video123" {
		t.Fatalf("expected the video ID in the stream path, got %q", parsed.Path)
	}
	if parsed.Query().Get("key") != config.MeowApiKey || parsed.Query().Get("type") != "audio" || parsed.Query().Get("quality") != "128" {
		t.Fatalf("unexpected Meow query parameters: %v", parsed.Query())
	}
}

func TestMeowVideoDownloadUses720p(t *testing.T) {
	if meowVideoQuality != "720" {
		t.Fatalf("Meow video quality = %q, want 720p", meowVideoQuality)
	}
	wantQualities := []string{"720", "480", "360", "240"}
	if len(meowVideoQualities) != len(wantQualities) {
		t.Fatalf("Meow quality fallback list = %v, want %v", meowVideoQualities, wantQualities)
	}
	for i, quality := range wantQualities {
		if meowVideoQualities[i] != quality {
			t.Fatalf("Meow quality fallback list = %v, want %v", meowVideoQualities, wantQualities)
		}
	}

	previousURL, previousKey := config.MeowApiUrl, config.MeowApiKey
	config.MeowApiUrl = "https://music.example.test/api/"
	config.MeowApiKey = "test-key"
	t.Cleanup(func() {
		config.MeowApiUrl, config.MeowApiKey = previousURL, previousKey
	})

	streamURL, err := buildMeowStreamURL("video123", "video", meowVideoQuality)
	if err != nil {
		t.Fatalf("buildMeowStreamURL returned an error: %v", err)
	}
	parsed, err := url.Parse(streamURL)
	if err != nil {
		t.Fatalf("returned URL is invalid: %v", err)
	}
	if got := parsed.Query().Get("quality"); got != "720" {
		t.Fatalf("video quality = %q, want 720", got)
	}
}

func TestMeowVideoFailuresAtEveryQualityInvokeFallback(t *testing.T) {
	var (
		mu        sync.Mutex
		qualities []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		qualities = append(qualities, r.URL.Query().Get("quality"))
		mu.Unlock()
		http.Error(w, "video unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	previousURL, previousKey, previousDir, previousExecutable := config.MeowApiUrl, config.MeowApiKey, config.DownloadsDir, ytDlpExecutable
	config.MeowApiUrl = server.URL
	config.MeowApiKey = "test-key"
	config.DownloadsDir = t.TempDir()
	ytDlpExecutable = t.TempDir() + "/missing-yt-dlp"
	t.Cleanup(func() {
		config.MeowApiUrl, config.MeowApiKey, config.DownloadsDir = previousURL, previousKey, previousDir
		ytDlpExecutable = previousExecutable
	})

	_, err := newYouTubeData("").downloadVideoWithRetry("video123")
	if err == nil || !strings.Contains(err.Error(), "yt-dlp video fallback") {
		t.Fatalf("downloadVideoWithRetry error = %v, want yt-dlp fallback error", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(qualities) != len(meowVideoQualities) {
		t.Fatalf("Meow qualities requested = %v, want %v", qualities, meowVideoQualities)
	}
	for i, quality := range meowVideoQualities {
		if qualities[i] != quality {
			t.Fatalf("Meow qualities requested = %v, want %v", qualities, meowVideoQualities)
		}
	}
}

func TestBuildYtDlpVideoArgs(t *testing.T) {
	previousProxy := config.Proxy
	config.Proxy = "http://proxy.example.test:8080"
	t.Cleanup(func() {
		config.Proxy = previousProxy
	})

	args := buildYtDlpVideoArgs("video123")
	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"bestvideo[height<=720]+bestaudio/best[height<=720]/best",
		"--merge-output-format mkv",
		"video123-fallback.%(ext)s",
		"--proxy http://proxy.example.test:8080",
		"https://www.youtube.com/watch?v=video123",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("yt-dlp args %v do not contain %q", args, expected)
		}
	}
}

func TestBuildYtDlpAudioArgs(t *testing.T) {
	previousProxy := config.Proxy
	config.Proxy = "http://proxy.example.test:8080"
	t.Cleanup(func() {
		config.Proxy = previousProxy
	})

	args := buildYtDlpAudioArgs("video123")
	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"-f bestaudio/best",
		"--extract-audio",
		"--audio-format mp3",
		"video123-audio-fallback.%(ext)s",
		"--proxy http://proxy.example.test:8080",
		"https://www.youtube.com/watch?v=video123",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("yt-dlp audio args %v do not contain %q", args, expected)
		}
	}
}

func TestValidateDecodedVideoFrameCount(t *testing.T) {
	tests := []struct {
		name       string
		frameCount int
		wantError  bool
	}{
		{name: "no frames", frameCount: 0, wantError: true},
		{name: "one frame", frameCount: 1, wantError: true},
		{name: "two frames", frameCount: 2},
		{name: "many frames", frameCount: 30},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDecodedVideoFrameCount(test.frameCount)
			if (err != nil) != test.wantError {
				t.Fatalf("validateDecodedVideoFrameCount(%d) error = %v, want error %t",
					test.frameCount, err, test.wantError)
			}
		})
	}
}

func TestYouTubeDownloadsFallbackWhenMeowIsNotConfigured(t *testing.T) {
	previousURL, previousKey := config.MeowApiUrl, config.MeowApiKey
	config.MeowApiUrl, config.MeowApiKey = "", ""
	previousExecutable := ytDlpExecutable
	ytDlpExecutable = t.TempDir() + "/missing-yt-dlp"
	t.Cleanup(func() {
		config.MeowApiUrl, config.MeowApiKey = previousURL, previousKey
		ytDlpExecutable = previousExecutable
	})

	for _, video := range []bool{false, true} {
		_, err := newYouTubeData("https://www.youtube.com/watch?v=abc123").downloadTrack(
			&utils.TrackInfo{Id: "abc123", Platform: utils.YouTube},
			video,
		)
		if err == nil || !strings.Contains(err.Error(), "could not run yt-dlp") {
			t.Fatalf("downloadTrack(video=%t) error = %v, want yt-dlp fallback error", video, err)
		}
	}

	_, err := (&apiData{}).downloadTrack(
		&utils.TrackInfo{
			Id:       "abc123",
			CdnURL:   "https://cdn.example.test/video.mp4",
			Platform: utils.YouTube,
		},
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "could not run yt-dlp") {
		t.Fatalf("YouTube API wrapper error = %v, want yt-dlp fallback error", err)
	}
}

func TestMediaDownloadsUseLongerTimeoutThanAPIRequests(t *testing.T) {
	if downloadTimeout <= defaultRequestTimeout || downloadClient.Timeout != downloadTimeout {
		t.Fatalf("media download timeout = %s, request timeout = %s", downloadClient.Timeout, defaultRequestTimeout)
	}
}

func TestParsePositiveDuration(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want float64
	}{
		{name: "valid", raw: "12.5", want: 12.5},
		{name: "zero", raw: "0", want: 0},
		{name: "negative", raw: "-1", want: 0},
		{name: "unknown", raw: "N/A", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parsePositiveDuration(test.raw); got != test.want {
				t.Fatalf("parsePositiveDuration(%q) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}
