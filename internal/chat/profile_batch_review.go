package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/promptutil"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
)

type AsyncProfileBatchReviewInput struct {
	ConfirmedProfile model.UserProfile
	TopCandidates    []model.ProfileCandidate
	ReviewWindow     []model.Message
}

type ProfileBatchReviewDecision struct {
	SlotName           string  `json:"slot_name"`
	Decision           string  `json:"decision"`
	Value              string  `json:"value"`
	EvidenceMessageIDs []int64 `json:"evidence_message_ids"`
	EvidenceType       string  `json:"evidence_type"`
	Reason             string  `json:"reason"`
}

type AsyncProfileBatchReviewResult struct {
	Slots []ProfileBatchReviewDecision `json:"slots"`
}

type AsyncProfileBatchReviewer struct {
	llm LLM
}

func NewAsyncProfileBatchReviewer(llm LLM) *AsyncProfileBatchReviewer {
	if llm == nil {
		return nil
	}
	return &AsyncProfileBatchReviewer{llm: llm}
}

func (r *AsyncProfileBatchReviewer) Review(ctx context.Context, input AsyncProfileBatchReviewInput) (AsyncProfileBatchReviewResult, error) {
	if r == nil || r.llm == nil {
		return AsyncProfileBatchReviewResult{}, nil
	}

	raw, err := r.llm.Chat(ctx, buildAsyncProfileBatchReviewPrompt(input))
	if err != nil {
		return AsyncProfileBatchReviewResult{}, err
	}

	return parseAsyncProfileBatchReviewResult(raw)
}

func buildAsyncProfileBatchReviewPrompt(input AsyncProfileBatchReviewInput) []ollama.Message {
	var userSection strings.Builder

	if summary := profileSummary(input.ConfirmedProfile); summary != "" {
		promptutil.WriteSection(&userSection, "Confirmed Profile", summary)
	}
	if summary := buildBatchReviewCandidateSummary(input.TopCandidates); summary != "" {
		promptutil.WriteSection(&userSection, "Top Active Candidates", summary)
	}
	if summary := buildBatchReviewWindowSummary(input.ReviewWindow); summary != "" {
		promptutil.WriteSection(&userSection, "Review Window", summary)
	}

	systemPrompt := strings.TrimSpace(`
You review user profile candidates in batches.
Return one JSON object only. No markdown.
Review only the provided latest 30 user turns and assistant messages in between.
Conservative rules:
- confirm only when the review window contains explicit user evidence for that slot
- use keep_candidate when evidence is tentative or incomplete
- use keep_existing when the current confirmed value is safer
- use dismiss when the candidate is contradicted, unsupported, or likely hallucinated
- evidence_message_ids must reference user message IDs from the review window
- evidence_type must be one of: explicit, tentative, none
- slot_name must be one of: name, gender, age, job, current_focus, hobby, location, affiliation
- decision must be one of: confirm, keep_existing, keep_candidate, dismiss
Schema:
{"slots":[{"slot_name":"job","decision":"keep_candidate","value":"","evidence_message_ids":[123],"evidence_type":"tentative","reason":""}]}
`)

	return []ollama.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userSection.String()},
	}
}

func parseAsyncProfileBatchReviewResult(raw string) (AsyncProfileBatchReviewResult, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return AsyncProfileBatchReviewResult{}, fmt.Errorf("empty async profile batch review response")
	}

	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end >= start {
		trimmed = trimmed[start : end+1]
	}

	var payload AsyncProfileBatchReviewResult
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return AsyncProfileBatchReviewResult{}, fmt.Errorf("decode async profile batch review response: %w", err)
	}

	normalized := make([]ProfileBatchReviewDecision, 0, len(payload.Slots))
	for _, decision := range payload.Slots {
		decision.SlotName = normalizeProfileSlotName(decision.SlotName)
		if decision.SlotName == "" {
			continue
		}
		decision.Decision = normalizeProfileBatchDecision(decision.Decision)
		decision.EvidenceType = normalizeBatchReviewEvidenceType(decision.EvidenceType)
		decision.Reason = strings.TrimSpace(decision.Reason)
		decision.Value = normalizeProfileBatchDecisionValue(decision.SlotName, decision.Value)
		if len(decision.EvidenceMessageIDs) == 0 {
			decision.EvidenceMessageIDs = nil
		}
		normalized = append(normalized, decision)
	}
	payload.Slots = normalized

	return payload, nil
}

func normalizeProfileSlotName(value string) string {
	switch strings.TrimSpace(value) {
	case profileSlotName:
		return profileSlotName
	case profileSlotGender:
		return profileSlotGender
	case profileSlotAge:
		return profileSlotAge
	case profileSlotJob:
		return profileSlotJob
	case profileSlotCurrentFocus:
		return profileSlotCurrentFocus
	case profileSlotHobby:
		return profileSlotHobby
	case profileSlotLocation:
		return profileSlotLocation
	case profileSlotAffiliation:
		return profileSlotAffiliation
	default:
		return ""
	}
}

