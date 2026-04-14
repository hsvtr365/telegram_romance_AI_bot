package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
)

type StructuredExtractor struct {
	llm      LLM
	minChars int
}

const (
	structuredExtractTimeout   = 300 * time.Second
	structuredExtractWorkers   = 1
	structuredExtractQueueSize = 32
)

type structuredField struct {
	Value        string `json:"value"`
	Confidence   string `json:"confidence"`
	EvidenceText string `json:"evidence_text"`
	EvidenceType string `json:"evidence_type"`
}

func (s *structuredField) UnmarshalJSON(data []byte) error {
	// 1. Try to unmarshal as the standard object: {"value": "...", "confidence": "..."}
	type alias struct {
		Value        string `json:"value"`
		Confidence   string `json:"confidence"`
		EvidenceText string `json:"evidence_text"`
		EvidenceType string `json:"evidence_type"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err == nil {
		s.Value = a.Value
		s.Confidence = a.Confidence
		s.EvidenceText = a.EvidenceText
		s.EvidenceType = a.EvidenceType
		return nil
	}

	// 2. Try to unmarshal as a raw string: "..."
	var val string
	if err := json.Unmarshal(data, &val); err == nil {
		s.Value = val
		s.Confidence = "medium" // Fallback confidence
		return nil
	}

	// 3. Try to unmarshal as a number (sometimes happens for age): 25
	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		s.Value = fmt.Sprintf("%v", num)
		s.Confidence = "medium"
		return nil
	}

	return nil // Allow empty or unknown formats to prevent hard failure
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
	TraitType    string `json:"trait_type"`
	Value        string `json:"value"`
	Confidence   string `json:"confidence"`
	EvidenceType string `json:"evidence_type"`
}

func (s *structuredTrait) UnmarshalJSON(data []byte) error {
	// 1. Try to unmarshal as the standard object
	type alias struct {
		TraitType    string `json:"trait_type"`
		Value        string `json:"value"`
		Confidence   string `json:"confidence"`
		EvidenceType string `json:"evidence_type"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err == nil {
		s.TraitType = a.TraitType
		s.Value = a.Value
		s.Confidence = a.Confidence
		s.EvidenceType = a.EvidenceType
		return nil
	}

	// 2. Try to unmarshal as a raw string
	var val string
	if err := json.Unmarshal(data, &val); err == nil {
		s.Value = val
		s.TraitType = "like" // Assume string statements are typically likes/preferences in this context
		s.Confidence = "medium"
		s.EvidenceType = "explicit"
		return nil
	}

	// If everything fails, ignore it rather than crash the whole payload
	return nil
}

type structuredExtractionPayload struct {
	Profile structuredProfile `json:"profile"`
	Traits  []structuredTrait `json:"traits"`
}

type compatStructuredExtractionPayload struct {
	Name         structuredField   `json:"name"`
	Gender       structuredField   `json:"gender"`
	Age          structuredField   `json:"age"`
	Job          structuredField   `json:"job"`
	Occupation   structuredField   `json:"occupation"`
	CurrentFocus structuredField   `json:"current_focus"`
	Hobby        structuredField   `json:"hobby"`
	Interests    structuredField   `json:"interests"`
	Location     structuredField   `json:"location"`
	Affiliation  structuredField   `json:"affiliation"`
	Traits       []structuredTrait `json:"traits"`
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

func (s *Service) captureUserSignals(ctx context.Context, userID int64, input string, conversation *store.ConversationContext, sourceVersion int64) {
	if s == nil || conversation == nil || userID == 0 {
		return
	}

	// Delegate all profile and trait extraction to the async LLM runner.
	// Synchronous extraction is unnecessary because the main model already receives
	// the recent conversation history (30 turns) which includes the latest input.
	if s.shouldUseStructuredExtraction(input) {
		s.dispatchStructuredExtraction(userID, conversation.RecentConversation, input, sourceVersion)
	}
}

func (s *Service) dispatchStructuredExtraction(userID int64, history []model.Message, input string, sourceVersion int64) {
	if s == nil || s.extractor == nil || s.store == nil || s.analyticRunner == nil || userID == 0 {
		return
	}

	if sourceVersion <= 0 {
		sourceVersion = time.Now().UnixNano()
	}

	s.analyticRunner.Enqueue(AsyncTask{
		Key:     structuredExtractionTaskKey(userID),
		Version: sourceVersion,
		Build: func(ctx context.Context) (func(context.Context) error, error) {
			extracted, err := s.extractor.Extract(ctx, ollamaMessagesFromConversation(history), input)
			if err != nil {
				if s.logger != nil {
					s.logger.Warn("structured extraction failed", "user_id", userID, "error", err)
				}
				return nil, err
			}
			return func(commitCtx context.Context) error {
				if s.logger != nil {
					s.logger.Info("structured extraction success, persisting...", "user_id", userID, "has_profile", extracted.hasData())
				}
				s.persistStructuredExtraction(commitCtx, userID, input, history, extracted, sourceVersion)
				return nil
			}, nil
		},
		OnDrop: func(_ string, queueDepth int) {
			if s.logger != nil {
				s.logger.Warn("structured extraction queue full", "queue_depth", queueDepth, "user_id", userID)
			}
		},
		OnSuperseded: func(stage string) {
			if s.logger != nil {
				s.logger.Debug("structured extraction superseded", "stage", stage, "user_id", userID)
			}
		},
		OnComplete: func(duration time.Duration, err error, queueDepth int) {
			if s.logger == nil || err != nil {
				return
			}
			s.logger.Debug(
				"structured extraction completed",
				"metric", "structured_extract.total_ms",
				"duration_ms", duration.Milliseconds(),
				"queue_depth", queueDepth,
				"user_id", userID,
			)
		},
	})
}

func (s *Service) persistStructuredExtraction(ctx context.Context, userID int64, input string, history []model.Message, extracted structuredExtractionPayload, sourceMessageID int64) {
	if s == nil || s.store == nil || userID == 0 {
		return
	}

	// 1. Persist Traits (Additive)
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

	// 2. Persist Profile Candidates (Fallback for sync timeout)
	candidates := extractProfileCandidatesFromStructuredProfile(extracted.Profile, input, history)
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
			s.logger.Warn("failed to merge profile candidate (async)", "user_id", userID, "slot", candidate.SlotName, "value", candidate.CandidateValue, "error", err)
		}
	}
}

