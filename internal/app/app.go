package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/chat"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/config"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/holiday"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/httpserver"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/proactive"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/telegram"
)

type App struct {
	cfg        config.Config
	logger     *slog.Logger
	poller     *telegram.Poller
	httpServer *httpserver.Server
	proactive  *proactive.Scheduler
	holiday    *holiday.Syncer
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

	reminderLLM := buildVariantLLM(
		cfg.Ollama,
		cfg.Proactive.ReminderModel,
		cfg.Proactive.ReminderBaseURL,
		func(base ollama.Config) ollama.Config {
			base.TimeoutSec = maxInt(10, minInt(base.TimeoutSec, 20))
			base.KeepAlive = "5m"
			base.NumCtx = minInt(base.NumCtx, 1024)
			base.Temperature = 0.8
			return base
		},
		logger,
	)

	structuredLLM := buildVariantLLM(
		cfg.Ollama,
		cfg.Chat.StructuredModel,
		cfg.Chat.StructuredBaseURL,
		func(base ollama.Config) ollama.Config {
			base.TimeoutSec = maxInt(15, minInt(base.TimeoutSec, 240))
			base.KeepAlive = "3m"
			base.NumCtx = minInt(base.NumCtx, 768)
			base.Temperature = 0.1
			base.TopP = 0.8
			return base
		},
		logger,
	)

	memorySlotLLM := buildVariantLLM(
		cfg.Ollama,
		cfg.Chat.MemorySlotModel,
		cfg.Chat.MemorySlotBaseURL,
		func(base ollama.Config) ollama.Config {
			base.TimeoutSec = maxInt(15, minInt(base.TimeoutSec, 240))
			base.KeepAlive = "3m"
			base.NumCtx = minInt(base.NumCtx, 1024)
			base.Temperature = 0.2
			base.TopP = 0.8
			return base
		},
		logger,
	)

	var stateReviewLLM chat.LLM
	if cfg.Chat.StateReviewEnabled {
		stateReviewLLM = buildVariantLLM(
			cfg.Ollama,
			coalesce(cfg.Chat.StateReviewModel, cfg.Chat.StructuredModel),
			coalesce(cfg.Chat.StateReviewBaseURL, cfg.Chat.StructuredBaseURL),
			func(base ollama.Config) ollama.Config {
				base.TimeoutSec = maxInt(8, minInt(base.TimeoutSec, 20))
				base.KeepAlive = "2m"
				base.NumCtx = minInt(base.NumCtx, 768)
				base.Temperature = 0.1
				base.TopP = 0.7
				return base
			},
			logger,
		)
	}

	conversationStore, err := store.New(ctx, store.Config{
		PostgresDSN: cfg.Storage.PostgresDSN,
		RedisURL:    cfg.Storage.RedisURL,
	}, logger)
	if err != nil {
		return nil, err
	}

	chatService := chat.NewService(chat.Config{
		RecentTurnLimit:           cfg.Chat.RecentTurnLimit,
		SummaryTriggerMessages:    cfg.Chat.SummaryTriggerMessages,
		ResponseMaxChars:          cfg.Chat.ResponseMaxChars,
		PhaseRulesPath:            cfg.Chat.PhaseRulesPath,
		StateReviewEnabled:        cfg.Chat.StateReviewEnabled,
		StateReviewTimeoutMs:      cfg.Chat.StateReviewTimeoutMs,
		StateReviewWorkers:        cfg.Chat.StateReviewWorkers,
		StateReviewQueueSize:      cfg.Chat.StateReviewQueueSize,
		StructuredExtractEnabled:  cfg.Chat.StructuredExtract,
		StructuredExtractMinChars: cfg.Chat.StructuredMinChars,
		MemorySlotEnabled:         cfg.Chat.MemorySlotEnabled,
		MemorySlotMinChars:        cfg.Chat.MemorySlotMinChars,
		MemorySlotSyncTimeoutMs:   cfg.Chat.MemorySlotSyncTimeoutMs,
		MemorySlotAsyncTimeoutMs:  cfg.Chat.MemorySlotAsyncTimeoutMs,
		MemorySlotWorkers:         cfg.Chat.MemorySlotWorkers,
		MemorySlotQueueSize:       cfg.Chat.MemorySlotQueueSize,
	}, tgClient, ollamaClient, reminderLLM, structuredLLM, memorySlotLLM, stateReviewLLM, conversationStore, logger)

	holidayResolver := holiday.NewResolver(conversationStore, holiday.ResolverConfig{
		LookaheadDays:   cfg.Holiday.LookaheadDays,
		TodayPercent:    cfg.Holiday.PromptTodayPct,
		UpcomingPercent: cfg.Holiday.PromptUpcomingPct,
	}, logger)
	chatService.SetHolidayResolver(holidayResolver)

	poller := telegram.NewPoller(telegram.PollingConfig{
		TimeoutSec:     cfg.Telegram.PollTimeoutSec,
		Limit:          cfg.Telegram.PollLimit,
		AllowedUpdates: cfg.Telegram.AllowedUpdates,
	}, tgClient, chatService, logger)

	httpServer := httpserver.New(cfg.App.Port, logger)

	proactiveScheduler := proactive.NewScheduler(proactive.Config{
		Enabled:                 cfg.Proactive.Enabled,
		EnableReconnect:         true,
		EnableEventFollowup:     true,
		EnableMoodRepair:        true,
		EnableHabitPing:         true,
		RecentConversationLimit: cfg.Chat.RecentTurnLimit,
		FeedbackInterval:        time.Duration(cfg.Proactive.FeedbackIntervalSec) * time.Second,
		DecisionScanInterval:    time.Duration(cfg.Proactive.ScanIntervalSec) * time.Second,
		ReminderScanInterval:    time.Duration(cfg.Proactive.ReminderScanIntervalSec) * time.Second,
		TimezoneName:            cfg.Proactive.DefaultTimezone,
	}, newProactiveRepository(conversationStore), tgClient, ollamaClient, reminderLLM, logger)
	proactiveScheduler.SetHolidayResolver(holidayResolver)

	var holidaySyncer *holiday.Syncer
	if cfg.Holiday.SyncEnabled {
		holidayClient := holiday.NewClient(cfg.Holiday.APIServiceKey, 15*time.Second)
		holidayClient.SetBaseURL(cfg.Holiday.APIBaseURL)
		holidaySyncer = holiday.NewSyncer(
			conversationStore,
			holidayClient,
			time.Duration(cfg.Holiday.SyncIntervalHours)*time.Hour,
			logger,
		)
	}

	return &App{
		cfg:        cfg,
		logger:     logger,
		poller:     poller,
		httpServer: httpServer,
		proactive:  proactiveScheduler,
		holiday:    holidaySyncer,
		store:      conversationStore,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.store.Close()

	a.logger.Info(
		"starting telegram long polling bot",
		"app", a.cfg.App.Name,
		"ollama_model", a.cfg.Ollama.Model,
		"reminder_model", coalesce(a.cfg.Proactive.ReminderModel, a.cfg.Ollama.Model),
		"structured_model", coalesce(a.cfg.Chat.StructuredModel, a.cfg.Ollama.Model),
		"memory_slot_model", coalesce(a.cfg.Chat.MemorySlotModel, a.cfg.Ollama.Model),
		"state_review_model", coalesce(coalesce(a.cfg.Chat.StateReviewModel, a.cfg.Chat.StructuredModel), a.cfg.Ollama.Model),
	)

	errCh := make(chan error, 4)

	go func() {
		errCh <- a.httpServer.Run(ctx)
	}()

	go func() {
		errCh <- a.poller.Run(ctx)
	}()

	if a.proactive != nil {
		go func() {
			errCh <- a.proactive.Run(ctx)
		}()
	}

	if a.holiday != nil {
		go func() {
			errCh <- a.holiday.Run(ctx)
		}()
	}

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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func coalesce(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func buildVariantLLM(baseCfg config.OllamaConfig, model, baseURL string, tune func(ollama.Config) ollama.Config, logger *slog.Logger) chat.LLM {
	model = coalesce(model, baseCfg.Model)
	baseURL = coalesce(baseURL, baseCfg.BaseURL)

	cfg := ollama.Config{
		BaseURL:     baseURL,
		Model:       model,
		TimeoutSec:  baseCfg.TimeoutSec,
		KeepAlive:   baseCfg.KeepAlive,
		NumCtx:      baseCfg.NumCtx,
		Temperature: baseCfg.Temperature,
		TopP:        baseCfg.TopP,
	}
	if tune != nil {
		cfg = tune(cfg)
	}
	return ollama.NewClient(cfg, logger)
}
