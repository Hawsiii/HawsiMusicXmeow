/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package calls

import (
	"ashokshau/tgmusic/ntgcalls"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var urlRegex = regexp.MustCompile(`^https?://`)

// getMediaDescription creates a media description for ntgcalls based on the provided file path, video status, and ffmpeg parameters.
func getMediaDescription(filePath string, isVideo bool, _ int64, ffmpegParameters string) ntgcalls.MediaDescription {
	audioDescription := &ntgcalls.AudioDescription{
		MediaSource:  ntgcalls.MediaSourceShell,
		SampleRate:   48000,
		ChannelCount: 2,
	}

	quotedPath := strconv.Quote(filePath)
	isURL := urlRegex.MatchString(filePath)

	var audioCmd strings.Builder
	audioCmd.WriteString("ffmpeg ")
	if isURL {
		audioCmd.WriteString("-reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1 -reconnect_delay_max 2 ")
	}
	if shouldPaceInput(filePath) {
		audioCmd.WriteString("-re ")
	}

	var seekFlags, filterFlags string
	if ffmpegParameters != "" {
		if strings.Contains(ffmpegParameters, "filter:") {
			filterFlags = ffmpegParameters
		} else {
			seekFlags = ffmpegParameters
		}
	}
	videoFilter, audioFilter := splitFilterFlags(filterFlags)

	if seekFlags != "" {
		audioCmd.WriteString(seekFlags + " ")
	}
	audioCmd.WriteString("-i " + quotedPath + " ")
	if audioFilter != "" {
		audioCmd.WriteString("-af " + strconv.Quote(audioFilter) + " ")
	}
	audioCmd.WriteString(fmt.Sprintf(
		"-fflags +nobuffer -flags low_delay -threads 0 -f s16le -ac %d -ar %d -loglevel error pipe:1",
		audioDescription.ChannelCount,
		audioDescription.SampleRate,
	))

	audioDescription.Input = audioCmd.String()

	if !isVideo {
		return ntgcalls.MediaDescription{
			Microphone: audioDescription,
		}
	}

	width := 1280
	height := 720

	videoDescription := &ntgcalls.VideoDescription{
		MediaSource: ntgcalls.MediaSourceShell,
		Width:       int16(width),
		Height:      int16(height),
		Fps:         30,
	}

	scaleFilter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease:force_divisible_by=2:flags=lanczos,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1",
		width,
		height,
		width,
		height,
	)

	var videoCmd strings.Builder
	videoCmd.WriteString("ffmpeg -hide_banner ")

	if isURL {
		videoCmd.WriteString("-reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1 -reconnect_delay_max 2 ")
	}
	if shouldPaceInput(filePath) {
		videoCmd.WriteString("-re ")
	}

	if seekFlags != "" {
		videoCmd.WriteString(seekFlags + " ")
	}
	videoCmd.WriteString("-i " + quotedPath + " ")
	if videoFilter != "" {
		scaleFilter = videoFilter + "," + scaleFilter
	}
	videoCmd.WriteString(fmt.Sprintf(
		"-fflags +nobuffer -flags low_delay -threads 0 -f rawvideo -r %d -pix_fmt yuv420p -vf %s -loglevel error pipe:1",
		videoDescription.Fps,
		strconv.Quote(scaleFilter),
	))

	videoDescription.Input = videoCmd.String()

	return ntgcalls.MediaDescription{
		Microphone: audioDescription,
		Camera:     videoDescription,
	}
}

func splitFilterFlags(filterFlags string) (videoFilter, audioFilter string) {
	fields := strings.Fields(filterFlags)
	for i := 0; i+1 < len(fields); i++ {
		switch fields[i] {
		case "-filter:v":
			videoFilter = fields[i+1]
			i++
		case "-filter:a":
			audioFilter = fields[i+1]
			i++
		}
	}
	return videoFilter, audioFilter
}

func shouldPaceInput(input string) bool {
	if !urlRegex.MatchString(input) {
		return true
	}

	parsed, err := url.Parse(input)
	if err != nil {
		return true
	}

	switch strings.ToLower(path.Ext(parsed.Path)) {
	case ".m3u", ".m3u8", ".mpd":
		return false
	default:
		return true
	}
}
