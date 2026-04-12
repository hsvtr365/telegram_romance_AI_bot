package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/holiday"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/promptutil"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/telegram"
	"github.com/hsvtr365/telegram_romance_AI_bot/pkg/logx"
)

type Messenger interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	SendChatAction(ctx context.Context, chatID int64, action string) error
}

type Config struct {
	RecentTurnLimit           int
	SummaryTriggerMessages    int
	ResponseMaxChars          int
	PhaseRulesPath            string
	StateReviewEnabled        bool
	StateReviewTimeoutMs      int
	StateReviewWorkers        int
	StateReviewQueueSize      int
	StructuredExtractEnabled  bool
	StructuredExtractMinChars int
	MemorySlotEnabled         bool
	MemorySlotMinChars        int
	MemorySlotSyncTimeoutMs   int
	MemorySlotAsyncTimeoutMs  int
	MemorySlotWorkers         int
	MemorySlotQueueSize       int
}

type Service struct {
	cfg                  Config
	bot                  Messenger
	llm                  LLM
	reminderLLM          LLM
	store                *store.Manager
	prompt               *PromptBuilder
	fastState            *FastStateEvaluator
	stateReviewer        *AsyncStateReviewer
	stateReviewRunner    *AsyncRunner
	profileBatchReviewer *AsyncProfileBatchReviewer
	profileReviewRunner  *AsyncRunner
	extractor            *StructuredExtractor
	memoryAnalyzer       *MemorySlotAnalyzer
	structuredRunner     *AsyncRunner
	holidayResolver      holiday.ContextResolver
	logger               *slog.Logger

	generationCoordinator *GenerationCoordinator
	chatLocksMu           sync.Mutex
	chatLocks             map[int64]*sync.Mutex
}

func NewService(cfg Config, bot Messenger, llm LLM, reminderLLM LLM, structuredLLM LLM, memorySlotLLM LLM, stateReviewLLM LLM, conversationStore *store.Manager, logger *slog.Logger) *Service {
	if cfg.RecentTurnLimit <= 0 {
		cfg.RecentTurnLimit = 30
	}
	if cfg.StructuredExtractMinChars <= 0 {
		cfg.StructuredExtractMinChars = 1
	}
	if cfg.MemorySlotMinChars <= 0 {
		cfg.MemorySlotMinChars = 5
	}
	if cfg.MemorySlotSyncTimeoutMs < 0 {
		cfg.MemorySlotSyncTimeoutMs = 0
	}
	if cfg.MemorySlotAsyncTimeoutMs <= 0 {
		cfg.MemorySlotAsyncTimeoutMs = 90000
	}
	if cfg.MemorySlotWorkers <= 0 {
		cfg.MemorySlotWorkers = 2
	}
	if cfg.MemorySlotQueueSize <= 0 {
		cfg.MemorySlotQueueSize = 32
	}
	if cfg.StateReviewTimeoutMs <= 0 {
		cfg.StateReviewTimeoutMs = 1200
	}
	if cfg.StateReviewWorkers <= 0 {
		cfg.StateReviewWorkers = 2
	}
	if cfg.StateReviewQueueSize <= 0 {
		cfg.StateReviewQueueSize = 32
	}

	var extractor *StructuredExtractor
	if cfg.StructuredExtractEnabled && structuredLLM != nil {
		extractor = NewStructuredExtractor(structuredLLM, cfg.StructuredExtractMinChars)
	}

	var structuredRunner *AsyncRunner
	if extractor != nil && conversationStore != nil {
		structuredRunner = NewAsyncRunner(AsyncRunnerConfig{
			Workers:   structuredExtractWorkers,
			QueueSize: structuredExtractQueueSize,
			Timeout:   structuredExtractTimeout,
		}, logger)
	}

	var profileBatchReviewer *AsyncProfileBatchReviewer
	var profileReviewRunner *AsyncRunner
	if extractor != nil && conversationStore != nil {
		profileBatchReviewer = NewAsyncProfileBatchReviewer(structuredLLM)
		profileReviewRunner = NewAsyncRunner(AsyncRunnerConfig{
			Workers:   profileBatchReviewWorkers,
			QueueSize: profileBatchReviewQueueSize,
			Timeout:   profileBatchReviewTimeout,
		}, logger)
	}

	var memoryAnalyzer *MemorySlotAnalyzer
	if cfg.MemorySlotEnabled && memorySlotLLM != nil {
		memoryAnalyzer = NewMemorySlotAnalyzer(MemorySlotConfig{
			Enabled:         cfg.MemorySlotEnabled,
			MinChars:        cfg.MemorySlotMinChars,
			SyncTimeout:     time.Duration(cfg.MemorySlotSyncTimeoutMs) * time.Millisecond,
			AsyncTimeout:    time.Duration(cfg.MemorySlotAsyncTimeoutMs) * time.Millisecond,
			Workers:         cfg.MemorySlotWorkers,
			QueueSize:       cfg.MemorySlotQueueSize,
			RecentTurnLimit: cfg.RecentTurnLimit,
		}, memorySlotLLM, conversationStore, logger)
	}

	fastState := NewFastStateEvaluator(cfg.PhaseRulesPath, logger)

	var reviewer *AsyncStateReviewer
	if cfg.StateReviewEnabled && stateReviewLLM != nil {
		reviewer = NewAsyncStateReviewer(stateReviewLLM)
	}

	var stateReviewRunner *AsyncRunner
	if reviewer != nil && conversationStore != nil {
		stateReviewRunner = NewAsyncRunner(AsyncRunnerConfig{
			Workers:   cfg.StateReviewWorkers,
			QueueSize: cfg.StateReviewQueueSize,
			Timeout:   time.Duration(cfg.StateReviewTimeoutMs) * time.Millisecond,
		}, logger)
	}

	return &Service{
		cfg:                   cfg,
		bot:                   bot,
		llm:                   llm,
		reminderLLM:           reminderLLM,
		store:                 conversationStore,
		prompt:                NewPromptBuilder(),
		fastState:             fastState,
		stateReviewer:         reviewer,
		stateReviewRunner:     stateReviewRunner,
		profileBatchReviewer:  profileBatchReviewer,
		profileReviewRunner:   profileReviewRunner,
		extractor:             extractor,
		memoryAnalyzer:        memoryAnalyzer,
		structuredRunner:      structuredRunner,
		logger:                logger,
		generationCoordinator: NewGenerationCoordinator(),
		chatLocks:             make(map[int64]*sync.Mutex),
	}
}

