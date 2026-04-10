package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
)

type StructuredExtractor struct {
	llm      LLM
	minChars int
}

const structuredExtractTimeout = 8 * time.Second

type structuredField struct {
	Value      string `json:"value"`
	Confidence string `json:"confidence"`
}

type structuredProfile struct {
	Name         structuredField `json:"name"`
	Gender       structuredField `json:"gender"`
	Age          structuredField `json:"age"`
	Job          structuredField `json:"job"`
	CurrentFocus structuredField `json:"current_focus"`
	Hobby        structuredField `json:"hobby"`
	Location     structuredField `json:"location"`
	Affiliation  structuredField `json:"affiliation"`
}

type structuredTrait struct {
	TraitType  string `json:"trait_type"`
	Value      string `json:"value"`
	Confidence string `json:"confidence"`
}

type structuredExtractionPayload struct {
	Profile structuredProfile `json:"profile"`
	Traits  []structuredTrait `json:"traits"`
}

func NewStructuredExtractor(llm LLM, minChars int) *StructuredExtractor {
	if llm == nil {
		return nil
	}
	if minChars <= 0 {
		minChars = 12
	}
	return &StructuredExtractor{
		llm:      llm,
		minChars: minChars,
	}
}

func (s *Service) captureUserSignals(ctx context.Context, userID int64, input string, conversation *store.ConversationContext) {
	if s.store == nil || userID == 0 || conversation == nil {
		return
	}

	profilePatch := extractProfileSlotHints(input)
	traits := extractUserTraits(input)

	if hasProfileSlotUpdate(profilePatch) {
		profilePatch.UserID = userID
		profile, err := s.store.UpsertUserProfile(ctx, profilePatch)
		if err != nil {
			s.logger.Warn("failed to upsert user profile", "user_id", userID, "error", err)
		} else {
			conversation.UserProfile = profile
		}
	}

	if len(traits) > 0 {
		for _, trait := range traits {
			record, err := s.store.UpsertUserTrait(ctx, pgstore.UpsertUserTraitParams{
				UserID:          userID,
				TraitType:       trait.TraitType,
				NormalizedValue: normalizeTraitValue(trait.Value),
				DisplayValue:    cleanTraitValue(trait.Value),
				SourceText:      strings.TrimSpace(input),
			})
			if err != nil {
				s.logger.Warn("failed to upsert user trait", "user_id", userID, "trait_type", trait.TraitType, "value", trait.Value, "error", err)
				continue
			}
			conversation.UserTraits = mergeTrait(conversation.UserTraits, record)
		}
	}

	if s.shouldUseStructuredExtraction(input, profilePatch, traits) {
		s.dispatchStructuredExtraction(userID, input)
	}
}

func (s *Service) dispatchStructuredExtraction(userID int64, input string) {
	if s == nil || s.extractor == nil || s.store == nil || userID == 0 {
		return
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), structuredExtractTimeout)
		defer cancel()

		extracted, err := s.extractor.Extract(bgCtx, input)
		if err != nil {
			s.logger.Warn("structured extraction failed", "user_id", userID, "error", err)
			return
		}

		s.persistStructuredExtraction(bgCtx, userID, input, extracted)
	}()
}

func (s *Service) persistStructuredExtraction(ctx context.Context, userID int64, input string, extracted structuredExtractionPayload) {
	if s == nil || s.store == nil || userID == 0 {
		return
	}

	profilePatch := mergeStructuredProfilePatch(pgstore.UpsertUserProfileParams{}, extracted.Profile)
	if hasProfileSlotUpdate(profilePatch) {
		profilePatch.UserID = userID
		if _, err := s.store.UpsertUserProfile(ctx, profilePatch); err != nil {
			s.logger.Warn("failed to persist structured profile extraction", "user_id", userID, "error", err)
		}
	}

	traits := mergeStructuredTraits(nil, extracted.Traits)
	for _, trait := range traits {
		if _, err := s.store.UpsertUserTrait(ctx, pgstore.UpsertUserTraitParams{
			UserID:          userID,
			TraitType:       trait.TraitType,
			NormalizedValue: normalizeTraitValue(trait.Value),
			DisplayValue:    cleanTraitValue(trait.Value),
			SourceText:      strings.TrimSpace(input),
		}); err != nil {
			s.logger.Warn("failed to persist structured trait extraction", "user_id", userID, "trait_type", trait.TraitType, "value", trait.Value, "error", err)
		}
	}
}

func (s *Service) shouldUseStructuredExtraction(input string, profilePatch pgstore.UpsertUserProfileParams, traits []extractedTrait) bool {
	if s == nil || s.extractor == nil {
		return false
	}
	if hasProfileSlotUpdate(profilePatch) || len(traits) > 0 {
		return false
	}

	normalized := normalizeInput(input)
	if len([]rune(normalized)) < s.extractor.minChars {
		return false
	}

	return hasAnyKeyword(normalized,
		"내 ", "나는", "난 ", "저는", "전 ", "이름", "살", "년생",
		"사는", "살아", "요즘", "취미", "좋아", "싫어", "알레르기",
		"회사", "학교", "직장", "다녀", "준비", "공부", "운동",
	)
}

