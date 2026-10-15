package vc

import (
	"strings"
	"testing"
)

func TestGetMediaDescriptionUsesSharpVideoScaling(t *testing.T) {
	description := getMediaDescription("sample.mp4", true, "")
	if description.Camera == nil ||
		!strings.Contains(description.Camera.Input, "flags=lanczos") ||
		!strings.Contains(description.Camera.Input, "scale=1280:720:force_original_aspect_ratio=decrease") ||
		!strings.Contains(description.Camera.Input, "pad=1280:720") ||
		strings.Contains(description.Camera.Input, "crop=") {
		t.Fatalf("expected video input to use Lanczos scaling, got %#v", description.Camera)
	}
}