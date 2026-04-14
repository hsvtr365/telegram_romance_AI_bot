package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type MemorySlotConfig struct {
	Enabled         bool
	MinChars        int
	SyncTimeout     time.Duration
	AsyncTimeout    time.Duration
	Workers         int
	QueueSize       int
	RecentTurnLimit int
}

type MemorySlotAnalyzer struct {
	cfg           MemorySlotConfig
	llm           LLM
	store         *store.Manager
	promptBuilder *MemorySlotPromptBuilder
	logger        *slog.Logger
	runner        *AsyncRunner
}

type memorySlotJob struct {
	sessionID       int64
	sourceMessageID int64
	input           string
}

func NewMemorySlotAnalyzer(cfg MemorySlotConfig, llm LLM, conversationStore *store.Manager, runner *AsyncRunner, logger *slog.Logger) *MemorySlotAnalyzer {
	if !cfg.Enabled || llm == nil || conversationStore == nil {
		return nil
	}
	if cfg.MinChars <= 0 {
		cfg.MinChars = 16
	}
	if cfg.SyncTimeout <= 0 {
		cfg.SyncTimeout = 20 * time.Second
	}
	if cfg.AsyncTimeout <= 0 {
		cfg.AsyncTimeout = 300 * time.Second
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 32
	}
	if cfg.RecentTurnLimit <= 0 {
		cfg.RecentTurnLimit = 14
	}

	analyzer := &MemorySlotAnalyzer{
		cfg:           cfg,
		llm:           llm,
		store:         conversationStore,
		promptBuilder: NewMemorySlotPromptBuilder(),
		logger:        logger,
		runner: runner,
	}

	return analyzer
}

func (a *MemorySlotAnalyzer) SyncAnalyze(ctx context.Context, input MemorySlotAnalyzeInput) (*model.MemorySlotAnalysisResult, error) {
	if a == nil || a.llm == nil {
		return nil, nil
	}
	if !shouldAnalyzeMemorySlots(input.CurrentUserInput, a.cfg.MinChars) {
		return nil, nil
	}

	start := time.Now()
	syncCtx, cancel := context.WithTimeout(ctx, a.cfg.SyncTimeout)
	defer cancel()

	result, err := a.analyze(syncCtx, input)
	if a.logger != nil {
		a.logger.Debug(
			"memory slot sync analyze completed",
			"metric", "memory_slot.sync_ms",
			"duration_ms", time.Since(start).Milliseconds(),
			"session_id", input.Snapshot.SessionID,
			"success", err == nil && result != nil,
		)
	}
	return result, err
}

func (a *MemorySlotAnalyzer) AsyncEnqueue(sessionID int64, sourceMessageID int64, input string) {
	if a == nil || a.runner == nil || sessionID == 0 || sourceMessageID == 0 {
		return
	}

	job := memorySlotJob{
		sessionID:       sessionID,
		sourceMessageID: sourceMessageID,
		input:           input,
	}

	a.runner.Enqueue(AsyncTask{
		Key:     memorySlotTaskKey(sessionID),
		Version: sourceMessageID,
		Build: func(ctx context.Context) (func(context.Context) error, error) {
			return a.buildAsyncCommit(ctx, job)
		},
		OnEnqueue: func(queueDepth int) {
			if a.logger != nil {
				a.logger.Debug(
					"memory slot async enqueued",
					"metric", "memory_slot.queue_depth",
					"queue_depth", queueDepth,
					"session_id", sessionID,
					"source_message_id", sourceMessageID,
				)
			}
		},
		OnDrop: func(_ string, queueDepth int) {
			if a.logger != nil {
				a.logger.Warn(
					"memory slot async queue full",
					"metric", "memory_slot.queue_depth",
					"queue_depth", queueDepth,
					"session_id", sessionID,
					"source_message_id", sourceMessageID,
				)
			}
		},
		OnSuperseded: func(stage string) {
			if a.logger != nil {
				a.logger.Debug(
					"memory slot job superseded",
					"metric", "memory_slot.superseded_jobs",
					"stage", stage,
					"session_id", sessionID,
					"source_message_id", sourceMessageID,
				)
			}
		},
		OnComplete: func(duration time.Duration, err error, queueDepth int) {
			if err != nil {
				return
			}
			if a.logger != nil {
				a.logger.Debug(
					"memory slot async analyze completed",
					"metric", "memory_slot.async_ms",
					"duration_ms", duration.Milliseconds(),
					"session_id", sessionID,
					"source_message_id", sourceMessageID,
					"queue_depth", queueDepth,
				)
			}
		},
	})
}

