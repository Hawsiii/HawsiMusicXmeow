package calls

import (
	"strings"
	"testing"
)

func TestGetMediaDescriptionUsesSharpVideoScaling(t *testing.T) {
	description := getMediaDescription("sample.mp4", true, 0, "")
	if description.Camera == nil ||
		!strings.Contains(description.Camera.Input, "flags=lanczos") ||
		!strings.Contains(description.Camera.Input, "scale=1280:720:force_original_aspect_ratio=decrease") ||
		!strings.Contains(description.Camera.Input, "pad=1280:720") ||
		strings.Contains(description.Camera.Input, "crop=") {
		t.Fatalf("expected video input to use Lanczos scaling, got %#v", description.Camera)
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
