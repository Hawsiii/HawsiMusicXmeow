package downloader

import (
	"ashokshau/tgmusic/internal/config"
	"ashokshau/tgmusic/internal/utils"
	"math"
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

func TestVideoStreamCoversMediaDuration(t *testing.T) {
	tests := []struct {
		name          string
		videoDuration float64
		mediaDuration float64
		want          bool
	}{
		{name: "matching durations", videoDuration: 180, mediaDuration: 180, want: true},
		{name: "small container variance", videoDuration: 178, mediaDuration: 180, want: true},
		{name: "video ends much earlier", videoDuration: 1, mediaDuration: 180, want: false},
		{name: "unknown video duration", videoDuration: 0, mediaDuration: 180, want: true},
		{name: "unknown media duration", videoDuration: 180, mediaDuration: 0, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := videoStreamCoversDuration(test.videoDuration, test.mediaDuration); got != test.want {
				t.Fatalf("videoStreamCoversDuration(%v, %v) = %t, want %t",
					test.videoDuration, test.mediaDuration, got, test.want)
			}
		})
	}
}

func TestParseFrameRate(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want float64
	}{
		{name: "integer frame rate", raw: "30/1", want: 30},
		{name: "fractional frame rate", raw: "30000/1001", want: 30000.0 / 1001},
		{name: "unknown frame rate", raw: "0/0", want: 0},
		{name: "invalid frame rate", raw: "N/A", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parseFrameRate(test.raw); math.Abs(got-test.want) > 0.0001 {
				t.Fatalf("parseFrameRate(%q) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}

func TestDecodedVideoFramesCoverInitialWindow(t *testing.T) {
	tests := []struct {
		name          string
		frameCount    int
		frameRate     float64
		mediaDuration float64
		want          bool
	}{
		{name: "full five second window", frameCount: 150, frameRate: 30, mediaDuration: 180, want: true},
		{name: "video ends after one second", frameCount: 30, frameRate: 30, mediaDuration: 180, want: false},
		{name: "short media duration", frameCount: 30, frameRate: 30, mediaDuration: 1, want: true},
		{name: "no frames", frameCount: 0, frameRate: 30, mediaDuration: 180, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := decodedVideoFramesCoverWindow(test.frameCount, test.frameRate, test.mediaDuration); got != test.want {
				t.Fatalf("decodedVideoFramesCoverWindow(%d, %v, %v) = %t, want %t",
					test.frameCount, test.frameRate, test.mediaDuration, got, test.want)
			}
		})
	}
}
