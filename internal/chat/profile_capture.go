package chat

import (
	"context"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
)

const (
	profileHintSyncTimeout = 1200 * time.Millisecond
	profileHintMinChars    = 1
)

func (s *Service) captureUserProfileSlots(ctx context.Context, userID int64, input string, conversation *store.ConversationContext, sourceMessageID int64) {
	if s.store == nil || userID == 0 || conversation == nil {
		return
	}

	candidates, err := s.extractStructuredProfileCandidates(ctx, conversation.RecentConversation, input)
	if err != nil && s.logger != nil {
		s.logger.Debug("structured profile hint extraction failed", "user_id", userID, "error", err)
	}
	if len(candidates) == 0 {
		return
	}

	capturedAt := time.Now()
	for _, candidate := range candidates {
		if candidate.NormalizedValue == "" {
			continue
		}
		if _, err := s.store.MergeProfileCandidate(ctx, pgstore.MergeProfileCandidateParams{
			UserID:           userID,
			SlotName:         candidate.SlotName,
			CandidateValue:   candidate.CandidateValue,
			NormalizedValue:  candidate.NormalizedValue,
			BestEvidenceType: candidate.EvidenceType,
			BestConfidence:   candidate.Confidence,
			Evidence: model.ProfileCandidateEvidence{
				MessageID:    sourceMessageID,
				EvidenceText: candidate.EvidenceText,
				EvidenceType: candidate.EvidenceType,
				Confidence:   candidate.Confidence,
				CapturedAt:   capturedAt,
			},
		}); err != nil && s.logger != nil {
			s.logger.Warn("failed to merge profile candidate", "user_id", userID, "slot", candidate.SlotName, "value", candidate.CandidateValue, "error", err)
		}
	}

	topCandidates, err := s.store.ListTopProfileCandidatesByUserID(ctx, userID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to reload profile candidates", "user_id", userID, "error", err)
		}
		return
	}
	conversation.ProfileCandidates = topCandidates
}

func (s *Service) extractStructuredProfilePatch(ctx context.Context, history []model.Message, input string, existingProfile model.UserProfile) (pgstore.UpsertUserProfileParams, error) {
	if s == nil || s.extractor == nil {
		return pgstore.UpsertUserProfileParams{}, nil
	}

	normalized := normalizeInput(input)
	if len([]rune(normalized)) < profileHintMinChars {
		return pgstore.UpsertUserProfileParams{}, nil
	}

	syncCtx, cancel := context.WithTimeout(ctx, profileHintSyncTimeout)
	defer cancel()

	extracted, err := s.extractor.Extract(syncCtx, ollamaMessagesFromConversation(history), input)
	if err != nil {
		return pgstore.UpsertUserProfileParams{}, err
	}

	return mergeStructuredProfilePatchWithEvidence(pgstore.UpsertUserProfileParams{}, existingProfile, extracted.Profile, input), nil
}

func (s *Service) extractStructuredProfileCandidates(ctx context.Context, history []model.Message, input string) ([]extractedProfileCandidate, error) {
	if s == nil || s.extractor == nil {
		return nil, nil
	}

	normalized := normalizeInput(input)
	if len([]rune(normalized)) < profileHintMinChars {
		return nil, nil
	}

	syncCtx, cancel := context.WithTimeout(ctx, profileHintSyncTimeout)
	defer cancel()

	extracted, err := s.extractor.Extract(syncCtx, ollamaMessagesFromConversation(history), input)
	if err != nil {
		return nil, err
	}

	return extractProfileCandidatesFromStructuredProfile(extracted.Profile, input), nil
}
