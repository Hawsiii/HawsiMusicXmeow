package vc

import (
	"ashokshau/tgmusic/src/vc/ntgcalls"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var isURLRegex = regexp.MustCompile(`^https?://`)

// getMediaDescription creates a media description for ntgcalls based on the provided file path, video status, and ffmpeg parameters.
func getMediaDescription(filePath string, isVideo bool, ffmpegParameters string) ntgcalls.MediaDescription {
	audioDescription := &ntgcalls.AudioDescription{
		MediaSource:  ntgcalls.MediaSourceShell,
		SampleRate:   48000,
		ChannelCount: 2,
	}

	quotedPath := strconv.Quote(filePath)
	isURL := isURLRegex.MatchString(filePath)

	var audioCmd strings.Builder
	audioCmd.WriteString("ffmpeg ")
	if isURL {
		audioCmd.WriteString("-reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1 -reconnect_delay_max 2 ")
	}

	var seekFlags, filterFlags string
	if ffmpegParameters != "" {
		if strings.Contains(ffmpegParameters, "filter:") {
			filterFlags = ffmpegParameters
		} else {
			seekFlags = ffmpegParameters
		}
	}

	if seekFlags != "" {
		audioCmd.WriteString(seekFlags + " ")
	}

	audioCmd.WriteString("-i " + quotedPath + " ")
	if filterFlags != "" {
		audioCmd.WriteString(filterFlags + " ")
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

	var scaleFilter string
	// Scale to cover the target area then center-crop to 1280x720 so the video fills the frame
	scaleFilter = fmt.Sprintf(
		"scale='if(gt(a,%d/%d),-2,%d)':'if(gt(a,%d/%d),%d,-2)':flags=lanczos,crop=%d:%d",
		width,
		height,
		width,
		width,
		height,
		height,
		width,
		height,
	)

	var videoCmd strings.Builder
	videoCmd.WriteString("ffmpeg -hide_banner ")

	if isURL {
		videoCmd.WriteString("-reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1 -reconnect_delay_max 2 ")
	}

	if seekFlags != "" {
		videoCmd.WriteString(seekFlags + " ")
	}

	videoCmd.WriteString(fmt.Sprintf("-i %s ", quotedPath))
	if filterFlags != "" {
		videoCmd.WriteString(filterFlags + " ")
	}

	videoCmd.WriteString(fmt.Sprintf(
		"-fflags +nobuffer -flags low_delay -threads 0 -f rawvideo -r %d -pix_fmt yuv420p -vf \"%s\" -loglevel error pipe:1",
		videoDescription.Fps,
		scaleFilter,
	))
	videoDescription.Input = videoCmd.String()

	return ntgcalls.MediaDescription{
		Microphone: audioDescription,
		Camera:     videoDescription,
	}
}