func (s *Service) getChatLock(chatID int64) *sync.Mutex {
	s.chatLocksMu.Lock()
	defer s.chatLocksMu.Unlock()
	if m, ok := s.chatLocks[chatID]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.chatLocks[chatID] = m
	return m
}

func (s *Service) SetHolidayResolver(resolver holiday.ContextResolver) {
	if s == nil {
		return
	}
	s.holidayResolver = resolver
}

func (s *Service) HandleUpdate(ctx context.Context, update telegram.Update) error {
	if update.Message == nil {
		return nil
	}

	if update.Message.Chat.Type != "private" {
		return nil
	}

	input := strings.TrimSpace(update.Message.Text)
	if input == "" {
		return nil
	}

	chatID := update.Message.Chat.ID
	if s.generationCoordinator != nil {
		s.generationCoordinator.CancelActive(chatID, interruptReasonNewInput)
	}

	handled, err := s.handleCommand(ctx, update, input)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	chatMu := s.getChatLock(chatID)
	chatMu.Lock()

	conversation := store.ConversationContext{}
	profilePrompt := profilePromptContext{}
	userTurnCount := 0
	now := promptNow()
	userMessage := model.Message{}
	sourceMessageID := update.UpdateID
	if s.store != nil {
		stored, err := s.store.BootstrapContext(ctx, *update.Message, DefaultSessionMode, s.cfg.RecentTurnLimit)
		if err != nil {
			s.logger.Warn("failed to bootstrap conversation context", "chat_id", update.Message.Chat.ID, "error", err)
		} else {
			conversation = stored
			record, err := s.store.SaveTurnRecord(
				ctx,
				conversation.Session.ID,
				"user",
				input,
				update.Message.MessageID,
				update.UpdateID,
				DefaultSessionMode,
				s.cfg.RecentTurnLimit,
			)
			if err != nil {
				s.logger.Warn("failed to persist user message", "session_id", conversation.Session.ID, "error", err)
			} else {
				userMessage = record
			}
			s.captureProactiveSignals(ctx, conversation.Session.ID, *update.Message, input)
			if userMessage.ID != 0 {
				sourceMessageID = userMessage.ID
			}
			s.captureUserSignals(ctx, conversation.User.ID, input, &conversation, sourceMessageID)

			userTurnCount = conversation.UserTurnCount + 1

			s.applyFastConversationState(ctx, &conversation, input, userTurnCount, sourceMessageID)
			s.enqueueAsyncStateReview(chatID, &conversation, input, userTurnCount, sourceMessageID)
			s.enqueueProfileBatchReview(&conversation, userTurnCount, sourceMessageID)

			if userTurnCount > 0 && detectProfilePromptDiscomfort(input) {
				pauseUntil := userTurnCount + profilePromptTurnCooldown
				if err := s.store.PauseUserProfileCollection(ctx, conversation.User.ID, pauseUntil); err != nil {
					s.logger.Warn("failed to pause profile collection", "user_id", conversation.User.ID, "until_turn", pauseUntil, "error", err)
				} else {
					conversation.UserProfile.CollectionPausedUntilTurn = pauseUntil
				}
			}
			profilePrompt = buildProfilePromptContext(
				conversation.UserProfile,
				len(conversation.RecentConversation),
				input,
				userTurnCount,
				messageTimestamp(*update.Message),
			)
			if s.cfg.MemorySlotEnabled && conversation.Session.ID != 0 {
				if slots, err := s.store.ListActiveTopicSlots(ctx, conversation.Session.ID, 10); err != nil {
					s.logger.Warn("failed to load topic slots", "session_id", conversation.Session.ID, "error", err)
				} else {
					conversation.TopicSlots = slots
				}
			}
		}
	}
	chatMu.Unlock()

	var holidaySelection *holiday.Selection
	holidayContext := ""
	topicsForPrompt := conversation.TopicSlots
	if s.memoryAnalyzer != nil {
		syncResult, err := s.memoryAnalyzer.SyncAnalyze(ctx, MemorySlotAnalyzeInput{
			Snapshot: model.MemorySlotSnapshot{
				SessionID:                conversation.Session.ID,
				RecentMessages:           conversation.RecentConversation,
				TopicSlots:               conversation.TopicSlots,
				ConversationStateMachine: conversation.ConversationStateMachine,
			},
			CurrentUserInput: input,
		})
		if err != nil {
			s.logger.Debug("memory slot sync analyze skipped", "session_id", conversation.Session.ID, "error", err)
		} else if syncResult != nil {
			topicsForPrompt, _ = MergeMemorySlotAnalysis(
				conversation.TopicSlots,
				model.ConversationStateSlot{},
				*syncResult,
				userMessage.ID,
				messageTimestamp(*update.Message),
			)
		}
	}
	regenCount := 0
	waitBeforeFirstGenerate := true
	promptGenerated := false

generateLoop:
	for {
		stateForAttempt := normalizeConversationStateMachine(conversation.ConversationStateMachine)
		handle, genCtx := s.generationCoordinator.Begin(ctx, chatID, conversation.Session.ID, sourceMessageID, stateForAttempt.Revision, regenCount)

		if waitBeforeFirstGenerate {
			waitBeforeFirstGenerate = false
			select {
			case <-ctx.Done():
				s.generationCoordinator.Finish(chatID, handle.GenerationID)
				return ctx.Err()
			case <-genCtx.Done():
				reason := s.generationCoordinator.InterruptReason(chatID, handle.GenerationID)
				s.generationCoordinator.Finish(chatID, handle.GenerationID)
				if reason == interruptReasonSafetyRestate && regenCount < 1 {
					s.reloadConversationStateMachine(ctx, &conversation)
					regenCount++
					continue generateLoop
				}
				return nil
			case <-time.After(1500 * time.Millisecond):
			}
		}

		if holidaySelection == nil && s.holidayResolver != nil && conversation.Session.ID != 0 {
			selected, err := s.holidayResolver.Resolve(ctx, holiday.ResolveInput{
				SessionID: conversation.Session.ID,
				Now:       now,
				GateSeed:  strconv.FormatInt(update.UpdateID, 10),
			})
			if err != nil {
				s.logger.Warn("failed to resolve holiday context", "session_id", conversation.Session.ID, "error", err)
			} else if selected != nil {
				holidaySelection = selected
				holidayContext = selected.PromptText
			}
		}

		if err := s.bot.SendChatAction(ctx, update.Message.Chat.ID, telegram.ChatActionTyping); err != nil {
			s.logger.Warn("failed to send typing action", "chat_id", update.Message.Chat.ID, "error", err)
		}

		memorySections := BuildMemoryPromptSectionsFromStateMachine(topicsForPrompt, stateForAttempt)
		reply, err := s.generateReply(genCtx, now, messageTimestamp(*update.Message), input, conversation.RecentConversation, conversation.UserProfile, conversation.ProfileCandidates, conversation.UserTraits, stateForAttempt, profilePrompt, holidayContext, userTurnCount, memorySections, conversation.Session.HistorySummary)
		promptGenerated = err == nil
		if err != nil {
			reason := s.generationCoordinator.InterruptReason(chatID, handle.GenerationID)
			s.generationCoordinator.Finish(chatID, handle.GenerationID)
			if errors.Is(err, context.Canceled) {
				if reason == interruptReasonSafetyRestate && regenCount < 1 {
					s.reloadConversationStateMachine(ctx, &conversation)
					regenCount++
					continue generateLoop
				}
				return nil
			}
			s.logger.Error("응답 생성에 실패했습니다.", "chat_id", update.Message.Chat.ID, "원인", logx.KoreanError(err))
			reply = FallbackText()
		}

		replyParts := SplitReplyForTelegram(reply)
		if len(replyParts) == 0 {
			replyParts = []string{reply}
		}

		for i, part := range replyParts {
			if i > 0 {
				delayMs := len([]rune(replyParts[i-1])) * 50
				if delayMs < 1000 {
					delayMs = 1000
				} else if delayMs > 3000 {
					delayMs = 3000
				}

				s.bot.SendChatAction(ctx, update.Message.Chat.ID, telegram.ChatActionTyping)

				select {
				case <-ctx.Done():
					s.generationCoordinator.Finish(chatID, handle.GenerationID)
					return ctx.Err()
				case <-genCtx.Done():
					s.generationCoordinator.Finish(chatID, handle.GenerationID)
					return nil
				case <-time.After(time.Duration(delayMs) * time.Millisecond):
				}
			}

			decision := s.generationCoordinator.BeforeSend(chatID, handle.GenerationID)
			if !decision.Allow {
				s.generationCoordinator.Finish(chatID, handle.GenerationID)
				if decision.Regenerate && regenCount < 1 {
					s.reloadConversationStateMachine(ctx, &conversation)
					regenCount++
					continue generateLoop
				}
				return nil
			}

			if err := s.bot.SendMessage(ctx, update.Message.Chat.ID, part); err != nil {
				s.generationCoordinator.Finish(chatID, handle.GenerationID)
				return err
			}
			s.generationCoordinator.MarkPartSent(chatID, handle.GenerationID)

			if s.store != nil && conversation.Session.ID != 0 {
				if err := s.store.SaveTurn(
					ctx,
					conversation.Session.ID,
					"assistant",
					part,
					0,
					update.UpdateID,
					DefaultSessionMode,
					s.cfg.RecentTurnLimit,
				); err != nil {
					// If session was reset/deleted during generation, ignore FK error
					if !strings.Contains(err.Error(), "violates foreign key constraint") {
						s.logger.Warn("failed to persist assistant message", "session_id", conversation.Session.ID, "error", err)
					}
				}
			}
		}

		s.generationCoordinator.Finish(chatID, handle.GenerationID)
		break
	}

	if s.store != nil && conversation.User.ID != 0 && profilePrompt.TargetSlot != "" && userTurnCount > 0 {
		if err := s.store.MarkUserProfileSlotPrompted(ctx, conversation.User.ID, profilePrompt.TargetSlot, userTurnCount); err != nil {
			s.logger.Warn("failed to mark profile slot prompted", "user_id", conversation.User.ID, "slot", profilePrompt.TargetSlot, "error", err)
		}
	}

	if promptGenerated && holidaySelection != nil && s.holidayResolver != nil && conversation.Session.ID != 0 {
		if err := s.holidayResolver.MarkUsed(ctx, conversation.Session.ID, holiday.SourceChat, *holidaySelection); err != nil {
			s.logger.Warn("failed to mark holiday topic used", "session_id", conversation.Session.ID, "topic_key", holidaySelection.TopicKey, "error", err)
		}
	}

	triggerInterval := s.cfg.SummaryTriggerMessages
	if triggerInterval <= 0 {
		triggerInterval = 10
	}

	if s.memoryAnalyzer != nil && conversation.Session.ID != 0 && userMessage.ID != 0 {
		if userTurnCount > 0 && userTurnCount%triggerInterval == 0 {
			s.memoryAnalyzer.AsyncEnqueue(conversation.Session.ID, userMessage.ID, input)
		}
	}

	// Trigger history summarization every 10 turns, starting from turn 30
	if userTurnCount >= s.cfg.RecentTurnLimit && userTurnCount%10 == 0 {
		s.updateHistorySummaryAsync(conversation.Session.ID, s.cfg.RecentTurnLimit)
	}

	return nil
}

