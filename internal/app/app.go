package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
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
	cfg           config.Config
	logger        *slog.Logger
	poller        *telegram.Poller
	httpServer    *httpserver.Server
	proactive     *proactive.Scheduler
	holiday       *holiday.Syncer
	store         *store.Manager
	modelMonitors []backgroundRunner
}

type backgroundRunner interface {
	Run(ctx context.Context) error
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	tgClient := telegram.NewClient(cfg.Telegram.BotToken, logger)

	ollamaClient, mainMonitor := buildPrimaryLLM(cfg.Ollama, logger)
	modelMonitors := collectRunner(mainMonitor)

	reminderLLM, reminderMonitor := buildVariantLLM(
		cfg.Ollama,
		cfg.Proactive.ReminderModel,
		cfg.Proactive.ReminderBaseURL,
		func(base ollama.Config) ollama.Config {
			// Use base.TimeoutSec directly from config
			base.KeepAlive = "5m"
			base.NumCtx = minInt(base.NumCtx, 1024)
			base.Temperature = 0.8
			return base
		},
		logger,
	)
	modelMonitors = appendRunner(modelMonitors, reminderMonitor)

	structuredLLM, structuredMonitor := buildVariantLLM(
		cfg.Ollama,
		cfg.Chat.StructuredModel,
		cfg.Chat.StructuredBaseURL,
		func(base ollama.Config) ollama.Config {
			base.TimeoutSec = maxInt(120, minInt(base.TimeoutSec, 240))
			base.KeepAlive = "3m"
			base.NumCtx = minInt(base.NumCtx, 768)
			base.Temperature = 0.1
			base.TopP = 0.8
			return base
		},
		logger,
	)
	modelMonitors = appendRunner(modelMonitors, structuredMonitor)

	memorySlotLLM, memorySlotMonitor := buildVariantLLM(
		cfg.Ollama,
		cfg.Chat.MemorySlotModel,
		cfg.Chat.MemorySlotBaseURL,
		func(base ollama.Config) ollama.Config {
			base.TimeoutSec = maxInt(120, minInt(base.TimeoutSec, 240))
			base.KeepAlive = "3m"
			base.NumCtx = minInt(base.NumCtx, 1024)
			base.Temperature = 0.2
			base.TopP = 0.8
			return base
		},
		logger,
	)
	modelMonitors = appendRunner(modelMonitors, memorySlotMonitor)

	var stateReviewLLM chat.LLM
	if cfg.Chat.StateReviewEnabled {
		var stateReviewMonitor backgroundRunner
		stateReviewLLM, stateReviewMonitor = buildVariantLLM(
			cfg.Ollama,
			coalesce(cfg.Chat.StateReviewModel, cfg.Chat.StructuredModel),
			coalesce(cfg.Chat.StateReviewBaseURL, cfg.Chat.StructuredBaseURL),
			func(base ollama.Config) ollama.Config {
				// Use base.TimeoutSec directly from config
				base.KeepAlive = "2m"
				base.NumCtx = minInt(base.NumCtx, 768)
				base.Temperature = 0.1
				base.TopP = 0.7
				return base
			},
			logger,
		)
		modelMonitors = appendRunner(modelMonitors, stateReviewMonitor)
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
		cfg:           cfg,
		logger:        logger,
		poller:        poller,
		httpServer:    httpServer,
		proactive:     proactiveScheduler,
		holiday:       holidaySyncer,
		store:         conversationStore,
		modelMonitors: modelMonitors,
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

	for _, monitor := range a.modelMonitors {
		if monitor == nil {
			continue
		}
		go func(runner backgroundRunner) {
			errCh <- runner.Run(ctx)
		}(monitor)
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

func buildPrimaryLLM(baseCfg config.OllamaConfig, logger *slog.Logger) (chat.LLM, backgroundRunner) {
	endpoints := baseCfg.Endpoints
	if len(endpoints) == 0 {
		endpoints = []config.OllamaEndpoint{{
			BaseURL: baseCfg.BaseURL,
			Model:   baseCfg.Model,
		}}
	}

	clients := make([]*ollama.Client, 0, len(endpoints))
	for _, endpoint := range endpoints {
		cfg := ollama.Config{
			BaseURL:     endpoint.BaseURL,
			Model:       endpoint.Model,
			TimeoutSec:  baseCfg.TimeoutSec,
			KeepAlive:   baseCfg.KeepAlive,
			NumCtx:      baseCfg.NumCtx,
			Temperature: baseCfg.Temperature,
			TopP:        baseCfg.TopP,
		}
		clients = append(clients, ollama.NewClient(cfg, logger))
	}

	pool := ollama.NewEndpointPoolClient(
		clients,
		time.Duration(baseCfg.HealthCheckIntervalSec)*time.Second,
		logger,
	)

	return pool, pool
}

func buildVariantLLM(baseCfg config.OllamaConfig, model, baseURL string, tune func(ollama.Config) ollama.Config, logger *slog.Logger) (chat.LLM, backgroundRunner) {
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
	baseURLs := resolveVariantBaseURLs(baseCfg, cfg.BaseURL, model != baseCfg.Model)

	clients := make([]*ollama.Client, 0, len(baseURLs))
	for _, currentBaseURL := range baseURLs {
		clientCfg := cfg
		clientCfg.BaseURL = currentBaseURL
		clients = append(clients, ollama.NewClient(clientCfg, logger))
	}

	pool := ollama.NewEndpointPoolClient(
		clients,
		time.Duration(baseCfg.HealthCheckIntervalSec)*time.Second,
		logger,
	)

	return pool, pool
}

func resolveVariantBaseURLs(baseCfg config.OllamaConfig, preferredBaseURL string, forceSingle bool) []string {
	if preferredBaseURL == "" {
		return baseCfg.BaseURLs
	}
	if forceSingle {
		return []string{preferredBaseURL}
	}

	if len(baseCfg.BaseURLs) == 0 {
		return []string{preferredBaseURL}
	}

	if !containsBaseURL(baseCfg.BaseURLs, preferredBaseURL) {
		return []string{preferredBaseURL}
	}

	return moveBaseURLToFront(baseCfg.BaseURLs, preferredBaseURL)
}

func containsBaseURL(values []string, target string) bool {
	target = normalizeBaseURL(target)
	for _, value := range values {
		if normalizeBaseURL(value) == target {
			return true
		}
	}
	return false
}

func moveBaseURLToFront(values []string, target string) []string {
	target = normalizeBaseURL(target)
	out := make([]string, 0, len(values))
	var front string
	for _, value := range values {
		if normalizeBaseURL(value) == target {
			front = value
			continue
		}
		out = append(out, value)
	}
	if front != "" {
		out = append([]string{front}, out...)
	}
	return out
}

func normalizeBaseURL(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimRight(value, "/")
}

func collectRunner(runner backgroundRunner) []backgroundRunner {
	if runner == nil {
		return nil
	}
	return []backgroundRunner{runner}
}

func appendRunner(runners []backgroundRunner, runner backgroundRunner) []backgroundRunner {
	if runner == nil {
		return runners
	}
	return append(runners, runner)
}
