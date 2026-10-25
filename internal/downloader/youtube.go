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
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	meowAudioQuality = "128"
	meowVideoQuality = "720"

	// Reject tiny error or placeholder responses before probing their media streams.
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

// downloadTrack handles the download of a track from YouTube.
func (y *youTubeData) downloadTrack(info *utils.TrackInfo, video bool) (string, error) {
	if info == nil || info.Id == "" {
		return "", errors.New("YouTube video ID is empty")
	}
	if !meowApiConfigured() {
		return "", errors.New("Meow API is required for YouTube downloads; configure MEOW_API_URL and MEOW_API_KEY")
	}

	if cachedPath := findCachedYouTubeMedia(info.Id, video); cachedPath != "" {
		return cachedPath, nil
	}

	mediaType := "audio"
	if video {
		mediaType = "video"
	}
	slog.Info("Downloading YouTube media via Meow API", "video_id", info.Id, "type", mediaType)
	filePath, err := y.downloadWithApi(info.Id, video)
	if err != nil {
		return "", err
	}
	if err = validateDownloadedMedia(filePath, video); err != nil {
		if removeErr := os.Remove(filePath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			slog.Warn("Failed to remove invalid Meow media", "video_id", info.Id, "path", filePath, "error", removeErr)
		}
		return "", fmt.Errorf("Meow API returned invalid media: %w", err)
	}
	return filePath, nil
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
		if validateDownloadedMedia(path, video) == nil {
			return path
		}
	}

	return ""
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
		"-show_entries", "format=duration:stream=codec_type,duration", "-of", "json", filePath)
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		if ctx.Err() != nil {
			return fmt.Errorf("timed out validating downloaded media: %s", filePath)
		}
		return fmt.Errorf("downloaded media is invalid or incomplete: %s", filePath)
	}

	var probe struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Duration  string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &probe); err != nil {
		return fmt.Errorf("failed to parse downloaded media probe: %w", err)
	}

	hasAudio, hasVideo := false, false
	duration := parsePositiveDuration(probe.Format.Duration)
	videoDuration := 0.0
	for _, stream := range probe.Streams {
		hasAudio = hasAudio || stream.CodecType == "audio"
		hasVideo = hasVideo || stream.CodecType == "video"
		if streamDuration := parsePositiveDuration(stream.Duration); stream.CodecType == "video" && streamDuration > videoDuration {
			videoDuration = streamDuration
		}
		if streamDuration := parsePositiveDuration(stream.Duration); streamDuration > duration {
			duration = streamDuration
		}
	}
	if !hasAudio || (video && !hasVideo) {
		return fmt.Errorf("downloaded media is missing required streams (audio=%t, video=%t): %s", hasAudio, hasVideo, filePath)
	}
	if duration <= 0 {
		return fmt.Errorf("downloaded media has no playable duration: %s", filePath)
	}
	if video && !videoStreamCoversDuration(videoDuration, duration) {
		return fmt.Errorf("downloaded video stream ends too early (video=%.2fs, media=%.2fs): %s", videoDuration, duration, filePath)
	}
	if video {
		if err := validateVideoFrameWindow(filePath, duration); err != nil {
			return err
		}
	}

	return nil
}

func validateVideoFrameWindow(filePath string, mediaDuration float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-read_intervals", "%+5",
		"-select_streams", "v:0", "-count_frames", "-show_entries", "stream=nb_read_frames,avg_frame_rate", "-of", "json", filePath)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("timed out decoding downloaded video: %s", filePath)
		}
		return fmt.Errorf("failed to decode downloaded video: %s: %w", filePath, err)
	}

	var probe struct {
		Streams []struct {
			FrameCount string `json:"nb_read_frames"`
			FrameRate  string `json:"avg_frame_rate"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output, &probe); err != nil {
		return fmt.Errorf("failed to parse downloaded video frame probe: %w", err)
	}
	if len(probe.Streams) == 0 {
		return fmt.Errorf("downloaded video has no decodable frames: %s", filePath)
	}

	frameCount, err := strconv.Atoi(probe.Streams[0].FrameCount)
	if err != nil || frameCount <= 0 {
		return fmt.Errorf("downloaded video has no decodable frames: %s", filePath)
	}
	frameRate := parseFrameRate(probe.Streams[0].FrameRate)
	if frameRate <= 0 {
		if !decodedVideoFramesCoverWindow(frameCount, frameRate, mediaDuration) {
			return fmt.Errorf("downloaded video ends immediately: %s", filePath)
		}
		return nil
	}

	if !decodedVideoFramesCoverWindow(frameCount, frameRate, mediaDuration) {
		expectedDuration := min(mediaDuration, 5)
		if expectedDuration <= 0 {
			expectedDuration = 5
		}
		decodedDuration := float64(frameCount) / frameRate
		return fmt.Errorf("downloaded video ends too early (decoded=%.2fs, expected=%.2fs): %s",
			decodedDuration, expectedDuration, filePath)
	}
	return nil
}

func decodedVideoFramesCoverWindow(frameCount int, frameRate, mediaDuration float64) bool {
	if frameCount <= 0 {
		return false
	}
	if frameRate <= 0 {
		return frameCount >= 2
	}

	expectedDuration := min(mediaDuration, 5)
	if expectedDuration <= 0 {
		expectedDuration = 5
	}
	decodedDuration := float64(frameCount) / frameRate
	tolerance := math.Max(0.5, expectedDuration*0.1)
	return expectedDuration-decodedDuration <= tolerance
}

func parseFrameRate(raw string) float64 {
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return 0
	}
	numerator, numeratorErr := strconv.ParseFloat(parts[0], 64)
	denominator, denominatorErr := strconv.ParseFloat(parts[1], 64)
	if numeratorErr != nil || denominatorErr != nil || denominator <= 0 {
		return 0
	}
	return numerator / denominator
}

func videoStreamCoversDuration(videoDuration, mediaDuration float64) bool {
	if videoDuration <= 0 || mediaDuration <= 0 {
		return true
	}

	tolerance := mediaDuration * 0.02
	if tolerance < 2 {
		tolerance = 2
	}
	return mediaDuration-videoDuration <= tolerance
}

func parsePositiveDuration(raw string) float64 {
	duration, err := strconv.ParseFloat(raw, 64)
	if err != nil || duration <= 0 {
		return 0
	}
	return duration
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

	if fi, err := os.Stat(fileName); err == nil && fi.Size() > meowMinValidSize {
		if err := validateDownloadedMedia(fileName, video); err == nil {
			return fileName, nil
		} else {
			slog.Warn("Cached Meow media is invalid; downloading a fresh copy", "video_id", videoID, "error", err)
		}
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
