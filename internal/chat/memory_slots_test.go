package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func TestParseMemorySlotAnalysisResult_StripsCodeFence(t *testing.T) {
	raw := "```json\n{\"topics\":[{\"label\":\"시험 준비\",\"summary\":\"이번 주 시험 준비가 이어짐\",\"status\":\"active\",\"importance\":84,\"confidence\":\"medium\",\"evidence_texts\":[\"요즘 시험 준비 중\"]}],\"conversation_state\":{\"current_stage\":\"support\",\"stage_direction\":\"warming\",\"emotional_tone\":\"tense\",\"interaction_mode\":\"supportive\",\"open_loop_summary\":\"시험 끝나면 결과를 다시 물어볼 것\",\"focus_topic_label\":\"시험 준비\",\"confidence\":\"medium\",\"evidence_texts\":[\"시험 준비 중\"]},\"resolved_topics\":[\"이전 잡담\"]}\n```"

	got, err := parseMemorySlotAnalysisResult(raw)
	if err != nil {
		t.Fatalf("parse memory slot analysis: %v", err)
	}
	if len(got.Topics) != 1 || got.Topics[0].Label != "시험 준비" {
		t.Fatalf("unexpected parsed topics: %#v", got.Topics)
	}
	if got.ConversationState.CurrentStage != "support" {
		t.Fatalf("unexpected conversation state: %#v", got.ConversationState)
	}
	if len(got.ResolvedTopics) != 1 || got.ResolvedTopics[0] != "이전 잡담" {
		t.Fatalf("unexpected resolved topics: %#v", got.ResolvedTopics)
	}
}

func TestMergeMemorySlotAnalysis_UpdatesAndResolvesTopics(t *testing.T) {
	now := time.Date(2026, 4, 11, 20, 0, 0, 0, time.UTC)
	existing := []model.TopicSlot{
		{
			SessionID:    11,
			SlotKey:      topicKeyFromLabel("시험 준비"),
			TopicLabel:   "시험 준비",
			Summary:      "시험이 다가와 긴장 중",
			Status:       model.TopicSlotStatusActive,
			Importance:   75,
			Confidence:   model.ConfidenceMedium,
			MentionCount: 1,
			FirstSeenAt:  now.Add(-24 * time.Hour),
			LastSeenAt:   now.Add(-2 * time.Hour),
		},
		{
			SessionID:    11,
			SlotKey:      topicKeyFromLabel("이전 잡담"),
			TopicLabel:   "이전 잡담",
			Summary:      "가벼운 수다",
			Status:       model.TopicSlotStatusActive,
			Importance:   20,
			Confidence:   model.ConfidenceLow,
			MentionCount: 1,
			FirstSeenAt:  now.Add(-48 * time.Hour),
			LastSeenAt:   now.Add(-25 * time.Hour),
		},
	}

	result := model.MemorySlotAnalysisResult{
		Topics: []model.AnalyzedTopicSlot{
			{
				Label:      "시험 준비",
				Summary:    "이번 주 시험 준비가 계속 이어짐",
				Status:     "active",
				Importance: 88,
				Confidence: "high",
			},
		},
		ConversationState: model.AnalyzedConversationState{
			CurrentStage:    "support",
			StageDirection:  "warming",
			EmotionalTone:   "tense",
			InteractionMode: "supportive",
			OpenLoopSummary: "시험 끝나고 다시 결과를 물어볼 것",
			FocusTopicLabel: "시험 준비",
			Confidence:      "medium",
		},
		ResolvedTopics: []string{"이전 잡담"},
	}

	mergedTopics, mergedState := MergeMemorySlotAnalysis(existing, model.ConversationStateSlot{SessionID: 11}, result, 33, now)
	byKey := make(map[string]model.TopicSlot, len(mergedTopics))
	for _, slot := range mergedTopics {
		byKey[slot.SlotKey] = slot
	}

	mainTopic := byKey[topicKeyFromLabel("시험 준비")]
	if mainTopic.MentionCount != 2 {
		t.Fatalf("expected mention count to increment, got %#v", mainTopic)
	}
	if mainTopic.Importance != 88 || mainTopic.Confidence != model.ConfidenceHigh {
		t.Fatalf("expected updated main topic, got %#v", mainTopic)
	}

	resolved := byKey[topicKeyFromLabel("이전 잡담")]
	if resolved.Status != model.TopicSlotStatusResolved {
		t.Fatalf("expected resolved topic, got %#v", resolved)
	}

	if mergedState.CurrentStage != "support" || mergedState.FocusTopicKey != topicKeyFromLabel("시험 준비") {
		t.Fatalf("unexpected merged state: %#v", mergedState)
	}
}

func TestBuildMemoryPromptSections_FiltersLowConfidenceSingleMention(t *testing.T) {
	slots := []model.TopicSlot{
		{
			TopicLabel:   "시험 준비",
			Summary:      "이번 주 시험이 다가옴",
			Status:       model.TopicSlotStatusActive,
			Confidence:   model.ConfidenceMedium,
			Importance:   85,
			MentionCount: 1,
			LastSeenAt:   time.Now(),
		},
		{
			TopicLabel:   "애매한 새 화제",
			Summary:      "한 번 스쳐 지나감",
			Status:       model.TopicSlotStatusActive,
			Confidence:   model.ConfidenceLow,
			Importance:   60,
			MentionCount: 1,
			LastSeenAt:   time.Now(),
		},
		{
			TopicLabel:   "반복된 저신뢰 화제",
			Summary:      "두 번 이상 다시 나옴",
			Status:       model.TopicSlotStatusActive,
			Confidence:   model.ConfidenceLow,
			Importance:   55,
			MentionCount: 2,
			LastSeenAt:   time.Now(),
		},
	}
	state := model.ConversationStateSlot{
		CurrentStage:    "support",
		StageDirection:  "warming",
		EmotionalTone:   "tense",
		InteractionMode: "supportive",
		OpenLoopSummary: "시험 끝난 뒤 다시 확인할 것",
		Confidence:      model.ConfidenceMedium,
	}

	sections := BuildMemoryPromptSections(slots, state)

	if !strings.Contains(sections.ActiveTopicsText, "시험 준비") {
		t.Fatalf("expected medium-confidence active topic in prompt: %q", sections.ActiveTopicsText)
	}
	if strings.Contains(sections.ActiveTopicsText, "애매한 새 화제") {
		t.Fatalf("did not expect low-confidence single-mention topic: %q", sections.ActiveTopicsText)
	}
	if !strings.Contains(sections.ActiveTopicsText, "반복된 저신뢰 화제") {
		t.Fatalf("expected repeated low-confidence topic to be visible: %q", sections.ActiveTopicsText)
	}
	if !strings.Contains(sections.ConversationStateText, "stage=support") {
		t.Fatalf("expected conversation state section: %q", sections.ConversationStateText)
	}
	if !strings.Contains(sections.OpenLoopsText, "시험 끝난 뒤") {
		t.Fatalf("expected open loop summary: %q", sections.OpenLoopsText)
	}
	if !strings.Contains(sections.MemorySummary, "[Active Topic Slots]") {
		t.Fatalf("expected combined memory summary: %q", sections.MemorySummary)
	}
}
