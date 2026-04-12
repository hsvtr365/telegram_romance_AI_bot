package chat

import "testing"

func TestDetectConversationPhaseSignal_Sexual(t *testing.T) {
	got := detectConversationPhaseSignal("나는 섹스톡이야. 더 꼴리게 해", phaseNeutral)
	if got.NextPhase != phaseSexual {
		t.Fatalf("expected sexual phase, got %+v", got)
	}
	if got.MatchedLanguage != "ko" {
		t.Fatalf("expected korean match metadata, got %+v", got)
	}
}

func TestDetectConversationPhaseSignal_DeescalatesOnDiscomfort(t *testing.T) {
	got := detectConversationPhaseSignal("갑자기 왜 갈망 이야기야 문맥 이상하다", phaseSexual)
	if got.NextPhase != phaseNeutral {
		t.Fatalf("expected neutral phase, got %+v", got)
	}
	if got.PauseSexualTurns <= 0 {
		t.Fatalf("expected sexual pause, got %+v", got)
	}
}

func TestDetectConversationPhaseSignal_SupportsEnglishSexualTrigger(t *testing.T) {
	got := detectConversationPhaseSignal("Let's do some sexting tonight. Turn me on.", phaseNeutral)
	if got.NextPhase != phaseSexual {
		t.Fatalf("expected sexual phase, got %+v", got)
	}
	if got.MatchedLanguage != "en" {
		t.Fatalf("expected english match metadata, got %+v", got)
	}
}

func TestDetectConversationPhaseSignal_SupportsJapaneseDeescalation(t *testing.T) {
	got := detectConversationPhaseSignal("急にどうした？文脈おかしいよ", phaseSexual)
	if got.NextPhase != phaseNeutral {
		t.Fatalf("expected neutral phase, got %+v", got)
	}
	if got.PauseSexualTurns <= 0 {
		t.Fatalf("expected sexual pause, got %+v", got)
	}
	if got.MatchedLanguage != "ja" {
		t.Fatalf("expected japanese match metadata, got %+v", got)
	}
}

func TestDetectConversationPhaseSignal_SupportsEnglishNeutralTopicShift(t *testing.T) {
	got := detectConversationPhaseSignal("Let's switch and talk about the code and debugging.", phaseSexual)
	if got.NextPhase != phaseNeutral {
		t.Fatalf("expected neutral phase, got %+v", got)
	}
	if got.MatchedRule != "neutral_topic_shift" {
		t.Fatalf("expected neutral topic rule, got %+v", got)
	}
}

func TestDetectConversationPhaseSignal_DoesNotDowngradeSexualToFlirty(t *testing.T) {
	got := detectConversationPhaseSignal("seduce me a little", phaseSexual)
	if got.NextPhase != "" {
		t.Fatalf("expected no downgrade from sexual, got %+v", got)
	}
}

func TestSanitizeByConversationPhase_PreservesTextInNeutral(t *testing.T) {
	got := sanitizeByConversationPhase("네 몸이 먼저 말하고 있잖아.\n이제 숨기지 마.", phaseNeutral)
	if got != "네 몸이 먼저 말하고 있잖아.\n이제 숨기지 마." {
		t.Fatalf("expected neutral phase sanitizer to preserve text, got %q", got)
	}
}
