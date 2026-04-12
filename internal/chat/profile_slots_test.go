package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func TestExtractStructuredProfilePatch_ParsesMultipleSlots(t *testing.T) {
	service := &Service{
		extractor: NewStructuredExtractor(&fakeStructuredLLM{
			reply: `{"profile":{"name":{"value":"수지","confidence":"high","evidence_text":"수지","evidence_type":"explicit"},"gender":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"age":{"value":"26살","confidence":"high","evidence_text":"26살","evidence_type":"explicit"},"job":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"current_focus":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"hobby":{"value":"수영","confidence":"medium","evidence_text":"수영","evidence_type":"explicit"},"location":{"value":"서울","confidence":"high","evidence_text":"서울","evidence_type":"explicit"},"affiliation":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"}},"traits":[]}`,
		}, 4),
	}
	got, err := service.extractStructuredProfilePatch(context.Background(), nil, "내 이름은 수지야 나 26살이고 요즘 수영 자주 해. 서울 살아", model.UserProfile{})
	if err != nil {
		t.Fatalf("extract structured profile patch: %v", err)
	}

	if got.NameValue != "수지" {
		t.Fatalf("expected name slot, got %q", got.NameValue)
	}
	if got.AgeValue != "26살" {
		t.Fatalf("expected age slot, got %q", got.AgeValue)
	}
	if got.HobbyValue != "수영" {
		t.Fatalf("expected hobby slot, got %q", got.HobbyValue)
	}
	if got.LocationValue != "서울" {
		t.Fatalf("expected location slot, got %q", got.LocationValue)
	}
}

func TestExtractStructuredProfilePatch_ParsesGender(t *testing.T) {
	service := &Service{
		extractor: NewStructuredExtractor(&fakeStructuredLLM{
			reply: `{"profile":{"name":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"gender":{"value":"여성","confidence":"high","evidence_text":"여자야","evidence_type":"explicit"},"age":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"job":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"current_focus":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"hobby":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"location":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"affiliation":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"}},"traits":[]}`,
		}, 4),
	}
	got, err := service.extractStructuredProfilePatch(context.Background(), nil, "난 여자야", model.UserProfile{})
	if err != nil {
		t.Fatalf("extract structured profile patch: %v", err)
	}
	if got.GenderValue != "여성" {
		t.Fatalf("expected gender slot, got %q", got.GenderValue)
	}
}

func TestExtractStructuredProfilePatch_DoesNotTreatJobAsName(t *testing.T) {
	service := &Service{
		extractor: NewStructuredExtractor(&fakeStructuredLLM{
			reply: `{"profile":{"name":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"gender":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"age":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"job":{"value":"개발자","confidence":"high","evidence_text":"개발자야","evidence_type":"explicit"},"current_focus":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"hobby":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"location":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"affiliation":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"}},"traits":[]}`,
		}, 4),
	}
	got, err := service.extractStructuredProfilePatch(context.Background(), nil, "난 개발자야", model.UserProfile{})
	if err != nil {
		t.Fatalf("extract structured profile patch: %v", err)
	}
	if got.NameValue != "" {
		t.Fatalf("did not expect name slot, got %q", got.NameValue)
	}
	if got.JobValue != "개발자" {
		t.Fatalf("expected job slot, got %q", got.JobValue)
	}
}

func TestExtractStructuredProfilePatch_ParsesBirthYearStyleAge(t *testing.T) {
	service := &Service{
		extractor: NewStructuredExtractor(&fakeStructuredLLM{
			reply: `{"profile":{"name":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"gender":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"age":{"value":"93년생","confidence":"high","evidence_text":"93년생","evidence_type":"explicit"},"job":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"current_focus":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"hobby":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"location":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"affiliation":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"}},"traits":[]}`,
		}, 4),
	}
	got, err := service.extractStructuredProfilePatch(context.Background(), nil, "93년생이야", model.UserProfile{})
	if err != nil {
		t.Fatalf("extract structured profile patch: %v", err)
	}
	if got.AgeValue != "93년생" {
		t.Fatalf("expected birth-year style age slot, got %q", got.AgeValue)
	}
}

func TestExtractStructuredProfilePatch_DoesNotOverwriteExistingJobWithoutExplicitEvidence(t *testing.T) {
	service := &Service{
		extractor: NewStructuredExtractor(&fakeStructuredLLM{
			reply: `{"profile":{"name":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"gender":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"age":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"job":{"value":"UX/UI 디자이너","confidence":"high","evidence_text":"","evidence_type":"inferred"},"current_focus":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"hobby":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"location":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"affiliation":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"}},"traits":[]}`,
		}, 4),
	}
	got, err := service.extractStructuredProfilePatch(context.Background(), nil, "맞아 피카레스크 장르 좋아해", model.UserProfile{JobValue: "개발자"})
	if err != nil {
		t.Fatalf("extract structured profile patch: %v", err)
	}
	if got.JobValue != "" {
		t.Fatalf("did not expect overwrite patch, got %q", got.JobValue)
	}
}

func TestBuildProfilePromptContext_SkipsMissingSlotsWhenConversationIsRich(t *testing.T) {
	ctx := buildProfilePromptContext(model.UserProfile{}, 20, "안녕", 3, time.Now())
	if ctx.Enabled {
		t.Fatalf("expected rich conversation to skip collection prompt")
	}
}

