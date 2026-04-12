package chat

import (
	"strings"
	"testing"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func TestApplyAsyncStateReviewResult_DeescalatesAndTriggersSafety(t *testing.T) {
	current := model.ConversationStateMachine{
		SessionID:           11,
		Revision:            4,
		TonePhase:           phaseSexual,
		RelationalStage:     "flirting",
		StageDirection:      "escalating",
		SafetyLockUntilTurn: 0,
		Confidence:          model.ConfidenceMedium,
	}
	review := AsyncStateReviewResult{
		SafetyAction:    "deescalate",
		SafetyLockTurns: 4,
		Confidence:      model.ConfidenceHigh,
		Reason:          "user is uncomfortable",
		EvidenceTexts:   []string{"갑자기 왜 그래"},
	}

	next, changed, trigger := applyAsyncStateReviewResult(current, nil, review, 10, 99)
	if !changed {
		t.Fatalf("expected state to change")
	}
	if !trigger {
		t.Fatalf("expected safety interrupt trigger")
	}
	if next.TonePhase != phaseNeutral {
		t.Fatalf("expected tone phase to deescalate, got %+v", next)
	}
	if next.SafetyLockUntilTurn != 14 {
		t.Fatalf("expected safety lock to extend, got %+v", next)
	}
	if next.Revision != 5 {
		t.Fatalf("expected revision bump, got %+v", next)
	}
}

func TestParseAsyncStateReviewResult_NormalizesPayload(t *testing.T) {
	raw := "```json\n{\"tone_phase\":\"Neutral\",\"safety_action\":\"deescalate\",\"safety_lock_turns\":3,\"relational_stage\":\"support\",\"stage_direction\":\"warming\",\"emotional_tone\":\" tense \",\"interaction_mode\":\" supportive \",\"open_loop_summary\":\" 결과 묻기 \",\"focus_topic_label\":\" 시험 준비 \",\"confidence\":\"medium\",\"reason\":\"switch tone\",\"evidence_texts\":[\"  시험 준비 중  \"]}\n```"

	got, err := parseAsyncStateReviewResult(raw)
	if err != nil {
		t.Fatalf("parse async state review: %v", err)
	}
	if got.TonePhase != phaseNeutral {
		t.Fatalf("expected normalized tone phase, got %+v", got)
	}
	if got.SafetyAction != "deescalate" || got.SafetyLockTurns != 3 {
		t.Fatalf("unexpected safety normalization: %+v", got)
	}
	if got.RelationalStage != "support" || got.StageDirection != "warming" {
		t.Fatalf("unexpected stage normalization: %+v", got)
	}
	if !strings.Contains(got.OpenLoopSummary, "결과") {
		t.Fatalf("expected cleaned open loop summary, got %+v", got)
	}
}