func (s *Service) handleCommand(ctx context.Context, update telegram.Update, input string) (bool, error) {
	if !strings.HasPrefix(input, "/") && !strings.HasPrefix(input, "!") {
		return false, nil
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false, nil
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/start":
		return true, s.bot.SendMessage(ctx, update.Message.Chat.ID, WelcomeText())
	case "/ping":
		return true, s.bot.SendMessage(ctx, update.Message.Chat.ID, "pong")
	case "/proactive_on", "/선톡켜", "!proactive_on", "!선톡켜":
		return true, s.handleProactiveOptCommand(ctx, update, true)
	case "/proactive_off", "/선톡꺼", "!proactive_off", "!선톡꺼":
		return true, s.handleProactiveOptCommand(ctx, update, false)
	case "/reset", "/리셋", "!reset", "!리셋":
		return true, s.handleResetCommand(ctx, update)
	case "!내정보", "!info", "/내정보", "/info":
		return true, s.handleMyInfoCommand(ctx, update)
	case "!대화내용", "!context", "/대화내용", "/context":
		return true, s.handleConversationContentCommand(ctx, update)
	case "!cleartraits", "/cleartraits":
		return true, s.handleClearTraitsCommand(ctx, update)
	case "!도움말", "/help", "!help", "/도움말":
		return true, s.bot.SendMessage(ctx, update.Message.Chat.ID, helpText())
	default:
		// If it's a known typo like the one the user made or clearly intended as a command,
		// we just absorb it without saving to DB to avoid confusing the LLM.
		if strings.HasPrefix(cmd, "/") || strings.HasPrefix(cmd, "!") {
			// Ensure it's not just a bunch of exclamation marks like "!!!"
			if len(cmd) > 1 && !strings.HasSuffix(cmd, "!") {
				s.bot.SendMessage(ctx, update.Message.Chat.ID, "알 수 없는 명령어에요. /help 를 입력해 보세요.")
				return true, nil
			}
		}
		return false, nil
	}
}

