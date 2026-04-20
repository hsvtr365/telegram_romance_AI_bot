package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/promptutil"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

type AsyncStateReviewInput struct {
	CurrentUserInput       string
	CurrentTonePhase       string
	CurrentStage           string
	CurrentDirection       string
	CurrentEmotionalTone   string
	CurrentInteractionMode string
	CurrentOpenLoop        string
	CurrentFocusTopicKey   string
	ActiveTopics           []string
	RecentConversation     []model.Message
}

type AsyncStateReviewResult struct {
	TonePhase       string   `json:"tone_phase"`
	SafetyAction    string   `json:"safety_action"`
	SafetyLockTurns int      `json:"safety_lock_turns"`
	RelationalStage string   `json:"relational_stage"`
	StageDirection  string   `json:"stage_direction"`
	EmotionalTone   string   `json:"emotional_tone"`
	InteractionMode string   `json:"interaction_mode"`
	OpenLoopSummary string   `json:"open_loop_summary"`
	FocusTopicLabel string   `json:"focus_topic_label"`
	Confidence      string   `json:"confidence"`
	Reason          string   `json:"reason"`
	EvidenceTexts   []string `json:"evidence_texts"`
}

type AsyncStateReviewer struct {
	llm LLM
}

func NewAsyncStateReviewer(llm LLM) *AsyncStateReviewer {
	if llm == nil {
		return nil
	}
	return &AsyncStateReviewer{llm: llm}
}

func (r *AsyncStateReviewer) Review(ctx context.Context, input AsyncStateReviewInput) (AsyncStateReviewResult, error) {
	if r == nil || r.llm == nil {
		return AsyncStateReviewResult{}, nil
	}

	raw, err := r.llm.Chat(ctx, buildAsyncStateReviewPrompt(input))
	if err != nil {
		return AsyncStateReviewResult{}, err
	}

	return parseAsyncStateReviewResult(raw)
}

func buildAsyncStateReviewPrompt(input AsyncStateReviewInput) []ollama.Message {
	var userSection strings.Builder
	promptutil.WriteLinesSection(&userSection, "Current State", []string{
		"tone_phase=" + strings.TrimSpace(input.CurrentTonePhase),
		"relational_stage=" + strings.TrimSpace(input.CurrentStage),
		"stage_direction=" + strings.TrimSpace(input.CurrentDirection),
		"emotional_tone=" + strings.TrimSpace(input.CurrentEmotionalTone),
		"interaction_mode=" + strings.TrimSpace(input.CurrentInteractionMode),
		"open_loop_summary=" + strings.TrimSpace(input.CurrentOpenLoop),
		"focus_topic_key=" + strings.TrimSpace(input.CurrentFocusTopicKey),
	})
	if len(input.ActiveTopics) > 0 {
		promptutil.WriteLinesSection(&userSection, "Active Topics", input.ActiveTopics)
	}
	if len(input.RecentConversation) > 0 {
		messages := make([]promptutil.MessageLine, 0, len(input.RecentConversation))
		for _, msg := range input.RecentConversation {
			messages = append(messages, promptutil.MessageLine{Role: msg.Role, Content: msg.Content, CreatedAt: msg.CreatedAt})
		}
		promptutil.WriteConversation(&userSection, "Recent Conversation", messages)
	}
	promptutil.WriteSection(&userSection, "Current User Input", input.CurrentUserInput)

	systemPrompt := strings.TrimSpace(`
You review a chat state machine after a user message.
Return one JSON object only. No markdown.
Use conservative updates:
- Keep tone_phase as "unchanged" unless the user clearly changes the tone.
- safety_action should be "deescalate" only when the user clearly rejects, feels uncomfortable, or questions sudden sexual tone.
- Only escalate to flirty or sexual when the user explicitly invites it.
- relational_stage should be one of: unchanged, opener, rapport, flirting, support, conflict, repair, planning.
- stage_direction should be one of: unchanged, warming, stable, escalating, cooling, shifting.
- tone_phase should be one of: unchanged, neutral, flirty, sexual.
- confidence must be one of: low, medium, high.
- focus_topic_label should match an active topic label when possible; otherwise empty string.
Schema:
{"tone_phase":"unchanged","safety_action":"none","safety_lock_turns":0,"relational_stage":"unchanged","stage_direction":"unchanged","emotional_tone":"","interaction_mode":"","open_loop_summary":"","focus_topic_label":"","confidence":"low","reason":"","evidence_texts":[""]}
`)

	return []ollama.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userSection.String()},
	}
}

func parseAsyncStateReviewResult(raw string) (AsyncStateReviewResult, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return AsyncStateReviewResult{}, fmt.Errorf("empty async state review response")
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

	var payload AsyncStateReviewResult
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return AsyncStateReviewResult{}, fmt.Errorf("decode async state review response: %w", err)
	}

	payload.TonePhase = normalizeAsyncTonePhase(payload.TonePhase)
	payload.SafetyAction = normalizeAsyncSafetyAction(payload.SafetyAction)
	payload.RelationalStage = normalizeAsyncStage(payload.RelationalStage)
	payload.StageDirection = normalizeAsyncStageDirection(payload.StageDirection)
	payload.Confidence = normalizeConfidence(payload.Confidence)
	payload.EmotionalTone = cleanMemoryText(payload.EmotionalTone)
	payload.InteractionMode = cleanMemoryText(payload.InteractionMode)
	payload.OpenLoopSummary = cleanMemoryText(payload.OpenLoopSummary)
	payload.FocusTopicLabel = cleanMemoryText(payload.FocusTopicLabel)
	payload.Reason = strings.TrimSpace(payload.Reason)
	payload.EvidenceTexts = normalizeEvidenceTexts(payload.EvidenceTexts)
	if payload.SafetyLockTurns < 0 {
		payload.SafetyLockTurns = 0
	}

	return payload, nil
}

func normalizeAsyncTonePhase(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "unchanged":
		return "unchanged"
	case phaseNeutral, phaseFlirty:
		return value
	case phaseSexual:
		return phaseFlirty // Redirect sexual to flirty
	default:
		return "unchanged"
	}
}

func normalizeAsyncSafetyAction(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "none":
		return "none"
	case "deescalate":
		return value
	default:
		return "none"
	}
}

func normalizeAsyncStage(value string) string {
	value = normalizeCurrentStage(value)
	switch value {
	case "":
		return "unchanged"
	default:
		return value
	}
}

func normalizeAsyncStageDirection(value string) string {
	value = normalizeStageDirection(value)
	switch value {
	case "":
		return "unchanged"
	default:
		return value
	}
}

func normalizeEvidenceTexts(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanMemoryText(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