func normalizeProfileBatchDecision(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "confirm":
		return "confirm"
	case "keep_existing":
		return "keep_existing"
	case "dismiss":
		return "dismiss"
	default:
		return "keep_candidate"
	}
}

func normalizeBatchReviewEvidenceType(value string) string {
	switch normalizeStructuredEvidenceType(value) {
	case "explicit":
		return "explicit"
	case "tentative":
		return "tentative"
	default:
		return "none"
	}
}

func normalizeProfileBatchDecisionValue(slot string, value string) string {
	normalized := normalizeStructuredProfileValue(slot, structuredField{
		Value:      value,
		Confidence: "high",
	})
	if normalized != "" {
		return normalized
	}
	return cleanSlotValue(value)
}

func buildBatchReviewCandidateSummary(candidates []model.ProfileCandidate) string {
	if len(candidates) == 0 {
		return ""
	}

	lines := make([]string, 0, len(candidates))
	for _, slot := range orderedProfileSlots() {
		for _, candidate := range candidates {
			if candidate.SlotName != slot || candidate.Status != model.ProfileCandidateStatusActive {
				continue
			}
			evidenceTexts := make([]string, 0, len(candidate.EvidenceMessages))
			for _, evidence := range candidate.EvidenceMessages {
				if text := strings.TrimSpace(evidence.EvidenceText); text != "" {
					evidenceTexts = append(evidenceTexts, text)
				}
			}
			lines = append(lines, fmt.Sprintf(
				"slot=%s value=%s evidence_type=%s mention_count=%d last_seen=%s evidence=%s",
				candidate.SlotName,
				normalizedProfileValue(candidate.CandidateValue),
				candidate.BestEvidenceType,
				candidate.MentionCount,
				candidate.LastSeenAt.Format(time.RFC3339),
				strings.Join(evidenceTexts, " | "),
			))
		}
	}
	return strings.Join(lines, "\n")
}

func buildBatchReviewWindowSummary(messages []model.Message) string {
	if len(messages) == 0 {
		return ""
	}

	lines := make([]string, 0, len(messages))
	for _, message := range messages {
		lines = append(lines, fmt.Sprintf("id=%d role=%s content=%s", message.ID, message.Role, strings.TrimSpace(message.Content)))
	}
	return strings.Join(lines, "\n")
}

func (s *Service) enqueueProfileBatchReview(conversation *store.ConversationContext, userTurnCount int, sourceMessageID int64) {
	if s == nil || s.store == nil || s.profileBatchReviewer == nil || s.analyticRunner == nil || conversation == nil {
		return
	}
	if conversation.User.ID == 0 || conversation.Session.ID == 0 || userTurnCount == 0 || userTurnCount%profileBatchReviewInterval != 0 {
		return
	}

	version := sourceMessageID
	if version <= 0 {
		version = time.Now().UnixNano()
	}
	userID := conversation.User.ID
	sessionID := conversation.Session.ID

	s.analyticRunner.Enqueue(AsyncTask{
		Key:     fmt.Sprintf("profile_batch_review:%d", userID),
		Version: version,
		Build: func(ctx context.Context) (func(context.Context) error, error) {
			reviewWindow, err := s.store.ListProfileReviewWindowMessages(ctx, sessionID, profileBatchReviewWindowTurns)
			if err != nil {
				return nil, err
			}
			if len(reviewWindow) == 0 {
				return nil, nil
			}

			confirmedProfile, err := s.store.GetUserProfileByUserID(ctx, userID)
			if err != nil {
				return nil, err
			}
			topCandidates, err := s.store.ListTopProfileCandidatesByUserID(ctx, userID)
			if err != nil {
				return nil, err
			}
			if len(topCandidates) == 0 {
				return nil, nil
			}

			review, err := s.profileBatchReviewer.Review(ctx, AsyncProfileBatchReviewInput{
				ConfirmedProfile: confirmedProfile,
				TopCandidates:    topCandidates,
				ReviewWindow:     reviewWindow,
			})
			if err != nil {
				return nil, err
			}

			return func(commitCtx context.Context) error {
				return s.applyProfileBatchReview(commitCtx, userID, confirmedProfile, topCandidates, reviewWindow, review)
			}, nil
		},
		OnDrop: func(_ string, queueDepth int) {
			if s.logger != nil {
				s.logger.Warn("profile batch review queue full", "user_id", userID, "queue_depth", queueDepth)
			}
		},
		OnComplete: func(duration time.Duration, err error, queueDepth int) {
			if s.logger == nil || err != nil {
				return
			}
			s.logger.Debug(
				"profile batch review completed",
				"metric", "profile_batch_review.total_ms",
				"duration_ms", duration.Milliseconds(),
				"queue_depth", queueDepth,
				"user_id", userID,
			)
		},
	})
}

