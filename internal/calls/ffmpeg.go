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
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var urlRegex = regexp.MustCompile(`^https?://`)

const (
	maxVideoWidth  = 1066
	maxVideoHeight = 600
)

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
	audioCmd.WriteString("-i " + quotedPath + " -map 0:a:0 ")
	if audioFilter != "" {
		audioCmd.WriteString("-af " + strconv.Quote(audioFilter) + " ")
	}
	audioCmd.WriteString(fmt.Sprintf(
		"-fflags +nobuffer -flags low_delay -threads 2 -f s16le -ac %d -ar %d -loglevel error pipe:1",
		audioDescription.ChannelCount,
		audioDescription.SampleRate,
	))

	audioDescription.Input = audioCmd.String()

	if !isVideo {
		return ntgcalls.MediaDescription{
			Microphone: audioDescription,
		}
	}

	width, height := maxVideoWidth, maxVideoHeight
	if !isURL {
		if sourceWidth, sourceHeight := getVideoDimensions(filePath); sourceWidth > 0 && sourceHeight > 0 {
			width, height = fitVideoDimensions(sourceWidth, sourceHeight, width, height)
		}
	}

	videoDescription := &ntgcalls.VideoDescription{
		MediaSource: ntgcalls.MediaSourceShell,
		Width:       int16(width),
		Height:      int16(height),
		Fps:         30,
	}

	scaleFilter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease:force_divisible_by=2:flags=fast_bilinear,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1",
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
	videoCmd.WriteString("-i " + quotedPath + " -map 0:v:0 ")
	if videoFilter != "" {
		scaleFilter = videoFilter + "," + scaleFilter
	}
	videoCmd.WriteString(fmt.Sprintf(
		"-fflags +nobuffer -flags low_delay -threads 2 -f rawvideo -r %d -pix_fmt yuv420p -vf %s -loglevel error pipe:1",
		videoDescription.Fps,
		strconv.Quote(scaleFilter),
	))

	videoDescription.Input = videoCmd.String()

	return ntgcalls.MediaDescription{
		Microphone: audioDescription,
		Screen:     videoDescription,
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

func fitVideoDimensions(sourceWidth, sourceHeight, maxWidth, maxHeight int) (int, int) {
	scale := min(1, float64(maxWidth)/float64(sourceWidth), float64(maxHeight)/float64(sourceHeight))
	width := int(math.Round(float64(sourceWidth) * scale))
	height := int(math.Round(float64(sourceHeight) * scale))
	width -= width % 2
	height -= height % 2
	return max(width, 2), max(height, 2)
}

func getVideoDimensions(filePath string) (int, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=s=x:p=0", filePath).Output()
	if err != nil {
		slog.Warn("Failed to probe video dimensions", "path", filePath, "error", err)
		return 0, 0
	}

	dimensions := strings.Split(strings.TrimSpace(string(output)), "x")
	if len(dimensions) != 2 {
		slog.Warn("Invalid video dimensions from ffprobe", "path", filePath, "output", strings.TrimSpace(string(output)))
		return 0, 0
	}

	width, widthErr := strconv.Atoi(dimensions[0])
	height, heightErr := strconv.Atoi(dimensions[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		slog.Warn("Invalid video dimensions from ffprobe", "path", filePath, "output", strings.TrimSpace(string(output)))
		return 0, 0
	}

	return width, height
}
