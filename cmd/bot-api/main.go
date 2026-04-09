package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/app"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/config"
	"github.com/hsvtr365/telegram_romance_AI_bot/pkg/logx"
)

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		panic(err)
	}

	logger := logx.New(cfg.App.Env)

	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("application stopped with error", "error", err)
		os.Exit(1)
	}
}
