package downloader

import (
	"ashokshau/tgmusic/internal/config"
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

func TestBuildYtdlpParamsDoesNotUseCookieFiles(t *testing.T) {
	params := (&youTubeData{}).buildYtdlpParams("abc123", false)

	for i := 0; i < len(params)-1; i++ {
		if params[i] == "--cookies" {
			t.Fatalf("expected yt-dlp params to avoid cookies, got %v", params)
		}
	}

	if len(params) == 0 {
		t.Fatal("expected yt-dlp params to be generated")
	}
	if !containsArgPair(params, "--js-runtimes", "deno:/usr/local/bin/deno") {
		t.Fatalf("expected yt-dlp to use the installed Deno runtime, got %v", params)
	}
	if !containsArgPair(params, "--remote-components", "ejs:github") {
		t.Fatalf("expected yt-dlp to load the YouTube EJS challenge solver, got %v", params)
	}

	if len(params) < 3 || params[len(params)-3] != "https://www.youtube.com/watch?v=abc123" {
		t.Fatalf("expected youtube URL near the end of params, got %v", params)
	}
}

func TestBuildYtdlpParamsPrefersCombined720pVideo(t *testing.T) {
	params := (&youTubeData{}).buildYtdlpParams("abc123", true)

	var found bool
	for i := 0; i < len(params)-1; i++ {
		if params[i] == "-f" &&
			strings.Contains(params[i+1], "bestvideo[height<=720]") &&
			strings.Contains(params[i+1], "+bestaudio") &&
			strings.Contains(params[i+1], "best[height<=720]") {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected yt-dlp params to prefer combined video up to 720p, got %v", params)
	}
}

func TestBuildYtdlpFallbackParamsBroadensFormatSelection(t *testing.T) {
	params := (&youTubeData{}).buildYtdlpFallbackParams("abc123", false)
	joined := strings.Join(params, " ")

	if !strings.Contains(joined, "ba/b") {
		t.Fatalf("expected fallback audio format, got %v", params)
	}
	if !strings.Contains(joined, "youtube:player_client=android,web") {
		t.Fatalf("expected alternate YouTube clients, got %v", params)
	}
}

func TestBuildYtdlpVideoFallbackKeepsAudioAndVideoTogether(t *testing.T) {
	params := (&youTubeData{}).buildYtdlpFallbackParams("abc123", true)
	for i := 0; i < len(params)-1; i++ {
		if params[i] == "-f" {
			if params[i+1] != "bestvideo[height<=720]+bestaudio/best[height<=720]" {
				t.Fatalf("expected fallback to select separate or combined A/V capped at 720p, got %q", params[i+1])
			}
			return
		}
	}
	t.Fatalf("expected a video format selector in fallback params, got %v", params)
}

func containsArgPair(args []string, name, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name && args[i+1] == value {
			return true
		}
	}
	return false
}
