package dl

import (
	"ashokshau/tgmusic/config"
	"net/url"
	"strings"
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

func TestBuildYtdlpParamsDoesNotUseCookies(t *testing.T) {
	params := (&youTubeData{}).buildYtdlpParams("abc123", false)

	for i := 0; i < len(params)-1; i++ {
		if params[i] == "--cookies" {
			t.Fatalf("expected yt-dlp params to avoid cookies, got %v", params)
		}
	}

	if len(params) == 0 {
		t.Fatal("expected yt-dlp params to be generated")
	}

	if len(params) < 3 || params[len(params)-3] != "https://www.youtube.com/watch?v=abc123" {
		t.Fatalf("expected youtube URL near the end of params, got %v", params)
	}
}

func TestBuildYtdlpParamsPrefers720pVideo(t *testing.T) {
	params := (&youTubeData{}).buildYtdlpParams("abc123", true)

	var found bool
	for i := 0; i < len(params)-1; i++ {
		if params[i] == "-f" && strings.Contains(params[i+1], "bestvideo[height<=720]") {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected yt-dlp params to prefer 720p video, got %v", params)
	}
}
