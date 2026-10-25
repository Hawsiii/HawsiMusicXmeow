/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package main

import (
	"ashokshau/tgmusic/internal/bot"
	"ashokshau/tgmusic/internal/calls"
	"ashokshau/tgmusic/internal/cleanup"
	"ashokshau/tgmusic/internal/config"
	"ashokshau/tgmusic/internal/db"
	"ashokshau/tgmusic/internal/downloader"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/AshokShau/gotdbot"
)

//go:generate go run github.com/AshokShau/gotdbot/scripts/tools

// main serves as the entry point for the application.
func main() {
	logger := slog.New(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: true,
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				switch a.Key {
				case slog.TimeKey:
					a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05"))
				case slog.SourceKey:
					if source, ok := a.Value.Any().(*slog.Source); ok {
						a.Value = slog.StringValue(fmt.Sprintf("%s:%d", filepath.Base(source.File), source.Line))
					}
				}
				return a
			},
		}),
	)
	slog.SetDefault(logger)

	if err := config.LoadEnv(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	if err := db.InitDatabase(); err != nil {
		slog.Error("failed to connect database", "error", err)
		os.Exit(1)
	}

	tdDir := "database"
	_ = os.Remove(tdDir)
	libPaths, err := filepath.Glob("./libtdjson.so.*")
	if err != nil || len(libPaths) == 0 {
		slog.Error("TDLib shared library not found", "pattern", "./libtdjson.so.*", "error", err)
		os.Exit(1)
	}
	libPath, err := filepath.Abs(libPaths[0])
	if err != nil {
		slog.Error("Failed to resolve TDLib shared library path", "path", libPaths[0], "error", err)
		os.Exit(1)
	}
	manager := gotdbot.NewClientManager(libPath)

	clientConfig := gotdbot.DefaultClientConfig()
	clientConfig.AutoRetry = &gotdbot.AutoRetry{
		ChatNotFound: true,
	}

	clientConfig.DatabaseDirectory = tdDir
	client, err := manager.RegisterClient(config.ApiId, config.ApiHash, config.Token, clientConfig)
	if err != nil {
		slog.Error("manager.RegisterClient error", "error", err)
		os.Exit(1)
	}

	if config.DlBotToken != "" {
		dlClientConfig := gotdbot.DefaultClientConfig()
		dlClientConfig.AutoRetry = &gotdbot.AutoRetry{
			ChatNotFound: true,
		}
		dlClientConfig.DatabaseDirectory = tdDir + "_dl"
		_ = os.Remove(dlClientConfig.DatabaseDirectory)

		dlClient, err := manager.RegisterClient(config.ApiId, config.ApiHash, config.DlBotToken, dlClientConfig)
		if err != nil {
			slog.Error("manager.RegisterClient (DL) error; using main client", "error", err)
			downloader.DlBot = client
		} else {
			downloader.DlBot = dlClient
			dlClient.Logger.Info("Download bot registered successfully")
		}
	}

	for i, session := range config.SessionStrings {
		err = calls.Calls.StartClient(config.ApiId, config.ApiHash, session, fmt.Sprintf("_%d", i))
		if err != nil {
			slog.Error("failed to start assistant client", "index", i, "error", err)
			os.Exit(1)
		}
	}

	calls.Calls.RegisterHandlers(client)
	cleanupScheduler, stopCleanup := cleanup.Start(client)
	defer stopCleanup()
	bot.LoadModules(client, cleanupScheduler.CancelChat)
	msg := "The bot has started!"
	client.Logger.Info(msg)
	_, _ = client.SendMessage(config.LoggerId, msg, nil)
	manager.Idle()
	client.Logger.Info("The bot is shutting down...")
	calls.Calls.StopAllClients()
}
