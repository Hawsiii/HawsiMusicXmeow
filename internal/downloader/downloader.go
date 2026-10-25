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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	td "github.com/AshokShau/gotdbot"
)

// processDownload processes a track download based on platform metadata.
func processDownload(track *utils.TrackInfo) (string, error) {
	if track.CdnURL == "" {
		return "", errMissingCDNURL
	}

	if track.Key != "" && strings.EqualFold(track.Platform, "spotify") {
		return processSpotify(track)
	}

	return track.CdnURL, nil
}

func DlCachedTrack(cached *utils.PlayerCache, bot *td.Client) (string, error) {
	if cached == nil {
		return "", fmt.Errorf("cached track is nil")
	}

	if cached.FilePath != "" && cachedFileUsable(cached.FilePath, cached.IsVideo) {
		return cached.FilePath, nil
	}

	if cached.Platform == utils.DirectLink {
		return cached.URL, nil
	}

	if cached.Platform == utils.Telegram {
		return downloadTelegramFile(cached, bot)
	}

	dlBot := bot
	if DlBot != nil {
		dlBot = DlBot
	}

	path, err := downloadViaWrapper(cached, dlBot)
	if err != nil {
		return "", err
	}

	if !isRemoteMediaPath(path) && !cachedFileUsable(path, cached.IsVideo) {
		return "", fmt.Errorf("downloaded media is invalid: %s", path)
	}

	cached.FilePath = path
	return path, nil
}

func isRemoteMediaPath(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

func cachedFileUsable(filePath string, video bool) bool {
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-show_entries", "stream=codec_type", "-of", "csv=p=0", filePath).Output()
	if err != nil {
		return false
	}

	hasAudio, hasVideo := false, false
	for _, stream := range strings.Fields(string(output)) {
		hasAudio = hasAudio || stream == "audio"
		hasVideo = hasVideo || stream == "video"
	}
	return hasAudio && (!video || hasVideo)
}

func downloadViaWrapper(cached *utils.PlayerCache, dlBot *td.Client) (string, error) {
	wrapper := NewDlWrapper(cached.URL)
	if !wrapper.IsValid() {
		return "", fmt.Errorf("invalid cached URL: %s", cached.URL)
	}

	track, err := wrapper.GetTrack()
	if err != nil {
		return "", fmt.Errorf("get track info: %w", err)
	}

	path, err := wrapper.DownloadTrack(track, cached.IsVideo)
	if err != nil {
		return "", err
	}

	if utils.TelegramMessageRegex.MatchString(path) {
		return downloadFromTelegramMessage(dlBot, path)
	}

	return path, nil
}

func downloadTelegramFile(cached *utils.PlayerCache, bot *td.Client) (string, error) {
	file, err := bot.GetRemoteFile(cached.TrackID, nil)
	if err != nil {
		return "", err
	}

	d, err := file.Download(bot, 1, &td.DownloadFileOpts{Synchronous: true})
	if err != nil {
		return "", err
	}

	return d.Local.Path, nil
}

func downloadFromTelegramMessage(bot *td.Client, msgURL string) (string, error) {
	msg, err := utils.GetMessage(bot, msgURL)
	if err != nil {
		return "", fmt.Errorf("get telegram message: %w", err)
	}

	file, err := msg.Download(bot, 1, &td.DownloadFileOpts{Synchronous: true})
	if err != nil {
		return "", err
	}

	if file == nil || file.Local == nil {
		return "", fmt.Errorf("failed to download file from Telegram message")
	}

	return file.Local.Path, nil
}
