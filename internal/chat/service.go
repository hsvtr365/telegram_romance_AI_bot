package chat

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/holiday"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/telegram"
)

type Messenger interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	SendChatAction(ctx context.Context, chatID int64, action string) error
}

type LLM interface {
	Chat(ctx context.Context, messages []ollama.Message) (string, error)
}

type Config struct {
	RecentTurnLimit           int
	ResponseMaxChars          int
	StructuredExtractEnabled  bool
	StructuredExtractMinChars int
}

type Service struct {
	cfg             Config
	bot             Messenger
	llm             LLM
	reminderLLM     LLM
	store           *store.Manager
	prompt          *PromptBuilder
	extractor       *StructuredExtractor
	holidayResolver holiday.ContextResolver
	logger          *slog.Logger
}

func NewService(cfg Config, bot Messenger, llm LLM, reminderLLM LLM, structuredLLM LLM, conversationStore *store.Manager, logger *slog.Logger) *Service {
	if cfg.RecentTurnLimit <= 0 {
		cfg.RecentTurnLimit = 14
	}
	if cfg.StructuredExtractMinChars <= 0 {
		cfg.StructuredExtractMinChars = 12
	}

	var extractor *StructuredExtractor
	if cfg.StructuredExtractEnabled && structuredLLM != nil {
		extractor = NewStructuredExtractor(structuredLLM, cfg.StructuredExtractMinChars)
	}

	return &Service{
		cfg:         cfg,
		bot:         bot,
		llm:         llm,
		reminderLLM: reminderLLM,
		store:       conversationStore,
		prompt:      NewPromptBuilder(),
		extractor:   extractor,
		logger:      logger,
	}
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

	handled, err := s.handleCommand(ctx, update, input)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	conversation := store.ConversationContext{}
	profilePrompt := profilePromptContext{}
	userTurnCount := 0
	now := promptNow()
	if s.store != nil {
		stored, err := s.store.BootstrapContext(ctx, *update.Message, DefaultSessionMode, s.cfg.RecentTurnLimit)
		if err != nil {
			s.logger.Warn("failed to bootstrap conversation context", "chat_id", update.Message.Chat.ID, "error", err)
		} else {
			conversation = stored
			if err := s.store.SaveTurn(
				ctx,
				conversation.Session.ID,
				"user",
				input,
				update.Message.MessageID,
				update.UpdateID,
				DefaultSessionMode,
				s.cfg.RecentTurnLimit,
			); err != nil {
				s.logger.Warn("failed to persist user message", "session_id", conversation.Session.ID, "error", err)
			}
			s.captureProactiveSignals(ctx, conversation.Session.ID, *update.Message, input)
			s.captureUserSignals(ctx, conversation.User.ID, input, &conversation)
			if count, err := s.store.CountUserTurns(ctx, conversation.Session.ID); err != nil {
				s.logger.Warn("failed to count user turns", "session_id", conversation.Session.ID, "error", err)
			} else {
				userTurnCount = count
			}
			s.applyConversationPhase(ctx, &conversation, input, userTurnCount)
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
		}
	}

	var holidaySelection *holiday.Selection
	if s.holidayResolver != nil && conversation.Session.ID != 0 {
		selected, err := s.holidayResolver.Resolve(ctx, holiday.ResolveInput{
			SessionID: conversation.Session.ID,
			Now:       now,
			GateSeed:  strconv.FormatInt(update.UpdateID, 10),
		})
		if err != nil {
			s.logger.Warn("failed to resolve holiday context", "session_id", conversation.Session.ID, "error", err)
		} else {
			holidaySelection = selected
		}
	}

	if err := s.bot.SendChatAction(ctx, update.Message.Chat.ID, telegram.ChatActionTyping); err != nil {
		s.logger.Warn("failed to send typing action", "chat_id", update.Message.Chat.ID, "error", err)
	}

	holidayContext := ""
	if holidaySelection != nil {
		holidayContext = holidaySelection.PromptText
	}

	reply, err := s.generateReply(ctx, now, input, conversation.RecentConversation, conversation.UserProfile, conversation.UserTraits, conversation.Session.ConversationPhase, profilePrompt, holidayContext, userTurnCount)
	promptGenerated := err == nil
	if err != nil {
		s.logger.Error("failed to generate reply", "chat_id", update.Message.Chat.ID, "error", err)
		reply = FallbackText()
	}

	replyParts := SplitReplyForTelegram(reply)
	if len(replyParts) == 0 {
		replyParts = []string{reply}
	}

	for _, part := range replyParts {
		if err := s.bot.SendMessage(ctx, update.Message.Chat.ID, part); err != nil {
			return err
		}

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
				s.logger.Warn("failed to persist assistant message", "session_id", conversation.Session.ID, "error", err)
			}
		}
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

	return nil
}

