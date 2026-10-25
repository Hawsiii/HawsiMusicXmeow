package calls

import (
	"strings"
	"testing"
)

func TestGetMediaDescriptionUsesSharpVideoScaling(t *testing.T) {
	description := getMediaDescription("sample.mp4", true, 0, "")
	if description.Camera == nil ||
		!strings.Contains(description.Camera.Input, "ffmpeg -hide_banner -re -i ") ||
		!strings.Contains(description.Camera.Input, "flags=lanczos") ||
		!strings.Contains(description.Camera.Input, "scale=1280:720:force_original_aspect_ratio=decrease") ||
		!strings.Contains(description.Camera.Input, "pad=1280:720") ||
		strings.Contains(description.Camera.Input, "crop=") {
		t.Fatalf("expected local video input to be paced and scaled, got %#v", description.Camera)
	}
	if description.Microphone == nil || !strings.Contains(description.Microphone.Input, "ffmpeg -re -i ") {
		t.Fatalf("expected local audio input to be paced, got %#v", description.Microphone)
	}
}

func TestGetMediaDescriptionPacesDirectVideoURLs(t *testing.T) {
	description := getMediaDescription("https://example.com/video.mp4", true, 0, "")
	if description.Camera == nil ||
		!strings.Contains(description.Camera.Input, "-reconnect 1") ||
		!strings.Contains(description.Camera.Input, " -re -i ") {
		t.Fatalf("expected direct remote video input to reconnect and be paced, got %#v", description.Camera)
	}

	googleVideoURL := "https://video-downloads.googleusercontent.com/opaque-signed-id?expire=123"
	description = getMediaDescription(googleVideoURL, true, 0, "")
	if description.Camera == nil || !strings.Contains(description.Camera.Input, " -re -i ") {
		t.Fatalf("expected extensionless signed video URL to be paced, got %#v", description.Camera)
	}

	manifest := getMediaDescription("https://example.com/live.m3u8?token=example", true, 0, "")
	if manifest.Camera == nil ||
		!strings.Contains(manifest.Camera.Input, "-reconnect 1") ||
		strings.Contains(manifest.Camera.Input, " -re -i ") {
		t.Fatalf("expected live manifest input to reconnect without file pacing, got %#v", manifest.Camera)
	}
}

func TestGetMediaDescriptionAppliesSpeedFilters(t *testing.T) {
	description := getMediaDescription("sample.mp4", true, 0, "-filter:v setpts=0.5*PTS -filter:a atempo=2.0")
	if description.Microphone == nil || !strings.Contains(description.Microphone.Input, `-af "atempo=2.0"`) {
		t.Fatalf("expected audio speed filter, got %#v", description.Microphone)
	}
	if description.Camera == nil ||
		!strings.Contains(description.Camera.Input, `-vf "setpts=0.5*PTS,scale=1280:720`) ||
		strings.Contains(description.Camera.Input, "-filter:a") {
		t.Fatalf("expected video speed filter combined with scaling, got %#v", description.Camera)
	}
}
