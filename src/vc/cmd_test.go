package vc

import (
	"strings"
	"testing"
)

func TestGetMediaDescriptionUsesSharpVideoScaling(t *testing.T) {
	description := getMediaDescription("sample.mp4", true, "")
	if description.Camera == nil || !strings.Contains(description.Camera.Input, "flags=lanczos") {
		t.Fatalf("expected video input to use Lanczos scaling, got %#v", description.Camera)
	}
}