func (e *StructuredExtractor) Extract(ctx context.Context, input string) (structuredExtractionPayload, error) {
	if e == nil || e.llm == nil {
		return structuredExtractionPayload{}, fmt.Errorf("structured extractor is disabled")
	}

	messages := []ollama.Message{
		{
			Role: "system",
			Content: strings.TrimSpace(`
Extract explicit user facts from Korean text.
Return one JSON object only. No markdown.
Do not infer. Use empty string when unclear.
confidence: high | medium | low
gender: 남성 | 여성
trait_type: like | avoid | allergy
Schema:
{"profile":{"name":{"value":"","confidence":"low"},"gender":{"value":"","confidence":"low"},"age":{"value":"","confidence":"low"},"job":{"value":"","confidence":"low"},"current_focus":{"value":"","confidence":"low"},"hobby":{"value":"","confidence":"low"},"location":{"value":"","confidence":"low"},"affiliation":{"value":"","confidence":"low"}},"traits":[{"trait_type":"like","value":"","confidence":"low"}]}
`),
		},
		{
			Role:    "user",
			Content: "입력: " + strings.TrimSpace(input),
		},
	}

	raw, err := e.llm.Chat(ctx, messages)
	if err != nil {
		return structuredExtractionPayload{}, err
	}

	return parseStructuredExtractionPayload(raw)
}

func parseStructuredExtractionPayload(raw string) (structuredExtractionPayload, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return structuredExtractionPayload{}, fmt.Errorf("empty structured extraction response")
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

	var payload structuredExtractionPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return structuredExtractionPayload{}, fmt.Errorf("decode structured extraction: %w", err)
	}

	return payload, nil
}

func mergeStructuredProfilePatch(base pgstore.UpsertUserProfileParams, profile structuredProfile) pgstore.UpsertUserProfileParams {
	if base.NameValue == "" {
		base.NameValue = normalizeStructuredProfileValue(profileSlotName, profile.Name)
	}
	if base.GenderValue == "" {
		base.GenderValue = normalizeStructuredProfileValue(profileSlotGender, profile.Gender)
	}
	if base.AgeValue == "" {
		base.AgeValue = normalizeStructuredProfileValue(profileSlotAge, profile.Age)
	}
	if base.JobValue == "" {
		base.JobValue = normalizeStructuredProfileValue(profileSlotJob, profile.Job)
	}
	if base.CurrentFocusValue == "" {
		base.CurrentFocusValue = normalizeStructuredProfileValue(profileSlotCurrentFocus, profile.CurrentFocus)
	}
	if base.HobbyValue == "" {
		base.HobbyValue = normalizeStructuredProfileValue(profileSlotHobby, profile.Hobby)
	}
	if base.LocationValue == "" {
		base.LocationValue = normalizeStructuredProfileValue(profileSlotLocation, profile.Location)
	}
	if base.AffiliationValue == "" {
		base.AffiliationValue = normalizeStructuredProfileValue(profileSlotAffiliation, profile.Affiliation)
	}
	return base
}

func normalizeStructuredProfileValue(slot string, field structuredField) string {
	confidence := normalizeConfidence(field.Confidence)
	if !acceptStructuredProfileConfidence(slot, confidence) {
		return ""
	}

	value := strings.TrimSpace(field.Value)
	switch slot {
	case profileSlotName:
		value = cleanSlotValue(value)
		value = strings.TrimSuffix(value, "이")
		value = strings.TrimSuffix(value, "야")
		if value == "" || len([]rune(value)) < 2 || len([]rune(value)) > 8 || looksLikeNonName(value) {
			return ""
		}
		return value
	case profileSlotGender:
		switch strings.TrimSpace(value) {
		case "남자", "남성":
			return "남성"
		case "여자", "여성":
			return "여성"
		default:
			return ""
		}
	case profileSlotAge:
		value = cleanSlotValue(value)
		if value == "" {
			return ""
		}
		if strings.HasSuffix(value, "살") || strings.HasSuffix(value, "년생") {
			return value
		}
		if digitsOnly(value) != "" {
			return digitsOnly(value) + "살"
		}
		return ""
	case profileSlotJob, profileSlotCurrentFocus, profileSlotHobby, profileSlotLocation, profileSlotAffiliation:
		return cleanSlotValue(value)
	default:
		return ""
	}
}

func acceptStructuredProfileConfidence(slot string, confidence string) bool {
	switch slot {
	case profileSlotCurrentFocus, profileSlotHobby:
		return confidence == "high" || confidence == "medium"
	default:
		return confidence == "high"
	}
}

func mergeStructuredTraits(base []extractedTrait, extra []structuredTrait) []extractedTrait {
	merged := append([]extractedTrait{}, base...)
	for _, item := range extra {
		if !acceptStructuredTraitConfidence(normalizeConfidence(item.Confidence)) {
			continue
		}
		traitType := strings.TrimSpace(item.TraitType)
		if traitType != userTraitLike && traitType != userTraitAvoid && traitType != userTraitAllergy {
			continue
		}
		value := cleanTraitValue(item.Value)
		if value == "" {
			continue
		}
		merged = append(merged, extractedTrait{
			TraitType: traitType,
			Value:     value,
		})
	}
	return dedupeExtractedTraits(merged)
}

func acceptStructuredTraitConfidence(confidence string) bool {
	return confidence == "high" || confidence == "medium"
}

func normalizeConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func digitsOnly(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
