package downloader

import "testing"

func TestGoogleVideoDownloadLinksDeferMetadataProbe(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "Google video download host",
			url:  "https://video-downloads.googleusercontent.com/opaque-signed-id",
			want: true,
		},
		{
			name: "other Googleusercontent subdomain",
			url:  "https://storage.googleapis.googleusercontent.com/file.mp4",
			want: true,
		},
		{
			name: "lookalike host",
			url:  "https://googleusercontent.com.attacker.example/file.mp4",
			want: false,
		},
		{
			name: "other media host",
			url:  "https://cdn.example.test/video.mp4",
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isDeferredProbeURL(test.url); got != test.want {
				t.Fatalf("isDeferredProbeURL(%q) = %t, want %t", test.url, got, test.want)
			}
		})
	}
}

func TestGoogleVideoDownloadLinkStreamsWithoutDownloading(t *testing.T) {
	rawURL := "https://video-downloads.googleusercontent.com/opaque-signed-id"
	path, err := newDirectLink(rawURL).downloadTrack(nil, true)
	if err != nil {
		t.Fatalf("downloadTrack returned an error: %v", err)
	}
	if path != rawURL {
		t.Fatalf("downloadTrack returned %q, want original URL %q", path, rawURL)
	}
	if title := directLinkTitle(rawURL); title != "Google video" {
		t.Fatalf("directLinkTitle returned %q, want %q", title, "Google video")
	}
}
