package downloader

import (
	"ashokshau/tgmusic/internal/config"
	"ashokshau/tgmusic/internal/utils"
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

func TestMeowVideoDownloadUses720p(t *testing.T) {
	if meowVideoQuality != "720" {
		t.Fatalf("Meow video quality = %q, want 720p", meowVideoQuality)
	}
	if len(videoRetryDelays) != videoRetryCount {
		t.Fatalf("expected %d attempts at the requested quality, got %d", videoRetryCount, len(videoRetryDelays))
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

func TestYouTubeDownloadsRequireMeowAPI(t *testing.T) {
	previousURL, previousKey := config.MeowApiUrl, config.MeowApiKey
	config.MeowApiUrl, config.MeowApiKey = "", ""
	t.Cleanup(func() {
		config.MeowApiUrl, config.MeowApiKey = previousURL, previousKey
	})

	for _, video := range []bool{false, true} {
		_, err := newYouTubeData("https://www.youtube.com/watch?v=abc123").downloadTrack(
			&utils.TrackInfo{Id: "abc123", Platform: utils.YouTube},
			video,
		)
		if err == nil || !strings.Contains(err.Error(), "Meow API is required") {
			t.Fatalf("downloadTrack(video=%t) error = %v, want missing Meow API error", video, err)
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
	if err == nil || !strings.Contains(err.Error(), "Meow API is required") {
		t.Fatalf("YouTube API wrapper error = %v, want missing Meow API error instead of CDN fallback", err)
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
