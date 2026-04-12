package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func TestExtractStructuredProfileCandidates_ExplicitCandidate(t *testing.T) {
	service := &Service{
		extractor: NewStructuredExtractor(&fakeStructuredLLM{
			reply: `{"profile":{"name":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"gender":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"age":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"job":{"value":"개발자","confidence":"high","evidence_text":"개발자야","evidence_type":"explicit"},"current_focus":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"hobby":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"location":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"affiliation":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"}},"traits":[]}`,
		}, 1),
	}

	got, err := service.extractStructuredProfileCandidates(context.Background(), nil, "난 개발자야")
	if err != nil {
		t.Fatalf("extract structured profile candidates: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %#v", got)
	}
	if got[0].SlotName != profileSlotJob || got[0].CandidateValue != "개발자" || got[0].EvidenceType != "explicit" {
		t.Fatalf("unexpected candidate: %#v", got[0])
	}
}

func TestExtractStructuredProfileCandidates_TentativeCandidate(t *testing.T) {
	service := &Service{
		extractor: NewStructuredExtractor(&fakeStructuredLLM{
			reply: `{"profile":{"name":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"gender":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"age":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"job":{"value":"개발자","confidence":"medium","evidence_text":"개발자일지도","evidence_type":"tentative"},"current_focus":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"hobby":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"location":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"affiliation":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"}},"traits":[]}`,
		}, 1),
	}

	got, err := service.extractStructuredProfileCandidates(context.Background(), nil, "나 개발자일지도?")
	if err != nil {
		t.Fatalf("extract structured profile candidates: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 tentative candidate, got %#v", got)
	}
	if got[0].EvidenceType != "tentative" {
		t.Fatalf("expected tentative evidence type, got %#v", got[0])
	}
}

func TestExtractStructuredProfileCandidates_BlocksInferredCandidate(t *testing.T) {
	service := &Service{
		extractor: NewStructuredExtractor(&fakeStructuredLLM{
			reply: `{"profile":{"name":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"gender":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"age":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"job":{"value":"UX/UI 디자이너","confidence":"high","evidence_text":"","evidence_type":"inferred"},"current_focus":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"hobby":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"location":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"affiliation":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"}},"traits":[]}`,
		}, 1),
	}

	got, err := service.extractStructuredProfileCandidates(context.Background(), nil, "피카레스크 장르 얘기 재밌네")
	if err != nil {
		t.Fatalf("extract structured profile candidates: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected inferred candidate to be blocked, got %#v", got)
	}
}

func TestCandidateProfileSummary_SkipsConfirmedDuplicate(t *testing.T) {
	summary := candidateProfileSummary(
		model.UserProfile{JobValue: "개발자"},
		[]model.ProfileCandidate{
			{SlotName: profileSlotJob, CandidateValue: "개발자", NormalizedValue: "개발자", Status: model.ProfileCandidateStatusActive},
			{SlotName: profileSlotHobby, CandidateValue: "수영", NormalizedValue: "수영", Status: model.ProfileCandidateStatusActive},
		},
	)

	if strings.Contains(summary, "job=개발자") {
		t.Fatalf("did not expect duplicate confirmed candidate, got %q", summary)
	}
	if !strings.Contains(summary, "hobby=수영 (후보)") {
		t.Fatalf("expected hobby candidate summary, got %q", summary)
	}
}

func TestRenderProfileFieldWithCandidate_ConflictShowsBothStates(t *testing.T) {
	got := renderProfileFieldWithCandidate(
		model.UserProfile{JobValue: "UX/UI 디자이너"},
		map[string]model.ProfileCandidate{
			profileSlotJob: {
				SlotName:        profileSlotJob,
				CandidateValue:  "개발자",
				NormalizedValue: "개발자",
				Status:          model.ProfileCandidateStatusActive,
			},
		},
		profileSlotJob,
	)

	if !strings.Contains(got, "UX/UI 디자이너 [확정]") || !strings.Contains(got, "개발자 [후보]") {
		t.Fatalf("expected confirmed/candidate conflict rendering, got %q", got)
	}
}

func TestPromptBuilder_IncludesCandidateUserProfileGuardrail(t *testing.T) {
	builder := NewPromptBuilder()
	messages := builder.Build(PromptInput{
		UserInput:               "안녕",
		CandidateProfileSummary: "job=개발자 (후보)",
	})

	content := messages[1].Content
	if !strings.Contains(content, "[Candidate User Profile]") {
		t.Fatalf("expected candidate user profile section, got %q", content)
	}
	if !strings.Contains(content, "사실처럼 단정하지 말고 참고만") {
		t.Fatalf("expected candidate guardrail, got %q", content)
	}
	if !strings.Contains(content, "job=개발자 (후보)") {
		t.Fatalf("expected candidate summary, got %q", content)
	}
}

func TestParseAsyncProfileBatchReviewResult_NormalizesPayload(t *testing.T) {
	raw := "```json\n{\"slots\":[{\"slot_name\":\"job\",\"decision\":\"confirm\",\"value\":\"개발자\",\"evidence_message_ids\":[101,102],\"evidence_type\":\"explicit\",\"reason\":\"최근 명시 발화\"},{\"slot_name\":\"age\",\"decision\":\"dismiss\",\"value\":\"26\",\"evidence_message_ids\":[],\"evidence_type\":\"inferred\",\"reason\":\"근거 없음\"}]}\n```"

	got, err := parseAsyncProfileBatchReviewResult(raw)
	if err != nil {
		t.Fatalf("parse async profile batch review: %v", err)
	}
	if len(got.Slots) != 2 {
		t.Fatalf("expected 2 slot decisions, got %#v", got.Slots)
	}
	if got.Slots[0].Decision != "confirm" || got.Slots[0].EvidenceType != "explicit" {
		t.Fatalf("unexpected first decision normalization: %#v", got.Slots[0])
	}
	if got.Slots[1].EvidenceType != "none" {
		t.Fatalf("expected inferred evidence to normalize to none, got %#v", got.Slots[1])
	}
	if got.Slots[1].Value != "26살" {
		t.Fatalf("expected age value normalization, got %#v", got.Slots[1])
	}
}
