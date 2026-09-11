package platforms

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	state "main/internal/core/models"
)

const (
	PlatformMeow     state.PlatformName = "Meow"
	meowMinValidSize                    = 10000
)

type MeowPlatform struct {
	name state.PlatformName
}

func init() {
	// Higher than Fallen API so Meow is tried first.
	Register(85, &MeowPlatform{
		name: PlatformMeow,
	})
}

func (m *MeowPlatform) Name() state.PlatformName {
	return m.name
}

func (m *MeowPlatform) CanGetTracks(query string) bool {
	// Meow is download-only. YouTube remains responsible for metadata/search.
	return false
}

func (m *MeowPlatform) GetTracks(
	_ string,
	_ bool,
) ([]*state.Track, error) {
	return nil, errors.New("meow is a download-only platform")
}

func (m *MeowPlatform) CanDownload(source state.PlatformName) bool {
	if strings.TrimSpace(config.MeowAPIURL) == "" ||
		strings.TrimSpace(config.MeowAPIKey) == "" {
		return false
	}

	return source == PlatformYouTube
}

func (m *MeowPlatform) Download(
	ctx context.Context,
	track *state.Track,
	_ *telegram.NewMessage,
) (string, error) {
	if track == nil || track.ID == "" {
		return "", errors.New("invalid track")
	}

	if path := findFile(track); path != "" {
		gologging.Debug("Meow: Download -> Cached File -> " + path)
		return path, nil
	}

	downloadCtx, cancel := meowDownloadContext(ctx)
	defer cancel()

	return downloadWithMeow(downloadCtx, track)
}

func downloadWithMeow(ctx context.Context, track *state.Track) (string, error) {
	if strings.TrimSpace(config.MeowAPIURL) == "" {
		return "", errors.New("meow API URL is not configured")
	}

	if strings.TrimSpace(config.MeowAPIKey) == "" {
		return "", errors.New("meow API key is not configured")
	}

	if track == nil || track.ID == "" {
		return "", errors.New("invalid track")
	}

	mediaType := "audio"
	quality := config.MeowAudioQuality
	ext := ".m4a"

	if track.Video {
		mediaType = "video"
		quality = config.MeowVideoQuality
		ext = ".mp4"
	}

	outputPath := getPath(track, ext)

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return "", fmt.Errorf("create downloads directory: %w", err)
	}

	apiURL := fmt.Sprintf(
		"%s/stream/%s?key=%s&type=%s&quality=%s",
		strings.TrimRight(config.MeowAPIURL, "/"),
		track.ID,
		config.MeowAPIKey,
		mediaType,
		quality,
	)

	gologging.InfoF(
		"Meow: Downloading %s (%s, quality=%s)",
		track.Title,
		mediaType,
		quality,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create Meow request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Meow request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Meow API returned HTTP %d", resp.StatusCode)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("create Meow output file: %w", err)
	}

	success := false
	defer func() {
		_ = file.Close()
		if !success {
			_ = os.Remove(outputPath)
		}
	}()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", fmt.Errorf("save Meow download: %w", err)
	}

	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close Meow output file: %w", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return "", fmt.Errorf("stat Meow output: %w", err)
	}

	if info.Size() < meowMinValidSize {
		return "", fmt.Errorf(
			"Meow returned an invalid file (%d bytes)",
			info.Size(),
		)
	}

	success = true

	gologging.InfoF(
		"Meow: Successfully downloaded %s",
		outputPath,
	)

	return outputPath, nil
}

func meowDownloadContext(parent context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok {
		return context.WithDeadline(context.Background(), deadline)
	}

	return context.WithTimeout(parent, 2*time.Minute)
}
