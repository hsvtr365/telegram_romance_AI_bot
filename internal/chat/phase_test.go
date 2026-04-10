package chat

import "testing"

func TestDetectConversationPhaseSignal_Sexual(t *testing.T) {
	got := detectConversationPhaseSignal("나는 섹스톡이야. 더 꼴리게 해", phaseNeutral)
	if got.NextPhase != phaseSexual {
		t.Fatalf("expected sexual phase, got %+v", got)
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

func TestSanitizeByConversationPhase_PreservesTextInNeutral(t *testing.T) {
	got := sanitizeByConversationPhase("네 몸이 먼저 말하고 있잖아.\n이제 숨기지 마.", phaseNeutral)
	if got != "네 몸이 먼저 말하고 있잖아.\n이제 숨기지 마." {
		t.Fatalf("expected neutral phase sanitizer to preserve text, got %q", got)
	}
}