func (a *MemorySlotAnalyzer) buildAsyncCommit(ctx context.Context, job memorySlotJob) (func(context.Context) error, error) {
	snapshot, err := a.store.BuildMemorySlotSnapshot(ctx, job.sessionID, a.cfg.RecentTurnLimit)
	if err != nil {
		if a.logger != nil {
			a.logger.Warn("failed to build memory slot snapshot", "session_id", job.sessionID, "error", err)
		}
		return nil, err
	}

	result, err := a.analyze(ctx, MemorySlotAnalyzeInput{
		Snapshot:         snapshot,
		CurrentUserInput: job.input,
	})
	if err != nil {
		if a.logger != nil {
			a.logger.Warn("memory slot async analyze failed", "session_id", job.sessionID, "source_message_id", job.sourceMessageID, "error", err)
		}
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	mergedTopics, _ := MergeMemorySlotAnalysis(snapshot.TopicSlots, snapshot.ConversationState, *result, job.sourceMessageID, time.Now().UTC())
	return func(commitCtx context.Context) error {
		if len(mergedTopics) > 0 {
			if _, err := a.store.UpsertTopicSlots(commitCtx, job.sessionID, mergedTopics); err != nil {
				if a.logger != nil {
					a.logger.Warn("failed to upsert topic slots", "session_id", job.sessionID, "error", err)
				}
				return err
			}
		}
		return nil
	}, nil
}

func (a *MemorySlotAnalyzer) analyze(ctx context.Context, input MemorySlotAnalyzeInput) (*model.MemorySlotAnalysisResult, error) {
	if a == nil || a.llm == nil {
		return nil, nil
	}

	prompt := a.promptBuilder.Build(input)
	raw, err := a.llm.Chat(ctx, prompt)
	if err != nil {
		if ctx.Err() != nil && a.logger != nil {
			a.logger.Debug("memory slot analyze timeout", "metric", "memory_slot.timeout_count", "error", ctx.Err())
		}
		return nil, err
	}

	result, err := parseMemorySlotAnalysisResult(raw)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func memorySlotTaskKey(sessionID int64) string {
	return fmt.Sprintf("memory_slot:%d", sessionID)
}

func parseMemorySlotAnalysisResult(raw string) (model.MemorySlotAnalysisResult, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return model.MemorySlotAnalysisResult{}, nil
	}

	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end >= start {
		trimmed = trimmed[start : end+1]
	} else {
		// No JSON skeleton found at all
		return model.MemorySlotAnalysisResult{}, nil
	}

	var result model.MemorySlotAnalysisResult
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return model.MemorySlotAnalysisResult{}, fmt.Errorf("decode memory slot analysis: %w (raw length: %d)", err, len(trimmed))
	}

	for idx := range result.Topics {
		result.Topics[idx].Label = cleanMemoryText(result.Topics[idx].Label)
		result.Topics[idx].Summary = cleanMemoryText(result.Topics[idx].Summary)
		result.Topics[idx].Status = normalizeTopicStatus(result.Topics[idx].Status)
		result.Topics[idx].Confidence = normalizeConfidence(result.Topics[idx].Confidence)
		if result.Topics[idx].Importance < 0 {
			result.Topics[idx].Importance = 0
		}
		if result.Topics[idx].Importance > 100 {
			result.Topics[idx].Importance = 100
		}
	}

	result.ConversationState.CurrentStage = normalizeCurrentStage(result.ConversationState.CurrentStage)
	result.ConversationState.StageDirection = normalizeStageDirection(result.ConversationState.StageDirection)
	result.ConversationState.EmotionalTone = cleanMemoryText(result.ConversationState.EmotionalTone)
	result.ConversationState.InteractionMode = cleanMemoryText(result.ConversationState.InteractionMode)
	result.ConversationState.OpenLoopSummary = cleanMemoryText(result.ConversationState.OpenLoopSummary)
	result.ConversationState.FocusTopicLabel = cleanMemoryText(result.ConversationState.FocusTopicLabel)
	result.ConversationState.Confidence = normalizeConfidence(result.ConversationState.Confidence)

	return result, nil
}
