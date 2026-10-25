/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package bot

import (
	"ashokshau/tgmusic/internal/cache"
	"ashokshau/tgmusic/internal/calls"
	"fmt"
	"strconv"

	td "github.com/AshokShau/gotdbot"
)

func speedHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	chatID := m.ChatId
	if !cache.ChatCache.IsActive(chatID) || cache.ChatCache.GetPlayingTrack(chatID) == nil {
		_, err := m.ReplyText(c, "The bot is not streaming in the video chat.", nil)
		return err
	}

	args := Args(m)
	if args == "" {
		_, err := m.ReplyText(c, "<b>Change Playback Speed</b>\n\n<b>Usage:</b> <code>/speed [value]</code>\n\nThe speed can be set between <code>0.5</code> and <code>4.0</code>.", &td.SendMessageOpts{ParseMode: "HTML"})
		return err
	}

	speed, err := strconv.ParseFloat(args, 64)
	if err != nil || speed < 0.5 || speed > 4.0 {
		_, err = m.ReplyText(c, "Invalid speed value. Please provide a number between 0.5 and 4.0.", nil)
		return err
	}

	if err := calls.Calls.ChangeSpeed(c, chatID, speed); err != nil {
		_, replyErr := m.ReplyText(c, fmt.Sprintf("An error occurred while changing the speed: %s", err.Error()), nil)
		return replyErr
	}

	_, err = m.ReplyText(c, fmt.Sprintf("Playback speed has been set to <code>%.2fx</code>.", speed), &td.SendMessageOpts{ParseMode: "HTML"})
	return err
}