func TestBuildProfilePromptContext_TargetsExpiredMutableSlot(t *testing.T) {
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	profile := model.UserProfile{
		NameValue:                  "수지",
		GenderValue:                "여성",
		AgeValue:                   "26",
		JobValue:                   "개발자",
		CurrentFocusValue:          "자격증 공부",
		CurrentFocusConfirmedAt:    now.Add(-15 * 24 * time.Hour),
		HobbyValue:                 "수영",
		HobbyConfirmedAt:           now.Add(-5 * 24 * time.Hour),
		LocationValue:              "서울",
		LocationConfirmedAt:        now.Add(-5 * 24 * time.Hour),
		AffiliationValue:           "회사원",
		AffiliationConfirmedAt:     now.Add(-5 * 24 * time.Hour),
		LastRequestedSlot:          "",
		LastRequestedUserTurnCount: 0,
	}

	ctx := buildProfilePromptContext(profile, 2, "요즘 바빠", 8, now)
	if !ctx.Enabled {
		t.Fatalf("expected expired slot prompt to be enabled")
	}
	if ctx.TargetSlot != profileSlotCurrentFocus {
		t.Fatalf("expected target %q, got %q", profileSlotCurrentFocus, ctx.TargetSlot)
	}
	if !strings.Contains(ctx.Instruction, "자격증 공부") {
		t.Fatalf("expected instruction to mention expired value, got %q", ctx.Instruction)
	}
}

func TestBuildProfilePromptContext_SkipsWhenCollectionPaused(t *testing.T) {
	ctx := buildProfilePromptContext(model.UserProfile{
		CollectionPausedUntilTurn: 8,
	}, 1, "안녕", 5, time.Now())
	if ctx.Enabled {
		t.Fatalf("expected collection prompt to be paused")
	}
}

func TestDetectProfilePromptDiscomfort(t *testing.T) {
	if !detectProfilePromptDiscomfort("음 너무 부담스럽다...") {
		t.Fatalf("expected discomfort signal")
	}
	if detectProfilePromptDiscomfort("오늘 날씨 좋네") {
		t.Fatalf("did not expect discomfort signal")
	}
}

func TestCleanSlotValue_TreatsUnknownPlaceholderAsEmpty(t *testing.T) {
	if got := cleanSlotValue("미정"); got != "" {
		t.Fatalf("expected placeholder to be dropped, got %q", got)
	}
	if got := cleanSlotValue("unknown"); got != "" {
		t.Fatalf("expected placeholder to be dropped, got %q", got)
	}
}

func TestNormalizedProfileValue_TreatsUnknownPlaceholderAsEmpty(t *testing.T) {
	if got := normalizedProfileValue("알 수 없음"); got != "" {
		t.Fatalf("expected placeholder to normalize empty, got %q", got)
	}
	if got := normalizedProfileValue("미정"); got != "" {
		t.Fatalf("expected placeholder to normalize empty, got %q", got)
	}
}

func TestMissingProfileSlots_TreatsUnknownPlaceholderAsMissing(t *testing.T) {
	profile := model.UserProfile{
		NameValue:  "수지",
		JobValue:   "미정",
		HobbyValue: "알 수 없음",
	}

	missing := missingProfileSlots(profile)
	if !containsString(missing, profileSlotJob) {
		t.Fatalf("expected job to be missing, got %#v", missing)
	}
	if !containsString(missing, profileSlotHobby) {
		t.Fatalf("expected hobby to be missing, got %#v", missing)
	}
	if containsString(missing, profileSlotName) {
		t.Fatalf("did not expect name to be missing, got %#v", missing)
	}
}

func TestPromptBuilder_IncludesKnownUserProfile(t *testing.T) {
	builder := NewPromptBuilder()
	messages := builder.Build(PromptInput{
		UserInput: "안녕",
		UserProfile: model.UserProfile{
			NameValue:   "수지",
			GenderValue: "여성",
			HobbyValue:  "수영",
		},
	})

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if !strings.Contains(messages[1].Content, "[Known User Profile]") {
		t.Fatalf("expected known profile section in prompt")
	}
	if !strings.Contains(messages[1].Content, "name=수지") {
		t.Fatalf("expected stored name in prompt, got %q", messages[1].Content)
	}
	if !strings.Contains(messages[1].Content, "gender=여성") {
		t.Fatalf("expected stored gender in prompt, got %q", messages[1].Content)
	}
	if !strings.Contains(messages[1].Content, "hobby=수영") {
		t.Fatalf("expected stored hobby in prompt, got %q", messages[1].Content)
	}
}

func TestPromptBuilder_SkipsUnknownProfilePlaceholders(t *testing.T) {
	builder := NewPromptBuilder()
	messages := builder.Build(PromptInput{
		UserInput: "안녕",
		UserProfile: model.UserProfile{
			NameValue:  "수지",
			JobValue:   "미정",
			HobbyValue: "unknown",
		},
	})

	content := messages[1].Content
	if !strings.Contains(content, "name=수지") {
		t.Fatalf("expected confirmed name in prompt, got %q", content)
	}
	if strings.Contains(content, "job=미정") {
		t.Fatalf("did not expect unknown job placeholder in prompt, got %q", content)
	}
	if strings.Contains(content, "hobby=unknown") {
		t.Fatalf("did not expect unknown hobby placeholder in prompt, got %q", content)
	}
}