func (s *Service) handleResetCommand(ctx context.Context, update telegram.Update) error {
	if s.generationCoordinator != nil && update.Message != nil {
		s.generationCoordinator.CancelActive(update.Message.Chat.ID, interruptReasonReset)
	}

	if s.store == nil || update.Message == nil || update.Message.From == nil {
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, ResetText())
	}

	if _, err := s.store.ResetConversation(ctx, update.Message.From.ID); err != nil {
		s.logger.Error("failed to reset conversation", "chat_id", update.Message.Chat.ID, "telegram_user_id", update.Message.From.ID, "error", err)
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "리셋하다가 잠깐 꼬였어. 한 번만 다시 쳐줘.")
	}

	// 2. Send fixed opening message and save to history
	// We bootstrap a clean context after reset to get new session/user info.
	conversation, err := s.store.BootstrapContext(ctx, *update.Message, DefaultSessionMode, s.cfg.RecentTurnLimit)
	if err != nil {
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "좋아, 다 잊었어! 우리 이제 새로 시작하자. 먼저 인사해 줄래?")
	}

	fullResponse := "좋아, 깔끔하게 다 잊었어! 우리 새로 시작하는 거다?\n안녕. 이름이 뭐야?"
	if err := s.bot.SendMessage(ctx, update.Message.Chat.ID, fullResponse); err != nil {
		return err
	}

	// 3. Save the opener to history so that user's next reply has context.
	if s.store != nil {
		_ = s.store.SaveTurn(ctx, conversation.Session.ID, "assistant", fullResponse, 0, update.UpdateID, DefaultSessionMode, s.cfg.RecentTurnLimit)
		// Mark name slot as prompted
		_ = s.store.MarkUserProfileSlotPrompted(ctx, conversation.User.ID, profileSlotName, 1)
	}

	return nil
}

