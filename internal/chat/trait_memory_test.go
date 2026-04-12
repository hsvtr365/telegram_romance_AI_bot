package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func TestStructuredExtractor_ExtractsAvoidAndAllergy(t *testing.T) {
	extractor := NewStructuredExtractor(&fakeStructuredLLM{
		reply: `{"profile":{"name":{"value":"","confidence":"low"},"gender":{"value":"","confidence":"low"},"age":{"value":"","confidence":"low"},"job":{"value":"","confidence":"low"},"current_focus":{"value":"","confidence":"low"},"hobby":{"value":"","confidence":"low"},"location":{"value":"","confidence":"low"},"affiliation":{"value":"","confidence":"low"}},"traits":[{"trait_type":"avoid","value":"커피","confidence":"high"},{"trait_type":"allergy","value":"새우","confidence":"high"}]}`,
	}, 4)

	payload, err := extractor.Extract(context.Background(), nil, "나는 커피 안 마셔. 새우 알레르기 있어")
	if err != nil {
		t.Fatalf("extract structured traits: %v", err)
	}
	traits := mergeStructuredTraits(nil, payload.Traits)

	foundAvoid := false
	foundAllergy := false
	for _, trait := range traits {
		switch {
		case trait.TraitType == userTraitAvoid && trait.Value == "커피":
			foundAvoid = true
		case trait.TraitType == userTraitAllergy && trait.Value == "새우":
			foundAllergy = true
		}
	}

	if !foundAvoid || !foundAllergy {
		t.Fatalf("unexpected traits: %+v", traits)
	}
}

func TestStructuredExtractor_ExtractsLike(t *testing.T) {
	extractor := NewStructuredExtractor(&fakeStructuredLLM{
		reply: `{"profile":{"name":{"value":"","confidence":"low"},"gender":{"value":"","confidence":"low"},"age":{"value":"","confidence":"low"},"job":{"value":"","confidence":"low"},"current_focus":{"value":"","confidence":"low"},"hobby":{"value":"","confidence":"low"},"location":{"value":"","confidence":"low"},"affiliation":{"value":"","confidence":"low"}},"traits":[{"trait_type":"like","value":"민트초코","confidence":"high"}]}`,
	}, 4)

	payload, err := extractor.Extract(context.Background(), nil, "민트초코 좋아해")
	if err != nil {
		t.Fatalf("extract structured traits: %v", err)
	}
	traits := mergeStructuredTraits(nil, payload.Traits)
	if len(traits) != 1 {
		t.Fatalf("expected 1 trait, got %d", len(traits))
	}
	if traits[0].TraitType != userTraitLike || traits[0].Value != "민트초코" {
		t.Fatalf("unexpected trait: %+v", traits[0])
	}
}

func TestPromptBuilder_IncludesKnownUserTraits(t *testing.T) {
	builder := NewPromptBuilder()
	messages := builder.Build(PromptInput{
		UserInput: "안녕",
		UserTraits: []model.UserTrait{
			{TraitType: userTraitAvoid, DisplayValue: "커피"},
			{TraitType: userTraitAllergy, DisplayValue: "새우"},
		},
		ConversationStateMachine: model.ConversationStateMachine{
			TonePhase: phaseNeutral,
		},
	})

	content := messages[1].Content
	if !strings.Contains(content, "[Known User Traits]") {
		t.Fatalf("expected known user traits section, got %q", content)
	}
	if !strings.Contains(content, "avoid=커피") {
		t.Fatalf("expected avoid trait in prompt, got %q", content)
	}
	if !strings.Contains(content, "allergy=새우") {
		t.Fatalf("expected allergy trait in prompt, got %q", content)
	}
}
