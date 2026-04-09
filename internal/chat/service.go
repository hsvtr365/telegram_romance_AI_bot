package chat

import (
	"context"
	"log/slog"
	"strings"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
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
	DefaultMode      Mode
	RecentTurnLimit  int
	ResponseMaxChars int
}

type Service struct {
	cfg    Config
	bot    Messenger
	llm    LLM
	store  *store.Manager
	prompt *PromptBuilder
	logger *slog.Logger
}

func NewService(cfg Config, bot Messenger, llm LLM, conversationStore *store.Manager, logger *slog.Logger) *Service {
	if cfg.DefaultMode == "" {
		cfg.DefaultMode = defaultMode
	}
	if cfg.RecentTurnLimit <= 0 {
		cfg.RecentTurnLimit = 14
	}

	return &Service{
		cfg:    cfg,
		bot:    bot,
		llm:    llm,
		store:  conversationStore,
		prompt: NewPromptBuilder(),
		logger: logger,
	}
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
	if s.store != nil {
		stored, err := s.store.BootstrapContext(ctx, *update.Message, string(s.cfg.DefaultMode), s.cfg.RecentTurnLimit)
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
				string(s.cfg.DefaultMode),
				s.cfg.RecentTurnLimit,
			); err != nil {
				s.logger.Warn("failed to persist user message", "session_id", conversation.Session.ID, "error", err)
			}
		}
	}

	if err := s.bot.SendChatAction(ctx, update.Message.Chat.ID, telegram.ChatActionTyping); err != nil {
		s.logger.Warn("failed to send typing action", "chat_id", update.Message.Chat.ID, "error", err)
	}

	reply, err := s.generateReply(ctx, input, conversation.RecentConversation)
	if err != nil {
		s.logger.Error("failed to generate reply", "chat_id", update.Message.Chat.ID, "error", err)
		reply = s.cfg.DefaultMode.FallbackText()
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
				string(s.cfg.DefaultMode),
				s.cfg.RecentTurnLimit,
			); err != nil {
				s.logger.Warn("failed to persist assistant message", "session_id", conversation.Session.ID, "error", err)
			}
		}
	}

	return nil
}

func (s *Service) handleCommand(ctx context.Context, update telegram.Update, input string) (bool, error) {
	switch input {
	case "/start":
		return true, s.bot.SendMessage(ctx, update.Message.Chat.ID, s.cfg.DefaultMode.WelcomeText())
	case "/ping":
		return true, s.bot.SendMessage(ctx, update.Message.Chat.ID, "pong")
	case "/reset", "/리셋":
		return true, s.handleResetCommand(ctx, update)
	default:
		return false, nil
	}
}

func (s *Service) handleResetCommand(ctx context.Context, update telegram.Update) error {
	if s.store == nil || update.Message == nil || update.Message.From == nil {
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, s.cfg.DefaultMode.ResetText())
	}

	if _, err := s.store.ResetConversation(ctx, update.Message.From.ID); err != nil {
		s.logger.Error("failed to reset conversation", "chat_id", update.Message.Chat.ID, "telegram_user_id", update.Message.From.ID, "error", err)
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "리셋하다가 잠깐 꼬였어. 한 번만 다시 쳐줘.")
	}

	return s.bot.SendMessage(ctx, update.Message.Chat.ID, s.cfg.DefaultMode.ResetText())
}

func (s *Service) generateReply(ctx context.Context, input string, recentConversation []ollama.Message) (string, error) {
	messages := s.prompt.Build(PromptInput{
		Mode:               s.cfg.DefaultMode,
		UserInput:          input,
		RecentConversation: recentConversation,
	})

	reply, err := s.llm.Chat(ctx, messages)
	if err != nil {
		return "", err
	}

	reply = PostProcess(reply, s.cfg.ResponseMaxChars)
	if reply == "" {
		reply = s.cfg.DefaultMode.FallbackText()
	}

	return reply, nil
}
