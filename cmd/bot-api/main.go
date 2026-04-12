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
		logger.Error("설정값이 올바르지 않습니다.", "원인", logx.KoreanError(err))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("애플리케이션 초기화에 실패했습니다.", "원인", logx.KoreanError(err))
		os.Exit(1)
	}

	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("애플리케이션이 오류로 종료되었습니다.", "원인", logx.KoreanError(err))
		os.Exit(1)
	}
}
