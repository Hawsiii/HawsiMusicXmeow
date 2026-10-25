/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package downloader

import (
	"ashokshau/tgmusic/internal/config"
	"ashokshau/tgmusic/internal/utils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	meowAudioQuality = "128"
	meowVideoQuality = "720"

	// meowMinValidSize mirrors the Python reference's 10000-byte sanity check
	// (rejects tiny error/placeholder responses before they hit ffprobe).
	meowMinValidSize = 10000
)

func meowApiConfigured() bool {
	return strings.TrimSpace(config.MeowApiUrl) != "" && strings.TrimSpace(config.MeowApiKey) != ""
}

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

func (y *youTubeData) getInfo() (*utils.PlatformTracks, error) {
	if !y.isValid() {
		return nil, errors.New("the provided URL is invalid or the platform is not supported")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	y.Query = normalizeYouTubeURL(y.Query)
	videoID := extractVideoID(y.Query)
	playlistID := extractPlaylistID(y.Query)

	switch {
	case playlistID != "":
		if strings.HasPrefix(playlistID, "RD") && !strings.HasPrefix(playlistID, "RDCLAK") {
			return GetYouTubeMixPlaylist(ctx, playlistID)
		}
		return getYouTubePlaylist(ctx, playlistID)

	case videoID != "":
		video, playerErr := getYouTubeVideo(ctx, videoID)
		if playerErr == nil {
			return video, nil
		}

		for _, query := range []string{videoID, y.Query} {
			tracks, err := searchYouTube(query, 10, ctx)
			if err != nil {
				continue
			}

			for _, track := range tracks {
				if track.Id == videoID {
					return &utils.PlatformTracks{Results: []utils.GetUrlTrack{track}}, nil
				}
			}
		}

		if ctx.Err() == nil {
			if title, err := getYouTubeTitleFromOEmbed(ctx, videoID); err == nil && title != "" {
				tracks, err := searchYouTube(title, 10, ctx)
				if err == nil {
					for _, track := range tracks {
						if track.Id == videoID {
							return &utils.PlatformTracks{Results: []utils.GetUrlTrack{track}}, nil
						}
					}
				}
			}
		}

		slog.Warn("YouTube metadata lookup failed; continuing with the video URL", "video_id", videoID, "error", playerErr)
		return &utils.PlatformTracks{Results: []utils.GetUrlTrack{{
			Id:       videoID,
			Url:      ytWatchURL + videoID,
			Title:    videoID,
			Platform: utils.YouTube,
		}}}, nil
	}

	return nil, errors.New("no video or playlist results were found")
}

func (y *youTubeData) search() (*utils.PlatformTracks, error) {
	if y.isValid() {
		return y.getInfo()
	}

	tracks, err := searchYouTube(y.Query, 5)
	if err != nil {
		return nil, err
	}

	return &utils.PlatformTracks{Results: tracks}, nil
}

func (y *youTubeData) getTrack() (*utils.TrackInfo, error) {
	if y.Query == "" {
		return nil, errors.New("the query is empty")
	}

	if !y.isValid() {
		return nil, errors.New("the provided URL is invalid or the platform is not supported")
	}

	videoID := extractVideoID(y.Query)
	if videoID == "" {
		return nil, errors.New("invalid YouTube URL")
	}

	if y.ApiUrl != "" && y.APIKey != "" {
		return &utils.TrackInfo{
			Id:       videoID,
			URL:      strings.TrimSpace(y.Query),
			Platform: utils.YouTube,
		}, nil
	}

	getInfo, err := y.getInfo()
	if err != nil || len(getInfo.Results) == 0 {
		if videoID != "" {
			slog.Info("Falling back to direct YouTube ID", "video_id", videoID)
			return &utils.TrackInfo{
				Id:       videoID,
				URL:      y.Query,
				Platform: utils.YouTube,
			}, nil
		}
		if err != nil {
			return nil, err
		}
		return nil, errors.New("no video results were found")
	}

	track := getInfo.Results[0]
	trackInfo := &utils.TrackInfo{
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
func (y *youTubeData) downloadTrack(info *utils.TrackInfo, video bool) (string, error) {
	if !video {
		if cachedPath := findCachedYouTubeMedia(info.Id, false); cachedPath != "" {
			return cachedPath, nil
		}

		if meowApiConfigured() {
			filePath, err := y.downloadWithApi(info.Id, false)
			if err == nil {
				if err = validateDownloadedMedia(filePath, false); err == nil {
					return filePath, nil
				}
				slog.Warn("Meow API returned invalid media", "video_id", info.Id, "error", err)
			} else {
				slog.Warn("Meow API download failed", "video_id", info.Id, "error", err)
			}
		}

		if info.CdnURL != "" {
			return info.CdnURL, nil
		}
		if y.ApiUrl != "" && y.APIKey != "" {
			if filePath, err := y.downloadWithGatewayApi(info.Id); err == nil {
				return filePath, nil
			} else {
				slog.Warn("API gateway download failed", "video_id", info.Id, "error", err)
			}
		}
		return y.downloadWithYtDlp(info.Id, false)
	}

	slog.Info("Downloading YouTube video with yt-dlp for group call playback", "video_id", info.Id)
	return y.downloadWithYtDlp(info.Id, video)
}

func findCachedYouTubeMedia(videoID string, video bool) string {
	if videoID == "" {
		return ""
	}

	paths, err := filepath.Glob(filepath.Join(config.DownloadsDir, videoID+".*"))
	if err != nil {
		return ""
	}

	for _, path := range paths {
		if cachedFileUsable(path, video) {
			return path
		}
	}

	return ""
}

// downloadWithYtDlp downloads media from YouTube using the yt-dlp command-line tool.
func (y *youTubeData) downloadWithYtDlp(videoID string, video bool) (string, error) {
	if videoID == "" {
		return "", errors.New("videoID is empty")
	}

	ytdlpParams := y.buildYtdlpParams(videoID, video)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	path, err := runYtDlpDownload(ctx, ytdlpParams, videoID, video)
	if err == nil {
		return path, nil
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
		return "", err
	}

	fallbackParams := y.buildYtdlpFallbackParams(videoID, video)
	slog.Warn("Retrying yt-dlp with fallback formats after download failure", "video_id", videoID, "error", err)
	fallbackPath, fallbackErr := runYtDlpDownload(ctx, fallbackParams, videoID, video)
	if fallbackErr != nil {
		return "", fmt.Errorf("yt-dlp primary attempt failed: %v; fallback attempt failed: %w", err, fallbackErr)
	}

	return fallbackPath, nil
}

func runYtDlpDownload(ctx context.Context, params []string, videoID string, video bool) (string, error) {

	slog.Info("Running yt-dlp", "args", params)
	cmd := exec.CommandContext(ctx, params[0], params[1:]...)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
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

	if _, err := os.Stat(downloadedPathStr); err != nil {
		return "", fmt.Errorf("downloaded file is unavailable at %s: %w", downloadedPathStr, err)
	}

	if err := validateDownloadedMedia(downloadedPathStr, video); err != nil {
		return "", err
	}

	return downloadedPathStr, nil
}

func (y *youTubeData) buildYtdlpFallbackParams(videoID string, video bool) []string {
	params := y.buildYtdlpParams(videoID, video)
	for index := range params {
		if params[index] == "-f" && index+1 < len(params) {
			if video {
				params[index+1] = "bestvideo[height<=720]+bestaudio/best[height<=720]"
			} else {
				params[index+1] = "ba/b"
			}
			break
		}
	}
	params = append(params, "--extractor-args", "youtube:player_client=android,web")
	return params
}

func validateDownloadedMedia(filePath string, video bool) error {
	if filePath == "" {
		return errors.New("download returned an empty file path")
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("downloaded file is unavailable at %s: %w", filePath, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("downloaded media is empty: %s", filePath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-show_entries", "stream=codec_type", "-of", "csv=p=0", filePath)
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		if ctx.Err() != nil {
			return fmt.Errorf("timed out validating downloaded media: %s", filePath)
		}
		return fmt.Errorf("downloaded media is invalid or incomplete: %s", filePath)
	}

	streams := strings.Fields(string(output))
	hasAudio, hasVideo := false, false
	for _, stream := range streams {
		hasAudio = hasAudio || stream == "audio"
		hasVideo = hasVideo || stream == "video"
	}
	if !hasAudio || (video && !hasVideo) {
		return fmt.Errorf("downloaded media is missing required streams (audio=%t, video=%t): %s", hasAudio, hasVideo, filePath)
	}

	return nil
}

func (y *youTubeData) buildYtdlpParams(videoID string, video bool) []string {
	outputTemplate := filepath.Join(config.DownloadsDir, "%(id)s.%(ext)s")

	params := []string{
		"yt-dlp",
		"--no-warnings",
		"--quiet",
		"--geo-bypass",
		"--js-runtimes", "deno:/usr/local/bin/deno",
		"--remote-components", "ejs:github",
		"--retries", "2",
		"--no-continue",
		"--force-overwrites",
		"--concurrent-fragments", "3",
		"--socket-timeout", "10",
		"--throttled-rate", "100K",
		"--retry-sleep", "1",
		"--no-write-thumbnail",
		"--no-write-info-json",
		"--no-embed-metadata",
		"--no-embed-chapters",
		"--no-embed-subs",
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

// downloadWithApi downloads a track from Meow's direct stream endpoint.
func (y *youTubeData) downloadWithApi(videoID string, video bool) (string, error) {
	if videoID == "" {
		return "", errors.New("videoID is empty")
	}

	if !meowApiConfigured() {
		return "", errors.New("invalid API configuration")
	}

	downloadType := "audio"
	ext := ".mp3"
	quality := meowAudioQuality
	if video {
		downloadType = "video"
		ext = ".mp4"
		quality = meowVideoQuality
	}

	fileName := filepath.Join(config.DownloadsDir, videoID+ext)

	// Reuse an already-downloaded file if it looks valid, same as the Python cache check.
	if fi, err := os.Stat(fileName); err == nil && fi.Size() > meowMinValidSize {
		return fileName, nil
	}

	streamURL, err := buildMeowStreamURL(videoID, downloadType, quality)
	if err != nil {
		return "", err
	}

	slog.Info("Downloading via Meow API", "video_id", videoID, "type", downloadType)

	localPath, err := downloadFile(streamURL, fileName, true)
	if err != nil {
		if _, statErr := os.Stat(fileName); statErr == nil {
			_ = os.Remove(fileName)
		}
		return "", fmt.Errorf("meow api download failed for %s: %w", videoID, err)
	}

	if fi, err := os.Stat(localPath); err != nil || fi.Size() <= meowMinValidSize {
		_ = os.Remove(localPath)
		return "", fmt.Errorf("meow api returned an undersized file for %s", videoID)
	}

	return localPath, nil
}

func buildMeowStreamURL(videoID, downloadType, quality string) (string, error) {
	if strings.TrimSpace(config.MeowApiUrl) == "" || strings.TrimSpace(config.MeowApiKey) == "" {
		return "", errors.New("invalid Meow API configuration")
	}

	baseURL, err := url.Parse(strings.TrimRight(config.MeowApiUrl, "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return "", fmt.Errorf("invalid Meow API URL: %q", config.MeowApiUrl)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/stream/" + url.PathEscape(videoID)
	params := baseURL.Query()
	params.Set("key", config.MeowApiKey)
	params.Set("type", downloadType)
	params.Set("quality", quality)
	baseURL.RawQuery = params.Encode()
	return baseURL.String(), nil
}

func (y *youTubeData) downloadWithGatewayApi(videoID string) (string, error) {
	videoURL := "https://www.youtube.com/watch?v=" + videoID
	track, err := newApiData(videoURL).getTrack()
	if err != nil {
		return "", err
	}
	return processDownload(track)
}

// getRecommendations retrieves native YouTube mix recommendations without external API.
func (y *youTubeData) getRecommendations(ctx context.Context, rawURL string) (*utils.PlatformTracks, error) {
	urlToUse := rawURL
	if urlToUse == "" {
		urlToUse = y.Query
	}

	videoID := extractVideoID(urlToUse)
	if videoID == "" && urlToUse != "" && !strings.HasPrefix(urlToUse, "http") {
		if len(urlToUse) == 11 {
			videoID = urlToUse
		}
	}

	if videoID == "" {
		return nil, errors.New("no valid YouTube video ID found for recommendations")
	}
	return GetYouTubeMixPlaylist(ctx, "RD"+videoID)
}