func (s *Service) handleProactiveOptCommand(ctx context.Context, update telegram.Update, enabled bool) error {
	if update.Message == nil {
		return nil
	}
	if s.store == nil {
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "선톡 설정은 지금 잠깐 안 되고 있어요.")
	}

	conversation, err := s.store.BootstrapContext(ctx, *update.Message, DefaultSessionMode, s.cfg.RecentTurnLimit)
	if err != nil {
		s.logger.Warn("failed to bootstrap context for proactive opt command", "chat_id", update.Message.Chat.ID, "error", err)
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "선톡 설정하다가 잠깐 꼬였어. 한 번만 다시 쳐줘.")
	}

	if err := s.store.SetSessionProactiveOptIn(ctx, conversation.Session.ID, enabled); err != nil {
		s.logger.Warn("failed to update proactive opt-in", "session_id", conversation.Session.ID, "enabled", enabled, "error", err)
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "선톡 설정하다가 잠깐 꼬였어. 한 번만 다시 쳐줘.")
	}

	if enabled {
		bestWindows := []map[string]any{}
		for _, window := range defaultBestTimeWindows(messageTimestamp(*update.Message)) {
			bestWindows = append(bestWindows, map[string]any{
				"weekdays": window["weekdays"],
				"start":    window["start"],
				"end":      window["end"],
				"score":    window["score"],
			})
		}
		if _, err := s.store.UpsertProactiveProfile(ctx, pgstore.UpsertProactiveProfileParams{
			UserID:                conversation.User.ID,
			PreferredTypesJSON:    mustJSON([]string{"event_followup", "reconnect", "habit_ping"}),
			DislikedTypesJSON:     mustJSON([]string{}),
			BestTimeWindowsJSON:   mustJSON(bestWindows),
			MaxPerDay:             1,
			MaxPerWeek:            4,
			MinGapHours:           20,
			AvgReplyDelaySec:      0,
			ProactiveSuccessScore: 0,
			TimezoneName:          "Asia/Seoul",
			WeightOverridesJSON:   mustJSON(map[string]float64{}),
		}); err != nil {
			s.logger.Warn("failed to ensure proactive profile", "user_id", conversation.User.ID, "error", err)
		}
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "선톡 켰어. 너무 들이대진 않고, 타이밍 맞을 때만 먼저 톡할게.")
	}

	return s.bot.SendMessage(ctx, update.Message.Chat.ID, "선톡 껐어. 이제 네가 먼저 말 걸 때만 답할게.")
}

func (s *Service) handleMyInfoCommand(ctx context.Context, update telegram.Update) error {
	if s.store == nil || update.Message == nil {
		return nil
	}

	conversation, err := s.store.BootstrapContext(ctx, *update.Message, DefaultSessionMode, s.cfg.RecentTurnLimit)
	if err != nil {
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "정보를 불러오는 데 실패했어.")
	}

	var sb strings.Builder
	sb.WriteString("👤 *[데이터상 내 정보]*\n")
	p := conversation.UserProfile
	candidatesBySlot := topProfileCandidateBySlot(conversation.ProfileCandidates)
	writeField := func(slot string) {
		sb.WriteString(fmt.Sprintf("• %s: %s\n", profileSlotLabel(slot), renderProfileFieldWithCandidate(p, candidatesBySlot, slot)))
	}

	writeField(profileSlotName)
	writeField(profileSlotGender)
	writeField(profileSlotAge)
	writeField(profileSlotJob)
	writeField(profileSlotLocation)
	writeField(profileSlotAffiliation)
	writeField(profileSlotHobby)
	writeField(profileSlotCurrentFocus)

	if len(conversation.UserTraits) > 0 {
		sb.WriteString("\n📊 *[파악된 특징]*\n")
		for _, t := range conversation.UserTraits {
			sb.WriteString(fmt.Sprintf("• %s: %s\n", t.TraitType, t.DisplayValue))
		}
	}

	return s.bot.SendMessage(ctx, update.Message.Chat.ID, sb.String())
}

