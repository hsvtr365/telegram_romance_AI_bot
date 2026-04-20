package chat

import (
	"context"
	"fmt"
	"strings"

	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/telegram"
)

func (s *Service) handleUpdateMyInfoCommand(ctx context.Context, update telegram.Update, input string) error {
	parts := strings.Fields(input)
	if len(parts) < 3 {
		err_msg := "사용법: !내정보변경 [항목] [내용]\n\n" +
			"지원 항목: 이름, 성별, 나이, 직업, 거주지, 소속, 취미, 관심사"
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, err_msg)
	}

	label := parts[1]
	value := strings.Join(parts[2:], " ")

	internalSlot := parseProfileSlotLabel(label)
	if internalSlot == "" {
		err_msg := fmt.Sprintf("알 수 없는 항목이야: '%s'\n지원 항목: 이름, 성별, 나이, 직업, 거주지, 소속, 취미, 관심사", label)
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, err_msg)
	}

	conversation, err := s.store.BootstrapContext(ctx, *update.Message, DefaultSessionMode, s.cfg.RecentTurnLimit)
	if err != nil {
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "세션 정보를 불러오는 데 실패했어.")
	}

	patch := pgstore.UpsertUserProfileParams{UserID: conversation.User.ID}
	
	// Re-using inner logic from batch review
	assignProfileSlotValue(&patch, internalSlot, value)

	if _, err := s.store.UpsertUserProfile(ctx, patch); err != nil {
		if s.logger != nil {
			s.logger.Error("failed to manual update profile", "error", err)
		}
		return s.bot.SendMessage(ctx, update.Message.Chat.ID, "내 정보 업데이트에 실패했어.")
	}

	slotLabel := profileSlotLabel(internalSlot)
	successMsg := fmt.Sprintf("✅ 내 정보가 업데이트되었어!\n• %s: %s", slotLabel, value)
	return s.bot.SendMessage(ctx, update.Message.Chat.ID, successMsg)
}