func (s *Service) handleCommand(ctx context.Context, update telegram.Update, input string) (bool, error) {
	switch input {
	case "/start":
		return true, s.bot.SendMessage(ctx, update.Message.Chat.ID, WelcomeText())
	case "/ping":
		return true, s.bot.SendMessage(ctx, update.Message.Chat.ID, "pong")
	case "/proactive_on", "/선톡켜":
		return true, s.handleProactiveOptCommand(ctx, update, true)
	case "/proactive_off", "/선톡꺼":
		return true, s.handleProactiveOptCommand(ctx, update, false)
	case "/reset", "/리셋":
		return true, s.handleResetCommand(ctx, update)
	default:
		return false, nil
	}
}

func (s *Service) handleResetCommand(ctx context.Context, update telegram.Update) error {
	if s.store == nil || update.Message == nil || update.Message.From == nil {
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, ResetText())
	}

	if _, err := s.store.ResetConversation(ctx, update.Message.From.ID); err != nil {
		s.logger.Error("failed to reset conversation", "chat_id", update.Message.Chat.ID, "telegram_user_id", update.Message.From.ID, "error", err)
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "리셋하다가 잠깐 꼬였어. 한 번만 다시 쳐줘.")
	}

	return s.bot.SendMessage(ctx, update.Message.Chat.ID, ResetText())
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

func (s *Service) generateReply(ctx context.Context, now time.Time, input string, recentConversation []ollama.Message, userProfile model.UserProfile, userTraits []model.UserTrait, conversationPhase string, profilePrompt profilePromptContext, holidayContext string, userTurnCount int) (string, error) {
	messages := s.prompt.Build(PromptInput{
		UserInput:          input,
		RecentConversation: recentConversation,
		UserProfile:        userProfile,
		UserTraits:         userTraits,
		ProfilePrompt:      profilePrompt,
		HolidayContextText: holidayContext,
		UserTurnCount:      userTurnCount,
		ConversationPhase:  conversationPhase,
		CurrentTimeText:    formatPromptCurrentTime(now),
	})

	reply, err := s.llm.Chat(ctx, messages)
	if err != nil {
		return "", err
	}

	reply = PostProcess(reply, s.cfg.ResponseMaxChars)
	if len(recentConversation) == 0 || userTurnCount <= 2 {
		reply = sanitizeFreshConversationReply(reply)
	}
	reply = sanitizeByConversationPhase(reply, conversationPhase)
	if reply == "" {
		reply = FallbackText()
	}

	return reply, nil
}

func formatPromptCurrentTime(now time.Time) string {
	return "지금 시각은 " + now.Format("2006-01-02 15:04 MST") + " 이다."
}

func promptNow() time.Time {
	return time.Now().In(time.FixedZone("KST", 9*60*60))
}

func (s *Service) applyConversationPhase(ctx context.Context, conversation *store.ConversationContext, input string, userTurnCount int) {
	if s.store == nil || conversation == nil || conversation.Session.ID == 0 {
		return
	}

	currentPhase := conversation.Session.ConversationPhase
	if currentPhase == "" {
		currentPhase = phaseNeutral
	}

	if conversation.Session.SexualPauseUntilTurn > 0 && userTurnCount < conversation.Session.SexualPauseUntilTurn {
		if currentPhase != phaseNeutral {
			if err := s.store.SetSessionConversationPhase(ctx, conversation.Session.ID, phaseNeutral); err != nil {
				s.logger.Warn("failed to force neutral phase during pause", "session_id", conversation.Session.ID, "error", err)
			} else {
				conversation.Session.ConversationPhase = phaseNeutral
			}
		}
		return
	}

	signal := detectConversationPhaseSignal(input, currentPhase)
	if signal.NextPhase != "" && signal.NextPhase != currentPhase {
		if err := s.store.SetSessionConversationPhase(ctx, conversation.Session.ID, signal.NextPhase); err != nil {
			s.logger.Warn("failed to update conversation phase", "session_id", conversation.Session.ID, "phase", signal.NextPhase, "error", err)
		} else {
			conversation.Session.ConversationPhase = signal.NextPhase
		}
	}

	if signal.PauseSexualTurns > 0 {
		untilTurn := userTurnCount + signal.PauseSexualTurns
		if err := s.store.SetSessionSexualPauseUntilTurn(ctx, conversation.Session.ID, untilTurn); err != nil {
			s.logger.Warn("failed to update sexual pause", "session_id", conversation.Session.ID, "until_turn", untilTurn, "error", err)
		} else {
			conversation.Session.SexualPauseUntilTurn = untilTurn
		}
		if conversation.Session.ConversationPhase != phaseNeutral {
			conversation.Session.ConversationPhase = phaseNeutral
		}
	}
}