func (s *Service) extractStructuredProfileCandidates(ctx context.Context, history []model.Message, input string) ([]extractedProfileCandidate, error) {
	if s.extractor == nil {
		return nil, nil
	}
	extracted, err := s.extractor.Extract(ctx, ollamaMessagesFromConversation(history), input)
	if err != nil {
		return nil, err
	}
	return extractProfileCandidatesFromStructuredProfile(extracted.Profile, input, history), nil
}

func (s *Service) extractStructuredProfilePatch(ctx context.Context, history []model.Message, input string, existingProfile model.UserProfile) (pgstore.UpsertUserProfileParams, error) {
	candidates, err := s.extractStructuredProfileCandidates(ctx, history, input)
	if err != nil {
		return pgstore.UpsertUserProfileParams{}, err
	}

	var patch pgstore.UpsertUserProfileParams
	for _, c := range candidates {
		assignProfileSlotValue(&patch, c.SlotName, c.CandidateValue)
	}
	return patch, nil
}

func (s *Service) shouldUseStructuredExtraction(input string) bool {
	if s == nil || s.extractor == nil {
		return false
	}

	normalized := normalizeInput(input)
	if len([]rune(normalized)) < 2 { // Increased minimum slightly to skip pure noise/reactions
		return false
	}

	return true
}

