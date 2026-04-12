package chat

import (
	"context"
	"testing"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
)

type fakeStructuredLLM struct {
	reply string
	err   error
	calls int
}

func (f *fakeStructuredLLM) Chat(_ context.Context, _ []ollama.Message) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.reply, nil
}

func TestParseStructuredExtractionPayload_StripsCodeFence(t *testing.T) {
	raw := "```json\n{\"profile\":{\"job\":{\"value\":\"개발자\",\"confidence\":\"high\"}},\"traits\":[{\"trait_type\":\"like\",\"value\":\"수영\",\"confidence\":\"medium\"}]}\n```"

	got, err := parseStructuredExtractionPayload(raw)
	if err != nil {
		t.Fatalf("parse structured extraction: %v", err)
	}

	if got.Profile.Job.Value != "개발자" {
		t.Fatalf("job mismatch: got %q", got.Profile.Job.Value)
	}
	if len(got.Traits) != 1 || got.Traits[0].Value != "수영" {
		t.Fatalf("traits mismatch: %#v", got.Traits)
	}
}

func TestParseStructuredExtractionPayload_CompatFlatSchema(t *testing.T) {
	raw := "```json\n{\"name\":\"수지\",\"gender\":\"Female (Inferred from name)\",\"occupation\":\"Developer\"}\n```"

	got, err := parseStructuredExtractionPayload(raw)
	if err != nil {
		t.Fatalf("parse compat structured extraction: %v", err)
	}

	if got.Profile.Name.Value != "수지" {
		t.Fatalf("name mismatch: got %q", got.Profile.Name.Value)
	}
	if got.Profile.Gender.Value != "Female (Inferred from name)" {
		t.Fatalf("gender mismatch: got %q", got.Profile.Gender.Value)
	}
	if got.Profile.Job.Value != "Developer" {
		t.Fatalf("job mismatch: got %q", got.Profile.Job.Value)
	}
}

func TestParseStructuredExtractionPayload_WithEvidence(t *testing.T) {
	raw := "```json\n{\"profile\":{\"job\":{\"value\":\"개발자\",\"confidence\":\"high\",\"evidence_text\":\"개발자야\",\"evidence_type\":\"explicit\"}},\"traits\":[]}\n```"

	got, err := parseStructuredExtractionPayload(raw)
	if err != nil {
		t.Fatalf("parse structured extraction with evidence: %v", err)
	}

	if got.Profile.Job.EvidenceText != "개발자야" {
		t.Fatalf("evidence text mismatch: got %q", got.Profile.Job.EvidenceText)
	}
	if got.Profile.Job.EvidenceType != "explicit" {
		t.Fatalf("evidence type mismatch: got %q", got.Profile.Job.EvidenceType)
	}
}

func TestMergeStructuredProfilePatch_UsesConservativeConfidence(t *testing.T) {
	base := pgstore.UpsertUserProfileParams{}
	profile := structuredProfile{
		Name:         structuredField{Value: "수지", Confidence: "medium"},
		Job:          structuredField{Value: "개발자", Confidence: "high"},
		CurrentFocus: structuredField{Value: "시험 준비", Confidence: "medium"},
	}

	got := mergeStructuredProfilePatch(base, profile)

	if got.NameValue != "수지" {
		t.Fatalf("name mismatch: %#v", got)
	}
	if got.JobValue != "개발자" {
		t.Fatalf("job mismatch: got %q", got.JobValue)
	}
	if got.CurrentFocusValue != "시험 준비" {
		t.Fatalf("current focus mismatch: got %q", got.CurrentFocusValue)
	}
}

func TestMergeStructuredTraits_DedupesAndFiltersLowConfidence(t *testing.T) {
	base := []extractedTrait{{TraitType: userTraitLike, Value: "커피"}}
	extra := []structuredTrait{
		{TraitType: userTraitLike, Value: "커피", Confidence: "high"},
		{TraitType: userTraitAllergy, Value: "새우", Confidence: "medium"},
		{TraitType: userTraitAvoid, Value: "오이", Confidence: "low"},
	}

	got := mergeStructuredTraits(base, extra)

	if len(got) != 2 {
		t.Fatalf("expected 2 traits, got %#v", got)
	}
	if got[1].TraitType != userTraitAllergy || got[1].Value != "새우" {
		t.Fatalf("unexpected merged trait: %#v", got[1])
	}
}

func TestShouldUseStructuredExtraction_UsesLengthGate(t *testing.T) {
	service := &Service{
		extractor: &StructuredExtractor{minChars: 8},
	}

	if !service.shouldUseStructuredExtraction("나는 요즘 시험 준비 중이야") {
		t.Fatalf("expected structured extraction to run for personal statement")
	}

	if service.shouldUseStructuredExtraction("안녕") {
		t.Fatalf("expected short input to skip structured extraction")
	}
}

func TestExtractStructuredProfilePatch_UsesLLMResult(t *testing.T) {
	service := &Service{
		extractor: NewStructuredExtractor(&fakeStructuredLLM{
			reply: `{"profile":{"name":{"value":"수지","confidence":"high","evidence_text":"","evidence_type":"inferred"},"gender":{"value":"여성","confidence":"high","evidence_text":"여자야","evidence_type":"explicit"},"age":{"value":"26살","confidence":"high","evidence_text":"","evidence_type":"inferred"},"job":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"current_focus":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"},"hobby":{"value":"수영","confidence":"medium","evidence_text":"수영","evidence_type":"explicit"},"location":{"value":"서울","confidence":"high","evidence_text":"서울","evidence_type":"explicit"},"affiliation":{"value":"","confidence":"low","evidence_text":"","evidence_type":"none"}},"traits":[]}`,
		}, 12),
	}

	got, err := service.extractStructuredProfilePatch(context.Background(), nil, "난 여자야. 서울 살고 수영 좋아해.", model.UserProfile{})
	if err != nil {
		t.Fatalf("extract structured profile patch: %v", err)
	}

	if got.GenderValue != "여성" {
		t.Fatalf("expected gender from llm result, got %#v", got)
	}
	if got.HobbyValue != "수영" {
		t.Fatalf("expected medium-confidence hobby from llm result, got %#v", got)
	}
	if got.LocationValue != "서울" {
		t.Fatalf("expected location from llm result, got %#v", got)
	}
}

func TestExtractStructuredProfilePatch_UsesCompatFlatLLMResult(t *testing.T) {
	service := &Service{
		extractor: NewStructuredExtractor(&fakeStructuredLLM{
			reply: "```json\n{\"name\":\"수지\",\"gender\":\"Female\",\"occupation\":\"Developer\"}\n```",
		}, 12),
	}

	got, err := service.extractStructuredProfilePatch(context.Background(), nil, "내이름은 수지야", model.UserProfile{})
	if err != nil {
		t.Fatalf("extract compat structured profile patch: %v", err)
	}

	if got.NameValue != "수지" {
		t.Fatalf("expected compat name, got %#v", got)
	}
	if got.GenderValue != "" {
		t.Fatalf("did not expect compat inferred gender without explicit evidence, got %#v", got)
	}
	if got.JobValue != "" {
		t.Fatalf("did not expect compat inferred job without explicit evidence, got %#v", got)
	}
}
