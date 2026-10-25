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

func TestMeowVideoQualityFallbackOrder(t *testing.T) {
	want := []string{"720", "480", "360", "240"}
	if len(meowVideoQualities) != len(want) {
		t.Fatalf("expected %d Meow video quality attempts, got %v", len(want), meowVideoQualities)
	}
	for i := range want {
		if meowVideoQualities[i] != want[i] {
			t.Fatalf("Meow video quality %d = %q, want %q", i, meowVideoQualities[i], want[i])
		}
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