func (e *StructuredExtractor) Extract(ctx context.Context, history []ollama.Message, input string) (structuredExtractionPayload, error) {
	if e == nil || e.llm == nil {
		return structuredExtractionPayload{}, fmt.Errorf("structured extractor is disabled")
	}

	var contextBuilder strings.Builder
	if len(history) > 0 {
		contextBuilder.WriteString("이전 대화 (태규는 AI 캐릭터입니다):\n")
		for _, msg := range history {
			role := "사용자"
			if msg.Role != "user" {
				role = "태규(AI)"
			}
			contextBuilder.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
		}
		contextBuilder.WriteString("\n")
	}
	contextBuilder.WriteString("최신 입력: " + strings.TrimSpace(input))

	messages := []ollama.Message{
		{
			Role: "system",
			Content: strings.TrimSpace(`
Extract user profile candidates from the latest user input.
Return one JSON object only. No markdown.
Use conversation history only to resolve pronouns or ellipsis. Never use history alone as evidence.
Do not infer or guess. Use empty string when unclear.
IMPORTANT: Extract ONLY the core value (e.g., just the name "수지", not "수지라고 불러").

Rules for Profile Fields:
- name: The user's name. Extract ONLY the name itself. 
  * ABSOLUTELY NO verbs or suffixes like "라고 불러", "이야", "입니다", "야".
  * If the user says "내 이름은 김수지", extract "김수지".
- gender: MUST be "남성" or "여성".
- age: Use digits like "25" or "1990년생".

Rules for Traits (likes/avoidances):
- 'value' MUST be the exact core noun/object/activity (e.g., "야식", "축구").
- extract ONLY when the user explicitly mentions a preference.
- NEVER extract meta-traits, personality impressions, or conversation status (e.g., "친근함", "기억력", "말 많음", "대화 중").
- If the user explicitly states they don't like, avoid, or hate something, use 'avoid'.
- If the user explicitly states they like or enjoy something, use 'like'.
- If the user explicitly retracts a previous statement, use 'delete'.

Evidence Rules:
- evidence_type must be one of: explicit | tentative | inferred | none
- explicit: the latest user input directly states the fact about the user.
- tentative: the latest user input weakly/self-speculatively suggests the fact.
- inferred: history suggest it, but the latest input does not directly state it. Use this for Traits ONLY if history is overwhelmingly clear.
- none: no usable evidence.
- evidence_text must quote the shortest supporting span from the latest user input when evidence_type is explicit or tentative.
- If the latest user input does not support a field, keep value empty and set evidence_type to none.
- Never treat assistant messages as evidence.

Schema Template (MUST follow exactly):
{"profile":{"name":{"value":"","confidence":"high","evidence_text":"","evidence_type":"none"},"gender":{"value":"","confidence":"high","evidence_text":"","evidence_type":"none"},"age":{"value":"","confidence":"high","evidence_text":"","evidence_type":"none"},"job":{"value":"","confidence":"high","evidence_text":"","evidence_type":"none"},"current_focus":{"value":"","confidence":"high","evidence_text":"","evidence_type":"none"},"hobby":{"value":"","confidence":"high","evidence_text":"","evidence_type":"none"},"location":{"value":"","confidence":"high","evidence_text":"","evidence_type":"none"},"affiliation":{"value":"","confidence":"high","evidence_text":"","evidence_type":"none"}},"traits":[{"trait_type":"like","value":"","confidence":"high","evidence_type":"explicit"}]}
`),
		},
		{
			Role:    "user",
			Content: contextBuilder.String(),
		},
	}

	raw, err := e.llm.Chat(ctx, messages)
	if err != nil {
		return structuredExtractionPayload{}, err
	}

	return parseStructuredExtractionPayload(raw)
}

func structuredExtractionTaskKey(userID int64) string {
	return fmt.Sprintf("structured_extract:%d", userID)
}

func parseStructuredExtractionPayload(raw string) (structuredExtractionPayload, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return structuredExtractionPayload{}, nil
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
		// No JSON skeleton
		return structuredExtractionPayload{}, nil
	}

	var payload structuredExtractionPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return structuredExtractionPayload{}, fmt.Errorf("decode structured extraction: %w (raw length: %d)", err, len(trimmed))
	}
	if !payload.hasData() {
		compat, err := parseCompatStructuredExtractionPayload(trimmed)
		if err != nil {
			return structuredExtractionPayload{}, err
		}
		if compat.hasData() {
			return compat, nil
		}
	}

	return payload, nil
}

func (p structuredExtractionPayload) hasData() bool {
	return strings.TrimSpace(p.Profile.Name.Value) != "" ||
		strings.TrimSpace(p.Profile.Gender.Value) != "" ||
		strings.TrimSpace(p.Profile.Age.Value) != "" ||
		strings.TrimSpace(p.Profile.Job.Value) != "" ||
		strings.TrimSpace(p.Profile.CurrentFocus.Value) != "" ||
		strings.TrimSpace(p.Profile.Hobby.Value) != "" ||
		strings.TrimSpace(p.Profile.Location.Value) != "" ||
		strings.TrimSpace(p.Profile.Affiliation.Value) != "" ||
		len(p.Traits) > 0
}

func parseCompatStructuredExtractionPayload(raw string) (structuredExtractionPayload, error) {
	var compat compatStructuredExtractionPayload
	if err := json.Unmarshal([]byte(raw), &compat); err != nil {
		return structuredExtractionPayload{}, fmt.Errorf("decode compat structured extraction: %w (raw length: %d)", err, len(raw))
	}

	payload := structuredExtractionPayload{
		Profile: structuredProfile{
			Name:         compat.Name,
			Gender:       compat.Gender,
			Age:          compat.Age,
			Job:          firstNonEmptyStructuredField(compat.Job, compat.Occupation),
			CurrentFocus: compat.CurrentFocus,
			Hobby:        firstNonEmptyStructuredField(compat.Hobby, compat.Interests),
			Location:     compat.Location,
			Affiliation:  compat.Affiliation,
		},
		Traits: compat.Traits,
	}

	return payload, nil
}

