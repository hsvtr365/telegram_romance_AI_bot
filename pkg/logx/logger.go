package logx

import (
	"log/slog"
	"os"
	"strings"
)

func New(env string) *slog.Logger {
	options := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	if strings.EqualFold(strings.TrimSpace(env), "prod") || strings.EqualFold(strings.TrimSpace(env), "production") {
		return slog.New(slog.NewJSONHandler(os.Stdout, options))
	}

	return slog.New(NewPrettyHandler(os.Stdout, options))
}
