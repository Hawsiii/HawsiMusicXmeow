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
	"log/slog"

	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type apiData struct {
	Query    string
	ApiUrl   string
	APIKey   string
	Patterns map[string]*regexp.Regexp
}

var apiPatterns = map[string]*regexp.Regexp{
	utils.Apple:      regexp.MustCompile(`(?i)^https?:\/\/music\.apple\.com\/[a-zA-Z-]+\/(?:song\/(?:[^\/]+\/)?\d+|album\/[^\/]+\/\d+(?:\?i=\d+)?|playlist\/[^\/]+\/pl\.[\w.-]+|artist\/[^\/]+\/\d+)(?:\?.*)?$`),
	utils.Spotify:    regexp.MustCompile(`(?i)^(https?://)?([a-z0-9-]+\.)*spotify\.com/(track|playlist|album|artist)/[a-zA-Z0-9]+(\?.*)?$`),
	utils.YouTube:    regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?(?:youtube\.com|youtu\.be)/.*$`),
	utils.JioSaavn:   regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?(?:jiosaavn|saavn)\.com\/(?:s\/)?(song|album|playlist|featured)(?:\/[^\/]+)*\/([A-Za-z0-9_,-]+)(?:\/)?(?:\?.*)?$`),
	utils.Deezer:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?deezer\.com\/(?:[a-z]{2}\/)?(track|album|playlist)\/(\d+)`),
	utils.SoundCloud: regexp.MustCompile(`(?i)^(https?://)?(www\.)?soundcloud\.com/[a-zA-Z0-9_-]+/(sets/)?[a-zA-Z0-9._-]+(\?.*)?$`),
	utils.Gaana:      regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?gaana\.com\/(song|album|playlist|artist)\/([A-Za-z0-9\-]+)`),
	utils.Tidal:      regexp.MustCompile(`(?i)https?:\/\/(?:www\.|listen\.)?tidal\.com\/(?:browse\/)?(track|album|playlist)\/([a-zA-Z0-9-]+)(?:[\/?].*)?`),
	utils.MXPlayer:   regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?mxplayer\.in\/(?:show|movie)\/.*`),
	utils.Twitch:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.|m\.)?twitch\.tv\/(?:videos|[\w._-]+\/video)\/\d+`),
	utils.TwitchClip: regexp.MustCompile(
		`(?i)https?:\/\/(?:www\.|m\.)?(?:` +
			`twitch\.tv\/clip\/[\w-]+|` +
			`clips\.twitch\.tv\/[\w-]+|` +
			`twitch\.tv\/[\w-]+\/clip\/[\w-]+` +
			`)`,
	),
	utils.Kick:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?kick\.com\/[\w._-]+\/videos\/[a-fA-F0-9-]+`),
	utils.KickClip: regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?kick\.com\/[\w._-]+\/clips\/[\w-]+`),
}

func newApiData(query string) *apiData {
	return &apiData{
		Query:    strings.TrimSpace(query),
		ApiUrl:   strings.TrimRight(config.ApiUrl, "/"),
		APIKey:   config.ApiKey,
		Patterns: apiPatterns,
	}
}

func (a *apiData) isValid() bool {
	if a.Query == "" || a.ApiUrl == "" || a.APIKey == "" {
		return false
	}

	for _, pattern := range a.Patterns {
		if pattern.MatchString(a.Query) {
			return true
		}
	}
	return false
}

func (a *apiData) getInfo() (*utils.PlatformTracks, error) {
	if !a.isValid() {
		return nil, errors.New("the provided URL is invalid or the platform is not supported")
	}

	params := url.Values{"url": {a.Query}}
	resp, err := a.apiRequest("/api/get_url", params)
	if err != nil {
		return nil, fmt.Errorf("the GetInfo request failed: %w", err)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code while fetching info: %s", resp.Status)
	}

	var data utils.PlatformTracks
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode the GetInfo response: %w", err)
	}
	return &data, nil
}

func (a *apiData) search() (*utils.PlatformTracks, error) {
	if a.isValid() {
		return a.getInfo()
	}

	params := url.Values{
		"query": {a.Query},
		"limit": {"5"},
	}
	resp, err := a.apiRequest("/api/search", params)
	if err != nil {
		return nil, fmt.Errorf("the search request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code during search: %s", resp.Status)
	}

	var data utils.PlatformTracks
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		slog.Warn("Failed to decode search response", "error", err)
		return nil, fmt.Errorf("failed to decode the search response: %w", err)
	}

	return &data, nil
}

// getTrack retrieves detailed information for a single track from the API.
func (a *apiData) getTrack() (*utils.TrackInfo, error) {
	params := url.Values{"url": {a.Query}}
	resp, err := a.apiRequest("/api/track", params)
	if err != nil {
		slog.Warn("GetTrack request failed", "error", err)
		return nil, fmt.Errorf("the GetTrack request failed: %w", err)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code while fetching the track: %s", resp.Status)
	}

	var data utils.TrackInfo
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		slog.Warn("Failed to decode the GetTrack response", "error", err)
		return nil, fmt.Errorf("failed to decode the GetTrack response: %w", err)
	}

	return &data, nil
}

func (a *apiData) apiRequest(path string, params url.Values) (*http.Response, error) {
	if a.ApiUrl == "" || a.APIKey == "" {
		return nil, errors.New("invalid API configuration")
	}

	if params == nil {
		params = url.Values{}
	}

	headers := map[string]string{"X-API-Key": a.APIKey}
	requestURL := a.ApiUrl + path + "?" + params.Encode()
	resp, err := sendRequest(http.MethodGet, requestURL, nil, headers)
	if err != nil {
		if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "unexpected status code") {
			return nil, err
		}
		return a.apiRequestWithQueryKey(path, params)
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return a.apiRequestWithQueryKey(path, params)
	}

	return resp, nil
}

func (a *apiData) apiRequestWithQueryKey(path string, params url.Values) (*http.Response, error) {
	fallbackParams := make(url.Values, len(params)+1)
	for key, values := range params {
		fallbackParams[key] = append([]string(nil), values...)
	}
	fallbackParams.Set("api_key", a.APIKey)
	return sendRequest(http.MethodGet, a.ApiUrl+path+"?"+fallbackParams.Encode(), nil, nil)
}

func (a *apiData) downloadTrack(info *utils.TrackInfo, video bool) (string, error) {
	// if the track is from YouTube and video:true
	yt := newYouTubeData(a.Query)
	if info.Platform == utils.YouTube && video {
		return yt.downloadTrack(info, video)
	}

	filePath, err := processDownload(info)
	if err != nil {
		if info.Platform == utils.YouTube {
			return yt.downloadTrack(info, video)
		}
		return "", fmt.Errorf("the download process failed: %w", err)
	}

	if strings.Contains(a.ApiUrl, filePath) {
		return downloadFile(filePath, "", false)
	}

	return filePath, nil
}

// getRecommendations retrieves recommended tracks for a given track URL from the API gateway.
func (a *apiData) getRecommendations(ctx context.Context, rawURL string) (*utils.PlatformTracks, error) {
	if a.ApiUrl == "" || a.APIKey == "" {
		return nil, errors.New("API URL or API key is not configured")
	}

	urlToUse := rawURL
	if urlToUse == "" {
		urlToUse = a.Query
	}
	if urlToUse == "" {
		return nil, errors.New("no URL provided for recommendations")
	}

	fullURL := fmt.Sprintf("%s/api/recommendations?%s", a.ApiUrl, url.Values{"url": {urlToUse}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", a.APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the GetRecommendations request failed: %w", err)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code while fetching recommendations: %s", resp.Status)
	}

	var data utils.PlatformTracks
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode the GetRecommendations response: %w", err)
	}
	return &data, nil
}
