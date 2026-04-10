package chat

import (
	"testing"

	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
)

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

func TestMergeStructuredProfilePatch_UsesConservativeConfidence(t *testing.T) {
	base := pgstore.UpsertUserProfileParams{}
	profile := structuredProfile{
		Name:         structuredField{Value: "수지", Confidence: "medium"},
		Job:          structuredField{Value: "개발자", Confidence: "high"},
		CurrentFocus: structuredField{Value: "시험 준비", Confidence: "medium"},
	}

	got := mergeStructuredProfilePatch(base, profile)

	if got.NameValue != "" {
		t.Fatalf("name should remain empty on medium confidence: %#v", got)
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

func TestShouldUseStructuredExtraction_OnlyWhenRegexMisses(t *testing.T) {
	service := &Service{
		extractor: &StructuredExtractor{minChars: 8},
	}

	if !service.shouldUseStructuredExtraction("나는 요즘 시험 준비 중이야", pgstore.UpsertUserProfileParams{}, nil) {
		t.Fatalf("expected structured extraction to run for personal statement")
	}

	if service.shouldUseStructuredExtraction("내 이름은 수지야", pgstore.UpsertUserProfileParams{NameValue: "수지"}, nil) {
		t.Fatalf("should skip structured extraction when regex already found a profile value")
	}

	if service.shouldUseStructuredExtraction("커피 좋아해", pgstore.UpsertUserProfileParams{}, []extractedTrait{{TraitType: userTraitLike, Value: "커피"}}) {
		t.Fatalf("should skip structured extraction when regex already found a trait")
	}
}
