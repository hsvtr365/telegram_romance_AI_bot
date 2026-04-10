package chat

import (
	"strings"
	"testing"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func TestExtractUserTraits_AvoidAndAllergy(t *testing.T) {
	traits := extractUserTraits("나는 커피 안 마셔. 새우 알레르기 있어")
	if len(traits) != 2 {
		t.Fatalf("expected 2 traits, got %d", len(traits))
	}

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

func TestExtractUserTraits_Like(t *testing.T) {
	traits := extractUserTraits("민트초코 좋아해")
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
		ConversationPhase: phaseNeutral,
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
