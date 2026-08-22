/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package dl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ashokshau/tgmusic/config"
	"ashokshau/tgmusic/src/utils"
)

// youTubeData provides an interface for fetching track and playlist information from YouTube.
type youTubeData struct {
	Query    string
	ApiUrl   string
	APIKey   string
	Patterns map[string]*regexp.Regexp
}

type ytDlpInfo struct {
	URL       string      `json:"url"`
	Title     string      `json:"title"`
	Thumbnail string      `json:"thumbnail"`
	Duration  float64     `json:"duration"`
	IsLive    bool        `json:"is_live"`
	Formats   []ytFormat  `json:"formats"`
	Entries   []ytDlpInfo `json:"entries"`
}

type ytFormat struct {
	URL string `json:"url"`
}

var youtubePatterns = map[string]*regexp.Regexp{
	"youtube":   regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtube\.com/.*`),
	"youtu_be":  regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtu\.be/.*`),
	"yt_music":  regexp.MustCompile(`(?i)^(?:https?://)?music\.youtube\.com/.*`),
	"yt_shorts": regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtube\.com/shorts/.*`),
}

// newYouTubeData initializes a youTubeData instance with pre-compiled regex patterns and a cleaned query.
func newYouTubeData(query string) *youTubeData {
	return &youTubeData{
		Query:    strings.TrimSpace(query),
		ApiUrl:   strings.TrimRight(config.ApiUrl, "/"),
		APIKey:   config.ApiKey,
		Patterns: youtubePatterns,
	}
}
func (y *youTubeData) isValid() bool {
	if y.Query == "" {
		slog.Info("The query or patterns are empty.")
		return false
	}

	for _, pattern := range y.Patterns {
		if pattern.MatchString(y.Query) {
			return true
		}
	}
	return false
}

func (y *youTubeData) getInfo() (utils.PlatformTracks, error) {
	if !y.isValid() {
		return utils.PlatformTracks{}, errors.New("the provided URL is invalid or the platform is not supported")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	y.Query = normalizeYouTubeURL(y.Query)
	videoID := extractVideoID(y.Query)
	playlistID := extractPlaylistID(y.Query)

	switch {
	case playlistID != "":
		if strings.HasPrefix(playlistID, "RD") {
			return getYouTubeMixPlaylist(ctx, playlistID)
		}
		return getYouTubePlaylist(ctx, playlistID)

	case videoID != "":
		return getYouTubeVideo(ctx, videoID)
	}

	return utils.PlatformTracks{}, errors.New("no video or playlist results were found")
}

func (y *youTubeData) search() (utils.PlatformTracks, error) {
	tracks, err := searchYouTube(y.Query, 5)
	if err != nil {
		return utils.PlatformTracks{}, err
	}

	if len(tracks) == 0 {
		return utils.PlatformTracks{}, errors.New("no video results were found")
	}

	return utils.PlatformTracks{Results: tracks}, nil
}

func (y *youTubeData) getTrack() (utils.TrackInfo, error) {
	if y.Query == "" {
		return utils.TrackInfo{}, errors.New("the query is empty")
	}

	if !y.isValid() {
		return utils.TrackInfo{}, errors.New("the provided URL is invalid or the platform is not supported")
	}

	videoID := extractVideoID(y.Query)
	if videoID == "" {
		return utils.TrackInfo{}, errors.New("invalid YouTube URL")
	}

	if y.ApiUrl != "" && y.APIKey != "" {
		return utils.TrackInfo{
			Id:       videoID,
			URL:      strings.TrimSpace(y.Query),
			Platform: utils.YouTube,
		}, nil
	}

	getInfo, err := y.getInfo()
	if err != nil || len(getInfo.Results) == 0 {

		videoID := extractVideoID(y.Query)

		if videoID != "" {
			slog.Info("Falling back to direct YouTube ID",
				"video_id", videoID,
			)

			return utils.TrackInfo{
				Id:       videoID,
				URL:      y.Query,
				Platform: utils.YouTube,
			}, nil
		}

		if err != nil {
			return utils.TrackInfo{}, err
		}

		return utils.TrackInfo{}, errors.New("no video results were found")
	}

	track := getInfo.Results[0]
	trackInfo := utils.TrackInfo{
		Id:       track.Id,
		URL:      track.Url,
		Platform: utils.YouTube,
	}

	return trackInfo, nil
}
func (y *youTubeData) resolveLiveStream(videoID string) (string, bool, error) {
	if videoID == "" {
		return "", false, errors.New("videoID is empty")
	}

	args := []string{
		"yt-dlp",
		"--no-warnings",
		"--no-playlist",
		"-J",
		"https://www.youtube.com/watch?v=" + videoID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	slog.Info("Running yt-dlp resolver", "args", args)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	out, err := cmd.Output()
	if err != nil {
		return "", false, err
	}

	var info ytDlpInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return "", false, err
	}

	if len(info.Entries) > 0 {
		info = info.Entries[0]
	}

	stream := info.URL

	if stream == "" {
		for i := len(info.Formats) - 1; i >= 0; i-- {
			if info.Formats[i].URL != "" {
				stream = info.Formats[i].URL
				break
			}
		}
	}

	if stream == "" {
		return "", false, errors.New("no playable stream found")
	}

	return stream, info.IsLive, nil
}

// downloadTrack handles the download of a track from YouTube.
func (y *youTubeData) downloadTrack(info utils.TrackInfo, video bool) (string, error) {
	if y.ApiUrl != "" && y.APIKey != "" {
		filePath, err := y.downloadWithApi(info.Id, video)
		if err != nil {
			slog.Warn("YouTube API download failed", "video_id", info.Id, "error", err)
			return "", err
		}

		return filePath, nil
	}

	return y.downloadWithYtDlp(info.Id, video)
}

// downloadWithYtDlp downloads media from YouTube using the yt-dlp command-line tool.
func (y *youTubeData) downloadWithYtDlp(videoID string, video bool) (string, error) {
	if videoID == "" {
		return "", errors.New("videoID is empty")
	}

	ytdlpParams := y.buildYtdlpParams(videoID, video)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	slog.Info("Running yt-dlp", "args", ytdlpParams)
	cmd := exec.CommandContext(ctx, ytdlpParams[0], ytdlpParams[1:]...)

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := string(exitErr.Stderr)
			return "", fmt.Errorf("yt-dlp failed with exit code %d: %s", exitErr.ExitCode(), stderr)
		}

		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("yt-dlp timed out for video ID: %s", videoID)
		}

		return "", fmt.Errorf("an unexpected error occurred while downloading %s: %w", videoID, err)
	}

	downloadedPathStr := strings.TrimSpace(string(output))
	if downloadedPathStr == "" {
		return "", fmt.Errorf("no output path was returned for %s", videoID)
	}

	if _, err := os.Stat(downloadedPathStr); os.IsNotExist(err) {
		return "", fmt.Errorf("the file was not found at the reported path: %s", downloadedPathStr)
	}

	return downloadedPathStr, nil
}

func (y *youTubeData) buildYtdlpParams(videoID string, video bool) []string {
	outputTemplate := filepath.Join(config.DownloadsDir, "%(id)s.%(ext)s")

	params := []string{
		"yt-dlp",
		"--no-warnings",
		"--quiet",
		"--geo-bypass",
		"--retries", "2",
		"--continue",
		"--no-part",
		"--concurrent-fragments", "3",
		"--socket-timeout", "10",
		"--throttled-rate", "100K",
		"--retry-sleep", "1",
		"--no-write-thumbnail",
		"--no-write-info-json",
		"--no-embed-metadata",
		"--no-embed-chapters",
		"--no-embed-subs",
		"--extractor-args", "youtube:player_js_version=actual",
		"-o", outputTemplate,
	}

	if video {
		params = append(params,
			"-f",
			"bestvideo[height<=720][ext=mp4]+bestaudio[ext=m4a]/bestvideo[height<=720]+bestaudio/best[height<=720]",
			"--merge-output-format",
			"mp4",
		)
	} else {
		params = append(params,
			"-f",
			"bestaudio[ext=m4a]/bestaudio",
		)
	}

	if config.Proxy != "" {
		params = append(params, "--proxy", config.Proxy)
	}

	params = append(
		params,
		"https://www.youtube.com/watch?v="+videoID,
		"--print",
		"after_move:filepath",
	)

	return params
}

// downloadWithApi downloads a track using the external API.
func (y *youTubeData) downloadWithApi(videoID string, video bool) (string, error) {
	videoUrl := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	a := newApiData(videoUrl)
	if a.ApiUrl == "" || a.APIKey == "" {
		return "", errors.New("invalid API configuration")
	}

	track := utils.TrackInfo{
		Id:       videoID,
		URL:      videoUrl,
		Platform: utils.YouTube,
		CdnURL:   a.buildDownloadURL(video),
	}

	down, err := newDownload(track)
	if err != nil {
		slog.Info("Error creating download: " + err.Error())
		return "", err
	}

	localPath, err := down.Process()
	if err == nil {
		return localPath, nil
	}

	slog.Warn("API download failed, falling back to yt-dlp", "video_id", videoID, "error", err)
	return y.downloadWithYtDlp(videoID, video)
}
