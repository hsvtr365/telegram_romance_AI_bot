package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func TestExtractProfileSlotHints_ParsesMultipleSlots(t *testing.T) {
	got := extractProfileSlotHints("내 이름은 수지야 나 26살이고 요즘 수영 자주 해. 서울 살아")

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

func TestExtractProfileSlotHints_ParsesGender(t *testing.T) {
	got := extractProfileSlotHints("난 여자야")
	if got.GenderValue != "여성" {
		t.Fatalf("expected gender slot, got %q", got.GenderValue)
	}
}

func TestExtractProfileSlotHints_DoesNotTreatJobAsName(t *testing.T) {
	got := extractProfileSlotHints("난 개발자야")
	if got.NameValue != "" {
		t.Fatalf("did not expect name slot, got %q", got.NameValue)
	}
	if got.JobValue != "개발자" {
		t.Fatalf("expected job slot, got %q", got.JobValue)
	}
}

func TestExtractProfileSlotHints_ParsesBirthYearStyleAge(t *testing.T) {
	got := extractProfileSlotHints("93년생이야")
	if got.AgeValue != "93년생" {
		t.Fatalf("expected birth-year style age slot, got %q", got.AgeValue)
	}
}

func TestBuildProfilePromptContext_SkipsMissingSlotsWhenConversationIsRich(t *testing.T) {
	ctx := buildProfilePromptContext(model.UserProfile{}, 6, "안녕", 3, time.Now())
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
