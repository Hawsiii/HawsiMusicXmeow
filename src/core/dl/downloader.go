/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package dl

import (
	"ashokshau/tgmusic/src/utils"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	td "github.com/AshokShau/gotdbot"
)

func DownloadCachedTrack(cached *utils.CachedTrack, bot *td.Client) (string, error) {
	if cached == nil {
		return "", fmt.Errorf("cached track is nil")
	}

	if cached.FilePath != "" {
		if cachedFileUsable(cached.FilePath, cached.IsVideo) {
			return cached.FilePath, nil
		}
		cached.FilePath = ""
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

	streamType := "a:0"
	if video {
		streamType = "v:0"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", streamType,
		"-show_entries", "stream=codec_type", "-of", "csv=p=0", filePath).Output()
	return err == nil && strings.TrimSpace(string(output)) != ""
}

func downloadViaWrapper(cached *utils.CachedTrack, dlBot *td.Client) (string, error) {
	wrapper := NewDownloaderWrapper(cached.URL)
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

func downloadTelegramFile(cached *utils.CachedTrack, bot *td.Client) (string, error) {
	file, err := bot.GetRemoteFile(cached.TrackID, nil)
	if err != nil {
		return "", err
	}

	download, err := file.Download(bot, 0, 0, 1, &td.DownloadFileOpts{Synchronous: true})
	if err != nil {
		return "", err
	}

	return download.Local.Path, nil
}

func downloadFromTelegramMessage(bot *td.Client, msgURL string) (string, error) {
	msg, err := utils.GetMessage(bot, msgURL)
	if err != nil {
		return "", fmt.Errorf("get telegram message: %w", err)
	}

	file, err := msg.Download(bot, 1, 0, 0, true)
	if err != nil {
		return "", err
	}

	if file == nil || file.Local == nil {
		return "", fmt.Errorf("failed to download file from Telegram message")
	}

	return file.Local.Path, nil
}
