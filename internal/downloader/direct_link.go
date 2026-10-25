/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package downloader

import (
	"ashokshau/tgmusic/internal/utils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type directLink struct {
	query string
}

func newDirectLink(query string) *directLink {
	return &directLink{query: query}
}

func (d *directLink) isValid() bool {
	return strings.HasPrefix(d.query, "http://") || strings.HasPrefix(d.query, "https://")
}

func (d *directLink) getInfo() (*utils.PlatformTracks, error) {
	if !d.isValid() {
		return nil, errors.New("invalid url")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-rw_timeout", "20000000",
		"-analyzeduration", "10000000",
		"-probesize", "10000000",
		"-print_format", "json",
		"-show_format",
		d.query,
	)

	output, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			if !isDeferredProbeURL(d.query) {
				return nil, fmt.Errorf("timed out probing direct media link")
			}
		}
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && !isDeferredProbeURL(d.query) {
			details := strings.TrimSpace(string(exitErr.Stderr))
			details = strings.ReplaceAll(details, d.query, "[URL]")
			if len(details) > 500 {
				details = details[:500] + "..."
			}
			if details != "" {
				return nil, fmt.Errorf("invalid or unplayable link (ffprobe exit code %d): %s", exitErr.ExitCode(), details)
			}
			return nil, fmt.Errorf("invalid or unplayable link (ffprobe exit code %d)", exitErr.ExitCode())
		}
		if !isDeferredProbeURL(d.query) {
			return nil, fmt.Errorf("invalid or unplayable link: %w", err)
		}
	}

	var info utils.FFProbeFormat
	if len(output) > 0 {
		if err = json.Unmarshal(output, &info); err != nil {
			return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
		}
	}

	var duration int32
	if info.Format.Duration != "" {
		if d, err := strconv.ParseFloat(info.Format.Duration, 64); err == nil {
			duration = int32(d)
		}
	}

	title := info.Format.Tags.Title
	if title == "" {
		title = directLinkTitle(d.query)
		if title == "" {
			title = "Direct Link"
		}
	}

	const maxTitleLength = 30
	if len(title) > maxTitleLength {
		title = title[:maxTitleLength-3] + "..."
	}

	track := utils.GetUrlTrack{
		Title:    title,
		Duration: duration,
		Url:      d.query,
		Id:       d.query,
		Platform: utils.DirectLink,
	}

	return &utils.PlatformTracks{Results: []utils.GetUrlTrack{track}}, nil
}

func (d *directLink) search() (*utils.PlatformTracks, error) {
	return d.getInfo()
}

func (d *directLink) getTrack() (*utils.TrackInfo, error) {
	info, err := d.getInfo()
	if err != nil {
		return nil, err
	}

	if len(info.Results) == 0 {
		return nil, errors.New("no track found")
	}

	track := info.Results[0]
	return &utils.TrackInfo{
		Id:       track.Id,
		URL:      track.Url,
		CdnURL:   track.Url,
		Platform: track.Platform,
	}, nil
}

func (d *directLink) downloadTrack(_ *utils.TrackInfo, _ bool) (string, error) {
	if isDeferredProbeURL(d.query) {
		return d.query, nil
	}

	localPath, err := downloadFile(d.query, "", false)
	if err != nil {
		slog.Warn("Direct link download failed, falling back to remote stream", "url", d.query, "error", err)
		return d.query, nil
	}
	return localPath, nil
}

func isDeferredProbeURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	return host == "googleusercontent.com" || strings.HasSuffix(host, ".googleusercontent.com")
}

func directLinkTitle(rawURL string) string {
	if isDeferredProbeURL(rawURL) {
		return "Google video"
	}

	parts := strings.Split(rawURL, "/")
	if len(parts) == 0 {
		return ""
	}
	title := parts[len(parts)-1]
	title = strings.SplitN(title, "?", 2)[0]
	title = strings.SplitN(title, "#", 2)[0]
	title, _ = url.QueryUnescape(title)
	return title
}

func (d *directLink) getRecommendations(_ context.Context, _ string) (*utils.PlatformTracks, error) {
	return nil, errors.New("recommendations are not supported for direct links")
}
