package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/chat"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/config"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/httpserver"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/telegram"
)

type App struct {
	cfg        config.Config
	logger     *slog.Logger
	poller     *telegram.Poller
	httpServer *httpserver.Server
	store      *store.Manager
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	tgClient := telegram.NewClient(cfg.Telegram.BotToken, logger)

	ollamaClient := ollama.NewClient(ollama.Config{
		BaseURL:     cfg.Ollama.BaseURL,
		Model:       cfg.Ollama.Model,
		TimeoutSec:  cfg.Ollama.TimeoutSec,
		KeepAlive:   cfg.Ollama.KeepAlive,
		NumCtx:      cfg.Ollama.NumCtx,
		Temperature: cfg.Ollama.Temperature,
		TopP:        cfg.Ollama.TopP,
	}, logger)

	conversationStore, err := store.New(ctx, store.Config{
		PostgresDSN: cfg.Storage.PostgresDSN,
		RedisURL:    cfg.Storage.RedisURL,
	}, logger)
	if err != nil {
		return nil, err
	}

	chatService := chat.NewService(chat.Config{
		DefaultMode:      chat.ParseMode(cfg.Chat.DefaultMode),
		RecentTurnLimit:  cfg.Chat.RecentTurnLimit,
		ResponseMaxChars: cfg.Chat.ResponseMaxChars,
	}, tgClient, ollamaClient, conversationStore, logger)

	poller := telegram.NewPoller(telegram.PollingConfig{
		TimeoutSec:     cfg.Telegram.PollTimeoutSec,
		Limit:          cfg.Telegram.PollLimit,
		AllowedUpdates: cfg.Telegram.AllowedUpdates,
	}, tgClient, chatService, logger)

	httpServer := httpserver.New(cfg.App.Port, logger)

	return &App{
		cfg:        cfg,
		logger:     logger,
		poller:     poller,
		httpServer: httpServer,
		store:      conversationStore,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.store.Close()

	a.logger.Info(
		"starting telegram long polling bot",
		"app", a.cfg.App.Name,
		"mode", a.cfg.Chat.DefaultMode,
		"ollama_model", a.cfg.Ollama.Model,
	)

	errCh := make(chan error, 2)

	go func() {
		errCh <- a.httpServer.Run(ctx)
	}()

	go func() {
		errCh <- a.poller.Run(ctx)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed) {
				continue
			}
			return err
		}
	}
}
