package chat

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	channelx "github.com/hsvtr365/telegram_romance_AI_bot/internal/channel"
)

func (s *Service) handleAddCustomSlotCommand(ctx context.Context, message channelx.InboundMessage, input string) error {
	parts := strings.SplitN(input, " ", 2)
	if len(parts) < 2 {
		return s.sendText(ctx, message, "사용법: !추가슬롯 [내용]")
	}

	content := strings.TrimSpace(parts[1])
	if content == "" {
		return s.sendText(ctx, message, "추가할 내용을 입력해줘.")
	}

	conversation, err := s.store.BootstrapContext(ctx, message, s.defaultMode(), s.cfg.RecentTurnLimit)
	if err != nil {
		return s.sendText(ctx, message, "세션 정보를 불러오는 데 실패했어.")
	}

	slots := conversation.CustomSlots

	if len(slots) >= 10 {
		return s.sendText(ctx, message, "슬롯이 가득 찼어 (최대 10개). 삭제 후 다시 시도해줘. 조회 명령어: `!추가슬롯조회`")
	}

	// Find the lowest available index (0-9)
	used := make(map[int]bool)
	for _, slot := range slots {
		used[slot.SlotIndex] = true
	}

	targetIndex := -1
	for i := 0; i < 10; i++ {
		if !used[i] {
			targetIndex = i
			break
		}
	}

	nowStr := time.Now().Format("[2006-01-02]")
	finalContent := fmt.Sprintf("%s %s", nowStr, content)

	_, err = s.store.UpsertSessionCustomSlot(ctx, conversation.Session.ID, targetIndex, finalContent)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to add custom slot", "error", err)
		}
		return s.sendText(ctx, message, "설정 추가 중 오류가 발생했어.")
	}

	return s.sendText(ctx, message, fmt.Sprintf("✅ 슬롯 [%d]에 설정이 추가되었어.\n저장된 내용: %s", targetIndex, finalContent))
}

func (s *Service) handleDeleteCustomSlotCommand(ctx context.Context, message channelx.InboundMessage, input string) error {
	parts := strings.SplitN(input, " ", 2)
	if len(parts) < 2 {
		return s.sendText(ctx, message, "사용법: !추가슬롯삭제 [번호]")
	}

	indexStr := strings.TrimSpace(parts[1])
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 || index > 9 {
		return s.sendText(ctx, message, "올바른 슬롯 번호(0~9)를 입력해줘.")
	}

	conversation, err := s.store.BootstrapContext(ctx, message, s.defaultMode(), s.cfg.RecentTurnLimit)
	if err != nil {
		return s.sendText(ctx, message, "세션 정보를 불러오는 데 실패했어.")
	}

	err = s.store.DeleteSessionCustomSlot(ctx, conversation.Session.ID, index)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to delete custom slot", "error", err)
		}
		return s.sendText(ctx, message, "설정 삭제 중 오류가 발생했어.")
	}

	return s.sendText(ctx, message, fmt.Sprintf("🗑 슬롯 [%d]의 설정이 삭제되었어.", index))
}

func (s *Service) handleListCustomSlotsCommand(ctx context.Context, message channelx.InboundMessage) error {
	conversation, err := s.store.BootstrapContext(ctx, message, s.defaultMode(), s.cfg.RecentTurnLimit)
	if err != nil {
		return s.sendText(ctx, message, "세션 정보를 불러오는 데 실패했어.")
	}

	slots := conversation.CustomSlots
	if len(slots) == 0 {
		return s.sendText(ctx, message, "현재 저장된 추가 슬롯이 없어.\n`!추가슬롯 [내용]`으로 등록해봐.")
	}

	sort.Slice(slots, func(i, j int) bool {
		return slots[i].SlotIndex < slots[j].SlotIndex
	})

	var sb strings.Builder
	sb.WriteString("📌 *[저장된 추가 설정 목록]*\n\n")
	for _, slot := range slots {
		sb.WriteString(fmt.Sprintf("• [%d] %s\n", slot.SlotIndex, slot.Content))
	}

	sb.WriteString("\n*삭제 명령어*: `!추가슬롯삭제 [번호]`")

	return s.sendText(ctx, message, sb.String())
}