func firstNonEmptyStructuredField(values ...structuredField) structuredField {
	for _, value := range values {
		if strings.TrimSpace(value.Value) != "" {
			return value
		}
	}
	return structuredField{}
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

func mergeStructuredProfilePatchWithEvidence(base pgstore.UpsertUserProfileParams, existingProfile model.UserProfile, profile structuredProfile, input string) pgstore.UpsertUserProfileParams {
	if value := mergedStructuredProfileValue(profileSlotName, existingProfile.NameValue, profile.Name, input); value != "" {
		base.NameValue = value
	}
	if value := mergedStructuredProfileValue(profileSlotGender, existingProfile.GenderValue, profile.Gender, input); value != "" {
		base.GenderValue = value
	}
	if value := mergedStructuredProfileValue(profileSlotAge, existingProfile.AgeValue, profile.Age, input); value != "" {
		base.AgeValue = value
	}
	if value := mergedStructuredProfileValue(profileSlotJob, existingProfile.JobValue, profile.Job, input); value != "" {
		base.JobValue = value
	}
	if value := mergedStructuredProfileValue(profileSlotCurrentFocus, existingProfile.CurrentFocusValue, profile.CurrentFocus, input); value != "" {
		base.CurrentFocusValue = value
	}
	if value := mergedStructuredProfileValue(profileSlotHobby, existingProfile.HobbyValue, profile.Hobby, input); value != "" {
		base.HobbyValue = value
	}
	if value := mergedStructuredProfileValue(profileSlotLocation, existingProfile.LocationValue, profile.Location, input); value != "" {
		base.LocationValue = value
	}
	if value := mergedStructuredProfileValue(profileSlotAffiliation, existingProfile.AffiliationValue, profile.Affiliation, input); value != "" {
		base.AffiliationValue = value
	}
	return base
}

func mergedStructuredProfileValue(slot string, existingValue string, field structuredField, input string) string {
	nextValue := normalizeStructuredProfileValue(slot, field)
	if nextValue == "" {
		return ""
	}

	existingValue = normalizedProfileValue(existingValue)
	explicit := hasExplicitStructuredEvidence(slot, field, nextValue, input)
	if existingValue != "" && existingValue != nextValue && !explicit {
		return ""
	}
	if existingValue == nextValue {
		return ""
	}
	if existingValue == "" && !explicit {
		return ""
	}
	return nextValue
}

func hasExplicitStructuredEvidence(slot string, field structuredField, normalizedValue string, input string) bool {
	if normalizedValue == "" {
		return false
	}

	input = normalizeInput(input)
	if input == "" {
		return false
	}

	evidenceType := strings.ToLower(strings.TrimSpace(field.EvidenceType))
	evidenceText := normalizeInput(field.EvidenceText)
	if evidenceType != "" && evidenceType != "explicit" {
		return false
	}
	if evidenceText != "" {
		return strings.Contains(input, evidenceText)
	}
	return strings.Contains(input, normalizedValue)
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
		// More aggressive suffix removal to capture names in various speech styles
		for _, suffix := range []string{"라고 불러", "라고 해", "이라구", "이야", "에요", "예요", "입니다", "이야기", "이구요", "구요", "고요", "요", "야", "이"} {
			value = strings.TrimSuffix(value, suffix)
		}
		value = strings.TrimSpace(value)
		if value == "" || len([]rune(value)) < 1 || len([]rune(value)) > 12 || looksLikeNonName(value) {
			return ""
		}
		return value
	case profileSlotGender:
		normalized := strings.ToLower(strings.TrimSpace(value))
		switch {
		case strings.Contains(normalized, "female"), strings.Contains(normalized, "여성"), strings.Contains(normalized, "여자"):
			return "여성"
		case strings.Contains(normalized, "male"), strings.Contains(normalized, "남성"), strings.Contains(normalized, "남자"):
			return "남성"
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
	return confidence == "high" || confidence == "medium"
}

func mergeStructuredTraits(base []extractedTrait, extra []structuredTrait) []extractedTrait {
	merged := append([]extractedTrait{}, base...)
	for _, item := range extra {
		if !acceptStructuredTraitConfidence(normalizeConfidence(item.Confidence)) {
			continue
		}
		// Traits must have explicit or tentative evidence to prevent hallucinated meta-traits
		evidence := normalizeStructuredEvidenceType(item.EvidenceType)
		if evidence != "explicit" && evidence != "tentative" {
			continue
		}

		traitType := strings.TrimSpace(item.TraitType)
		if traitType != userTraitLike && traitType != userTraitAvoid && traitType != userTraitAllergy && traitType != userTraitDelete {
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