func (s *Service) handleConversationContentCommand(ctx context.Context, update telegram.Update) error {
	if s.store == nil || update.Message == nil {
		return nil
	}

	conversation, err := s.store.BootstrapContext(ctx, *update.Message, DefaultSessionMode, s.cfg.RecentTurnLimit)
	if err != nil {
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "대화 내용을 불러오는 데 실패했어.")
	}

	// Fetch additional memory slots not in BootstrapContext
	topics, _ := s.store.ListActiveTopicSlots(ctx, conversation.Session.ID, 10)
	state, _ := s.store.GetConversationStateMachine(ctx, conversation.Session.ID)

	var sb strings.Builder
	sb.WriteString("💬 *[현재 대화 맥락]*\n")
	if state.TonePhase != "" || state.RelationalStage != "" {
		sb.WriteString(fmt.Sprintf("• 톤 단계: %s\n", state.TonePhase))
		sb.WriteString(fmt.Sprintf("• 관계 단계: %s\n", state.RelationalStage))
		sb.WriteString(fmt.Sprintf("• 방향: %s\n", state.StageDirection))
		sb.WriteString(fmt.Sprintf("• 분위기: %s\n", state.EmotionalTone))
		sb.WriteString(fmt.Sprintf("• 모드: %s\n", state.InteractionMode))
		sb.WriteString(fmt.Sprintf("• 집중 주제: %s\n", state.FocusTopicKey))
		if state.SafetyLockUntilTurn > 0 {
			sb.WriteString(fmt.Sprintf("• safety lock: %d\n", state.SafetyLockUntilTurn))
		}
		if state.OpenLoopSummary != "" {
			sb.WriteString(fmt.Sprintf("• 미완결 루프: %s\n", state.OpenLoopSummary))
		}
		if conversation.Session.HistorySummary != "" {
			sb.WriteString(fmt.Sprintf("\n📜 *[과거 대화 요약 (30턴 이전)]*\n%s\n", conversation.Session.HistorySummary))
		}
	} else {
		sb.WriteString("(분석된 맥락 없음)\n")
	}

	if len(topics) > 0 {
		sb.WriteString("\n📌 *[저장된 대화 주제들]*\n")
		for i, t := range topics {
			importance := "⭐️"
			if t.Importance >= 7 {
				importance = "🔥"
			} else if t.Importance <= 3 {
				importance = "🍃"
			}
			sb.WriteString(fmt.Sprintf("%d. %s [%s]\n   └ %s\n", i+1, t.TopicLabel, importance, t.Summary))
		}
	}

	return s.bot.SendMessage(ctx, update.Message.Chat.ID, sb.String())
}

func (s *Service) handleClearTraitsCommand(ctx context.Context, update telegram.Update) error {
	if s.store == nil || update.Message == nil {
		return nil
	}

	conversation, err := s.store.BootstrapContext(ctx, *update.Message, DefaultSessionMode, s.cfg.RecentTurnLimit)
	if err != nil {
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "유저 정보를 불러올 수 없어.")
	}

	// Because we don't have a direct DeleteUserTraits method, we can run a raw query here just for debugging.
	// But actually the store manager doesn't easily expose the pool.
	// We can update the user traits to 'delete' via Upsert !
	for _, trait := range conversation.UserTraits {
		_, _ = s.store.UpsertUserTrait(ctx, pgstore.UpsertUserTraitParams{
			UserID:          conversation.User.ID,
			TraitType:       "delete",
			NormalizedValue: trait.NormalizedValue,
			DisplayValue:    trait.DisplayValue,
			SourceText:      "Admin cleared",
		})
	}

	return s.bot.SendMessage(ctx, update.Message.Chat.ID, "✅ 과거에 잘못 저장된 모든 취향 정보(Traits)를 리셋(Delete 처리) 완료했습니다.")
}

func helpText() string {
	var sb strings.Builder
	sb.WriteString("🤖 *[사용 가능한 명령어]*\n\n")

	sb.WriteString("📍 *설정 및 관리*\n")
	sb.WriteString("• `/start`: 대화 시작 및 환영 인사\n")
	sb.WriteString("• `/리셋` 또는 `/reset`: 지금까지의 대화 내용 초기화\n")
	sb.WriteString("• `/선톡켜` | `/선톡꺼`: 선톡(Proactive Message) 활성화/비활성화\n\n")

	sb.WriteString("🔍 *데이터 확인 (개발용)*\n")
	sb.WriteString("• `!내정보`: AI가 파악한 내 프로필 및 특징 확인\n")
	sb.WriteString("• `!대화내용`: 현재 대화의 맥락 분석 결과 및 저장된 주제 목록 확인\n")
	sb.WriteString("• `!도움말`: 명령어 목록 보기\n\n")

	sb.WriteString("기타 궁금한 점은 그냥 편하게 대화로 물어봐줘!")
	return sb.String()
}

func mustJSON(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return payload
}

func defaultBestTimeWindows(now time.Time) []map[string]any {
	_ = now
	return []map[string]any{
		{
			"weekdays": []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
			"start":    "08:00",
			"end":      "10:00",
			"score":    0.58,
		},
		{
			"weekdays": []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
			"start":    "12:00",
			"end":      "14:00",
			"score":    0.56,
		},
		{
			"weekdays": []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
			"start":    "19:00",
			"end":      "21:30",
			"score":    0.64,
		},
	}
}

func formatClock(hour int, minute int) string {
	return strings.TrimSpace(
		strings.Join([]string{
			leftPadTwo(hour),
			leftPadTwo(minute),
		}, ":"),
	)
}