func (s *Service) applyProfileBatchReview(ctx context.Context, userID int64, confirmedProfile model.UserProfile, topCandidates []model.ProfileCandidate, reviewWindow []model.Message, review AsyncProfileBatchReviewResult) error {
	if s == nil || s.store == nil || userID == 0 {
		return nil
	}

	candidatesBySlot := topProfileCandidateBySlot(topCandidates)
	userMessagesByID := make(map[int64]model.Message, len(reviewWindow))
	for _, message := range reviewWindow {
		if message.Role == "user" {
			userMessagesByID[message.ID] = message
		}
	}

	patch := pgstore.UpsertUserProfileParams{UserID: userID}
	now := time.Now()

	for _, decision := range review.Slots {
		candidate, ok := candidatesBySlot[decision.SlotName]
		if !ok {
			continue
		}

		reviewedAt := now
		validEvidenceIDs, evidenceType := validateProfileDecisionEvidence(decision, userMessagesByID)
		value := decision.Value
		if value == "" {
			value = normalizedProfileValue(candidate.CandidateValue)
		}

		switch decision.Decision {
		case "confirm":
			if evidenceType != "explicit" || len(validEvidenceIDs) == 0 {
				_ = s.store.UpdateProfileCandidateStatus(ctx, pgstore.UpdateProfileCandidateStatusParams{
					UserID:          userID,
					SlotName:        candidate.SlotName,
					NormalizedValue: candidate.NormalizedValue,
					Status:          model.ProfileCandidateStatusActive,
					ReviewedAt:      reviewedAt,
				})
				continue
			}

			assignProfileSlotValue(&patch, decision.SlotName, value)
			if s.logger != nil {
				s.logger.Info("profile slot confirmed by batch review", "user_id", userID, "slot", decision.SlotName, "value", value)
			}
			if err := s.store.UpdateProfileCandidateStatus(ctx, pgstore.UpdateProfileCandidateStatusParams{
				UserID:          userID,
				SlotName:        candidate.SlotName,
				NormalizedValue: candidate.NormalizedValue,
				Status:          model.ProfileCandidateStatusConfirmed,
				ReviewedAt:      reviewedAt,
				PromotedAt:      now,
			}); err != nil {
				return err
			}
		case "dismiss":
			if err := s.store.UpdateProfileCandidateStatus(ctx, pgstore.UpdateProfileCandidateStatusParams{
				UserID:          userID,
				SlotName:        candidate.SlotName,
				NormalizedValue: candidate.NormalizedValue,
				Status:          model.ProfileCandidateStatusDismissed,
				ReviewedAt:      reviewedAt,
			}); err != nil {
				return err
			}
		default:
			if err := s.store.UpdateProfileCandidateStatus(ctx, pgstore.UpdateProfileCandidateStatusParams{
				UserID:          userID,
				SlotName:        candidate.SlotName,
				NormalizedValue: candidate.NormalizedValue,
				Status:          model.ProfileCandidateStatusActive,
				ReviewedAt:      reviewedAt,
			}); err != nil {
				return err
			}
		}
	}

	if hasProfileSlotUpdate(patch) {
		if _, err := s.store.UpsertUserProfile(ctx, patch); err != nil {
			return err
		}
	}

	return nil
}

func validateProfileDecisionEvidence(decision ProfileBatchReviewDecision, userMessagesByID map[int64]model.Message) ([]int64, string) {
	if len(decision.EvidenceMessageIDs) == 0 {
		return nil, "none"
	}

	validIDs := make([]int64, 0, len(decision.EvidenceMessageIDs))
	for _, id := range decision.EvidenceMessageIDs {
		if _, ok := userMessagesByID[id]; ok {
			validIDs = append(validIDs, id)
		}
	}
	if len(validIDs) == 0 {
		return nil, "none"
	}
	return validIDs, normalizeBatchReviewEvidenceType(decision.EvidenceType)
}

func assignProfileSlotValue(patch *pgstore.UpsertUserProfileParams, slot string, value string) {
	if patch == nil {
		return
	}
	switch slot {
	case profileSlotName:
		patch.NameValue = value
	case profileSlotGender:
		patch.GenderValue = value
	case profileSlotAge:
		patch.AgeValue = value
	case profileSlotJob:
		patch.JobValue = value
	case profileSlotCurrentFocus:
		patch.CurrentFocusValue = value
	case profileSlotHobby:
		patch.HobbyValue = value
	case profileSlotLocation:
		patch.LocationValue = value
	case profileSlotAffiliation:
		patch.AffiliationValue = value
	}
}
