package chat

import (
	"context"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
)

func (s *Service) captureUserProfileSlots(ctx context.Context, userID int64, input string, conversation *store.ConversationContext) {
	if s.store == nil || userID == 0 || conversation == nil {
		return
	}

	patch := extractProfileSlotHints(input)
	if !hasProfileSlotUpdate(patch) {
		return
	}

	patch.UserID = userID
	profile, err := s.store.UpsertUserProfile(ctx, patch)
	if err != nil {
		s.logger.Warn("failed to upsert user profile", "user_id", userID, "error", err)
		return
	}

	conversation.UserProfile = profile
}
