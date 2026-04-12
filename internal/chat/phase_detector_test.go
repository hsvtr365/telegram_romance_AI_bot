package chat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

type fakePhaseJudgeLLM struct {
	reply string
	err   error
	calls int
}

func (f *fakePhaseJudgeLLM) Chat(_ context.Context, _ []ollama.Message) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.reply, nil
}

func TestLoadPhaseRuleSet_FromJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "phase_rules.json")
	payload := `{
  "version": 1,
  "ambiguity": { "min_direct_score": 4, "max_score_gap": 2 },
  "rules": [
    {
      "name": "test_rule",
      "next_phase": "flirty",
      "priority": 55,
      "keywords": {
        "en": [
          { "text": "tease me", "match_mode": "phrase", "weight": 2 }
        ]
      }
    }
  ]
}`
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatalf("write phase rules: %v", err)
	}

	ruleSet, err := LoadPhaseRuleSet(path)
	if err != nil {
		t.Fatalf("load phase rules: %v", err)
	}

	if ruleSet.Ambiguity.MinDirectScore != 4 {
		t.Fatalf("unexpected min direct score: %+v", ruleSet.Ambiguity)
	}
	if len(ruleSet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(ruleSet.Rules))
	}
	if ruleSet.Rules[0].KeywordSets[0].Phrases[0].MatchMode != "phrase" {
		t.Fatalf("unexpected phrase config: %+v", ruleSet.Rules[0].KeywordSets[0].Phrases[0])
	}
}

func TestHybridPhaseDetector_UsesJudgeForWeakSignal(t *testing.T) {
	fakeLLM := &fakePhaseJudgeLLM{
		reply: `{"next_phase":"neutral","pause_sexual_turns":0,"confidence":"high","reason":"user switched to a work topic"}`,
	}
	detector := HybridPhaseDetector{
		rules: RuleBasedPhaseDetector{
			ruleSet: PhaseRuleSet{
				Ambiguity: phaseAmbiguitySettings{MinDirectScore: 3, MaxScoreGap: 1},
				Rules: []phaseRule{
					{
						Name:      "neutral_topic_shift",
						NextPhase: phaseNeutral,
						Priority:  40,
						KeywordSets: []phaseKeywordSet{
							{
								Language: "en",
								Phrases: []phasePhraseSpec{
									{Text: "project", MatchMode: "word", Weight: 2},
								},
							},
						},
					},
				},
			},
		},
		judge: NewPhaseJudge(fakeLLM, time.Second),
	}

	got := detector.Detect("Let's talk about the project instead.", phaseSexual)
	if got.NextPhase != phaseNeutral {
		t.Fatalf("expected neutral from judge, got %+v", got)
	}
	if got.DecisionSource != "llm" {
		t.Fatalf("expected llm decision source, got %+v", got)
	}
	if fakeLLM.calls != 1 {
		t.Fatalf("expected 1 judge call, got %d", fakeLLM.calls)
	}
}

func TestHybridPhaseDetector_SkipsJudgeForStrongSignal(t *testing.T) {
	fakeLLM := &fakePhaseJudgeLLM{
		reply: `{"next_phase":"neutral","pause_sexual_turns":0,"confidence":"high","reason":"should not be used"}`,
	}
	detector := HybridPhaseDetector{
		rules: RuleBasedPhaseDetector{
			ruleSet: PhaseRuleSet{
				Ambiguity: phaseAmbiguitySettings{MinDirectScore: 3, MaxScoreGap: 1},
				Rules: []phaseRule{
					{
						Name:      "sexual_explicit",
						NextPhase: phaseSexual,
						Priority:  90,
						KeywordSets: []phaseKeywordSet{
							{
								Language: "en",
								Phrases: []phasePhraseSpec{
									{Text: "sexting", MatchMode: "word", Weight: 3},
								},
							},
						},
					},
				},
			},
		},
		judge: NewPhaseJudge(fakeLLM, time.Second),
	}

	got := detector.Detect("Let's do some sexting tonight.", phaseNeutral)
	if got.NextPhase != phaseSexual {
		t.Fatalf("expected sexual phase, got %+v", got)
	}
	if got.DecisionSource != "rule" {
		t.Fatalf("expected rule decision source, got %+v", got)
	}
	if fakeLLM.calls != 0 {
		t.Fatalf("expected judge to be skipped, got %d calls", fakeLLM.calls)
	}
}