func leftPadTwo(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func (s *Service) generateReply(ctx context.Context, now time.Time, currentInputAt time.Time, input string, recentConversation []model.Message, userProfile model.UserProfile, profileCandidates []model.ProfileCandidate, userTraits []model.UserTrait, stateMachine model.ConversationStateMachine, profilePrompt profilePromptContext, holidayContext string, userTurnCount int, memorySections MemoryPromptSections, historySummary string) (string, error) {
	messages := s.prompt.Build(PromptInput{
		UserInput:                input,
		RecentConversation:       recentConversation,
		UserProfile:              userProfile,
		CandidateProfileSummary:  candidateProfileSummary(userProfile, profileCandidates),
		UserTraits:               userTraits,
		ProfilePrompt:            profilePrompt,
		HolidayContextText:       holidayContext,
		UserTurnCount:            userTurnCount,
		ConversationStateMachine: stateMachine,
		CurrentTimeText:          promptutil.FormatPromptCurrentTime(now),
		CurrentUserInputTime:     currentInputAt,
		HistorySummary:           historySummary,
		ActiveTopicsText:         memorySections.ActiveTopicsText,
		ConversationStateText:    memorySections.ConversationStateText,
		OpenLoopsText:            memorySections.OpenLoopsText,
		MemorySummary:            memorySections.MemorySummary,
	})

	reply, err := s.llm.Chat(ctx, messages)
	if err != nil {
		return "", err
	}

	reply = PostProcess(reply, s.cfg.ResponseMaxChars)
	if len(recentConversation) == 0 || userTurnCount <= 2 {
		reply = sanitizeFreshConversationReply(reply)
	}
	reply = sanitizeByConversationPhase(reply, stateMachine.TonePhase)
	if reply == "" {
		reply = FallbackText()
	}

	return reply, nil
}

func promptNow() time.Time {
	return time.Now().In(promptutil.PromptTimeLocation())
}

func (s *Service) applyFastConversationState(ctx context.Context, conversation *store.ConversationContext, input string, userTurnCount int, sourceMessageID int64) {
	if s.store == nil || conversation == nil || conversation.Session.ID == 0 {
		return
	}

	current := normalizeConversationStateMachine(conversation.ConversationStateMachine)
	result := FastStateResult{}
	if s.fastState != nil {
		result = s.fastState.Evaluate(input, current.TonePhase)
	}

	next := current
	changed := false
	if current.SafetyLockUntilTurn > 0 && userTurnCount < current.SafetyLockUntilTurn && current.TonePhase != phaseNeutral {
		next.TonePhase = phaseNeutral
		changed = true
	}

	if signal := result.Signal; signal.NextPhase != "" && signal.NextPhase != next.TonePhase {
		next.TonePhase = signal.NextPhase
		changed = true
	}
	if signal := result.Signal; signal.PauseSexualTurns > 0 {
		untilTurn := userTurnCount + signal.PauseSexualTurns
		if untilTurn > next.SafetyLockUntilTurn {
			next.SafetyLockUntilTurn = untilTurn
			changed = true
		}
		if next.TonePhase != phaseNeutral {
			next.TonePhase = phaseNeutral
			changed = true
		}
	}

	if !changed && current.SessionID != 0 {
		conversation.ConversationStateMachine = current
		return
	}

	next.Revision = current.Revision + 1
	next.LastSourceMessageID = sourceMessageID
	next.LastDecisionSource = "rule"
	next.Confidence = model.ConfidenceHigh
	next.EvidenceJSON = mustJSON(map[string]string{
		"matched_rule":     result.Signal.MatchedRule,
		"matched_language": result.Signal.MatchedLanguage,
		"matched_phrase":   result.Signal.MatchedPhrase,
	})

	updated, err := s.store.UpsertConversationStateMachine(ctx, conversation.Session.ID, next)
	if err != nil {
		s.logger.Warn("failed to upsert conversation state machine", "session_id", conversation.Session.ID, "error", err)
		return
	}
	conversation.ConversationStateMachine = normalizeConversationStateMachine(updated)
}

func (s *Service) enqueueAsyncStateReview(chatID int64, conversation *store.ConversationContext, input string, userTurnCount int, sourceMessageID int64) {
	if s == nil || s.store == nil || s.stateReviewer == nil || s.stateReviewRunner == nil || conversation == nil || conversation.Session.ID == 0 || sourceMessageID == 0 {
		return
	}

	sessionID := conversation.Session.ID
	s.stateReviewRunner.Enqueue(AsyncTask{
		Key:     stateReviewTaskKey(sessionID),
		Version: sourceMessageID,
		Build: func(ctx context.Context) (func(context.Context) error, error) {
			snapshot, err := s.store.BuildStateReviewSnapshot(ctx, sessionID, s.cfg.RecentTurnLimit)
			if err != nil {
				return nil, err
			}

			review, err := s.stateReviewer.Review(ctx, buildAsyncStateReviewInput(snapshot, input))
			if err != nil {
				return nil, err
			}

			return func(commitCtx context.Context) error {
				current, err := s.store.GetConversationStateMachine(commitCtx, sessionID)
				if err != nil {
					return err
				}
				current = normalizeConversationStateMachine(current)
				if current.LastSourceMessageID > 0 && sourceMessageID < current.LastSourceMessageID {
					return nil
				}

				next, changed, triggerSafety := applyAsyncStateReviewResult(current, snapshot.TopicSlots, review, userTurnCount, sourceMessageID)
				if !changed {
					return nil
				}

				updated, err := s.store.UpsertConversationStateMachine(commitCtx, sessionID, next)
				if err != nil {
					return err
				}

				if triggerSafety && s.generationCoordinator != nil {
					s.generationCoordinator.RequestSafetyRestate(chatID, sessionID, updated.Revision)
				}
				return nil
			}, nil
		},
		OnDrop: func(reason string, queueDepth int) {
			if s.logger != nil {
				s.logger.Warn("state review queue full", "reason", reason, "queue_depth", queueDepth, "session_id", sessionID, "source_message_id", sourceMessageID)
			}
		},
		OnSuperseded: func(stage string) {
			if s.logger != nil {
				s.logger.Debug("state review superseded", "stage", stage, "session_id", sessionID, "source_message_id", sourceMessageID)
			}
		},
	})
}

func (s *Service) reloadConversationStateMachine(ctx context.Context, conversation *store.ConversationContext) {
	if s == nil || s.store == nil || conversation == nil || conversation.Session.ID == 0 {
		return
	}

	latest, err := s.store.GetConversationStateMachine(ctx, conversation.Session.ID)
	if err != nil {
		s.logger.Warn("failed to reload conversation state machine", "session_id", conversation.Session.ID, "error", err)
		return
	}
	conversation.ConversationStateMachine = normalizeConversationStateMachine(latest)
}

func normalizeConversationStateMachine(state model.ConversationStateMachine) model.ConversationStateMachine {
	if strings.TrimSpace(state.TonePhase) == "" {
		state.TonePhase = phaseNeutral
	}
	if strings.TrimSpace(state.RelationalStage) == "" {
		state.RelationalStage = "opener"
	}
	if strings.TrimSpace(state.StageDirection) == "" {
		state.StageDirection = "stable"
	}
	if strings.TrimSpace(state.Confidence) == "" {
		state.Confidence = model.ConfidenceLow
	}
	if strings.TrimSpace(state.LastDecisionSource) == "" {
		state.LastDecisionSource = "rule"
	}
	if len(state.EvidenceJSON) == 0 {
		state.EvidenceJSON = []byte("[]")
	}
	return state
}

func buildAsyncStateReviewInput(snapshot model.StateReviewSnapshot, input string) AsyncStateReviewInput {
	state := normalizeConversationStateMachine(snapshot.StateMachine)
	topics := make([]string, 0, len(snapshot.TopicSlots))
	for _, slot := range filterPromptTopicSlots(snapshot.TopicSlots) {
		label := buildTopicPromptLine(slot)
		if label == "" {
			continue
		}
		topics = append(topics, label)
	}

	return AsyncStateReviewInput{
		CurrentUserInput:       input,
		CurrentTonePhase:       state.TonePhase,
		CurrentStage:           state.RelationalStage,
		CurrentDirection:       state.StageDirection,
		CurrentEmotionalTone:   state.EmotionalTone,
		CurrentInteractionMode: state.InteractionMode,
		CurrentOpenLoop:        state.OpenLoopSummary,
		CurrentFocusTopicKey:   state.FocusTopicKey,
		ActiveTopics:           topics,
		RecentConversation:     snapshot.RecentMessages,
	}
}

func applyAsyncStateReviewResult(current model.ConversationStateMachine, topicSlots []model.TopicSlot, review AsyncStateReviewResult, userTurnCount int, sourceMessageID int64) (model.ConversationStateMachine, bool, bool) {
	next := normalizeConversationStateMachine(current)
	changed := false
	triggerSafety := false
	confident := review.Confidence == model.ConfidenceMedium || review.Confidence == model.ConfidenceHigh

	if confident && review.SafetyAction == "deescalate" {
		if next.TonePhase != phaseNeutral {
			next.TonePhase = phaseNeutral
			changed = true
			triggerSafety = true
		}
		if review.SafetyLockTurns > 0 {
			untilTurn := userTurnCount + review.SafetyLockTurns
			if untilTurn > next.SafetyLockUntilTurn {
				next.SafetyLockUntilTurn = untilTurn
				changed = true
				triggerSafety = true
			}
		}
	}

	if confident && review.TonePhase != "" && review.TonePhase != "unchanged" && review.TonePhase != next.TonePhase {
		next.TonePhase = review.TonePhase
		changed = true
	}
	if confident && review.RelationalStage != "" && review.RelationalStage != "unchanged" && review.RelationalStage != next.RelationalStage {
		next.RelationalStage = review.RelationalStage
		changed = true
	}
	if confident && review.StageDirection != "" && review.StageDirection != "unchanged" && review.StageDirection != next.StageDirection {
		next.StageDirection = review.StageDirection
		changed = true
	}
	if confident && strings.TrimSpace(review.EmotionalTone) != "" && review.EmotionalTone != next.EmotionalTone {
		next.EmotionalTone = review.EmotionalTone
		changed = true
	}
	if confident && strings.TrimSpace(review.InteractionMode) != "" && review.InteractionMode != next.InteractionMode {
		next.InteractionMode = review.InteractionMode
		changed = true
	}
	if confident && strings.TrimSpace(review.OpenLoopSummary) != "" && review.OpenLoopSummary != next.OpenLoopSummary {
		next.OpenLoopSummary = review.OpenLoopSummary
		changed = true
	}
	if confident && strings.TrimSpace(review.FocusTopicLabel) != "" {
		focusKey := topicKeyFromLabel(review.FocusTopicLabel)
		if focusKey != "" && focusKey != next.FocusTopicKey {
			next.FocusTopicKey = focusKey
			changed = true
		}
	}

	if !changed {
		return next, false, false
	}

	next.Revision = current.Revision + 1
	next.LastSourceMessageID = sourceMessageID
	next.LastDecisionSource = "async_state_review"
	next.Confidence = review.Confidence
	next.EvidenceJSON = evidenceJSON(review.EvidenceTexts)
	return next, true, triggerSafety
}

func stateReviewTaskKey(sessionID int64) string {
	return "state_machine:" + strconv.FormatInt(sessionID, 10)
}
