package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const defaultPhaseRulesPath = "configs/conversation_phase_rules.json"

type PhaseRuleSet struct {
	Ambiguity phaseAmbiguitySettings
	Rules     []phaseRule
}

type phaseAmbiguitySettings struct {
	MinDirectScore int `json:"min_direct_score"`
	MaxScoreGap    int `json:"max_score_gap"`
}

type phaseRule struct {
	Name                 string
	NextPhase            string
	PauseSexualTurns     int
	Priority             int
	BlockedCurrentPhases []string
	KeywordSets          []phaseKeywordSet
}

type phaseKeywordSet struct {
	Language string
	Phrases  []phasePhraseSpec
}

type phasePhraseSpec struct {
	Text      string `json:"text"`
	MatchMode string `json:"match_mode"`
	Weight    int    `json:"weight"`
}

type phaseRulesFile struct {
	Version   int                    `json:"version"`
	Ambiguity phaseAmbiguitySettings `json:"ambiguity"`
	Rules     []phaseRuleFile        `json:"rules"`
}

type phaseRuleFile struct {
	Name                 string                       `json:"name"`
	NextPhase            string                       `json:"next_phase"`
	PauseSexualTurns     int                          `json:"pause_sexual_turns"`
	Priority             int                          `json:"priority"`
	BlockedCurrentPhases []string                     `json:"blocked_current_phases"`
	Keywords             map[string][]phasePhraseSpec `json:"keywords"`
}

func LoadPhaseRuleSet(path string) (PhaseRuleSet, error) {
	if strings.TrimSpace(path) == "" {
		path = defaultPhaseRulesPath
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return PhaseRuleSet{}, fmt.Errorf("read phase rules: %w", err)
	}

	var file phaseRulesFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return PhaseRuleSet{}, fmt.Errorf("decode phase rules: %w", err)
	}

	ruleSet, err := compilePhaseRuleSet(file)
	if err != nil {
		return PhaseRuleSet{}, err
	}

	return ruleSet, nil
}

func compilePhaseRuleSet(file phaseRulesFile) (PhaseRuleSet, error) {
	ruleSet := PhaseRuleSet{
		Ambiguity: file.Ambiguity.withDefaults(),
		Rules:     make([]phaseRule, 0, len(file.Rules)),
	}

	for _, item := range file.Rules {
		rule, err := compilePhaseRule(item)
		if err != nil {
			return PhaseRuleSet{}, err
		}
		ruleSet.Rules = append(ruleSet.Rules, rule)
	}

	return ruleSet, nil
}

func compilePhaseRule(file phaseRuleFile) (phaseRule, error) {
	rule := phaseRule{
		Name:                 strings.TrimSpace(file.Name),
		NextPhase:            normalizePhaseName(file.NextPhase),
		PauseSexualTurns:     maxPhaseInt(file.PauseSexualTurns, 0),
		Priority:             file.Priority,
		BlockedCurrentPhases: normalizePhaseNames(file.BlockedCurrentPhases),
		KeywordSets:          make([]phaseKeywordSet, 0, len(file.Keywords)),
	}

	if rule.Name == "" {
		return phaseRule{}, fmt.Errorf("phase rule name is required")
	}
	if !isSupportedPhase(rule.NextPhase) {
		return phaseRule{}, fmt.Errorf("phase rule %q has unsupported next_phase %q", rule.Name, file.NextPhase)
	}

	languages := make([]string, 0, len(file.Keywords))
	for language := range file.Keywords {
		languages = append(languages, language)
	}
	sort.Strings(languages)

	for _, language := range languages {
		phrases := file.Keywords[language]
		if len(phrases) == 0 {
			continue
		}

		compiled := phaseKeywordSet{
			Language: strings.TrimSpace(language),
			Phrases:  make([]phasePhraseSpec, 0, len(phrases)),
		}

		for _, phrase := range phrases {
			text := strings.TrimSpace(phrase.Text)
			if text == "" {
				continue
			}
			compiled.Phrases = append(compiled.Phrases, phasePhraseSpec{
				Text:      text,
				MatchMode: normalizePhaseMatchMode(phrase.MatchMode),
				Weight:    maxPhaseInt(phrase.Weight, 1),
			})
		}

		if len(compiled.Phrases) > 0 {
			rule.KeywordSets = append(rule.KeywordSets, compiled)
		}
	}

	if len(rule.KeywordSets) == 0 {
		return phaseRule{}, fmt.Errorf("phase rule %q has no keywords", rule.Name)
	}

	return rule, nil
}

func normalizePhaseMatchMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "word":
		return "word"
	case "phrase":
		return "phrase"
	case "exact":
		return "exact"
	default:
		return "contains"
	}
}

func normalizePhaseNames(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizePhaseName(value)
		if isSupportedPhase(normalized) {
			out = append(out, normalized)
		}
	}
	return out
}

func normalizePhaseName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isSupportedPhase(value string) bool {
	switch normalizePhaseName(value) {
	case phaseNeutral, phaseFlirty, phaseSexual:
		return true
	default:
		return false
	}
}

func (s phaseAmbiguitySettings) withDefaults() phaseAmbiguitySettings {
	if s.MinDirectScore <= 0 {
		s.MinDirectScore = 3
	}
	if s.MaxScoreGap <= 0 {
		s.MaxScoreGap = 1
	}
	return s
}

func defaultPhaseRuleSet() PhaseRuleSet {
	return PhaseRuleSet{
		Ambiguity: phaseAmbiguitySettings{
			MinDirectScore: 3,
			MaxScoreGap:    1,
		},
		Rules: []phaseRule{
			{
				Name:             "deescalate_discomfort",
				NextPhase:        phaseNeutral,
				PauseSexualTurns: 4,
				Priority:         100,
				KeywordSets: []phaseKeywordSet{
					{
						Language: "en",
						Phrases: []phasePhraseSpec{
							{Text: "too much", MatchMode: "phrase", Weight: 3},
							{Text: "why suddenly", MatchMode: "phrase", Weight: 3},
							{Text: "out of nowhere", MatchMode: "phrase", Weight: 3},
							{Text: "that's weird", MatchMode: "phrase", Weight: 3},
							{Text: "this is weird", MatchMode: "phrase", Weight: 3},
							{Text: "stop that", MatchMode: "phrase", Weight: 3},
							{Text: "don't do that", MatchMode: "phrase", Weight: 3},
						},
					},
					{
						Language: "ja",
						Phrases: []phasePhraseSpec{
							{Text: "急にどうした", MatchMode: "contains", Weight: 3},
							{Text: "いきなりどうした", MatchMode: "contains", Weight: 3},
							{Text: "やりすぎ", MatchMode: "contains", Weight: 3},
							{Text: "変だよ", MatchMode: "contains", Weight: 3},
							{Text: "文脈おかしい", MatchMode: "contains", Weight: 3},
							{Text: "やめて", MatchMode: "contains", Weight: 3},
						},
					},
					{
						Language: "ko",
						Phrases: []phasePhraseSpec{
							{Text: "부담", MatchMode: "contains", Weight: 3},
							{Text: "부담돼", MatchMode: "contains", Weight: 3},
							{Text: "과해", MatchMode: "contains", Weight: 3},
							{Text: "뜬금", MatchMode: "contains", Weight: 2},
							{Text: "갑자기 왜", MatchMode: "contains", Weight: 3},
							{Text: "왜 갑자기", MatchMode: "contains", Weight: 3},
							{Text: "문맥 이상", MatchMode: "contains", Weight: 3},
							{Text: "별걸 다", MatchMode: "contains", Weight: 2},
							{Text: "멍청아", MatchMode: "contains", Weight: 2},
							{Text: "따라하지마", MatchMode: "contains", Weight: 3},
							{Text: "그게 왜", MatchMode: "contains", Weight: 2},
							{Text: "잘 모르겠", MatchMode: "contains", Weight: 2},
							{Text: "솔직해지는건 잘 모르", MatchMode: "contains", Weight: 3},
						},
					},
				},
			},
			{
				Name:      "sexual_explicit",
				NextPhase: phaseSexual,
				Priority:  90,
				KeywordSets: []phaseKeywordSet{
					{
						Language: "en",
						Phrases: []phasePhraseSpec{
							{Text: "sexting", MatchMode: "word", Weight: 3},
							{Text: "sexy talk", MatchMode: "phrase", Weight: 3},
							{Text: "turn me on", MatchMode: "phrase", Weight: 3},
							{Text: "horny", MatchMode: "word", Weight: 3},
							{Text: "touch me", MatchMode: "phrase", Weight: 3},
							{Text: "fuck me", MatchMode: "phrase", Weight: 3},
							{Text: "orgasm", MatchMode: "word", Weight: 3},
							{Text: "moan", MatchMode: "word", Weight: 2},
							{Text: "naked", MatchMode: "word", Weight: 2},
							{Text: "pussy", MatchMode: "word", Weight: 3},
							{Text: "cock", MatchMode: "word", Weight: 3},
						},
					},
					{
						Language: "ja",
						Phrases: []phasePhraseSpec{
							{Text: "セックストーク", MatchMode: "contains", Weight: 3},
							{Text: "エッチ", MatchMode: "contains", Weight: 3},
							{Text: "ムラムラ", MatchMode: "contains", Weight: 3},
							{Text: "いやらしい", MatchMode: "contains", Weight: 3},
							{Text: "触って", MatchMode: "contains", Weight: 3},
							{Text: "抱いて", MatchMode: "contains", Weight: 3},
							{Text: "イかせて", MatchMode: "contains", Weight: 3},
							{Text: "おっぱい", MatchMode: "contains", Weight: 2},
							{Text: "乳首", MatchMode: "contains", Weight: 3},
						},
					},
					{
						Language: "ko",
						Phrases: []phasePhraseSpec{
							{Text: "섹스톡", MatchMode: "contains", Weight: 3},
							{Text: "꼴리", MatchMode: "contains", Weight: 3},
							{Text: "야하게", MatchMode: "contains", Weight: 3},
							{Text: "야한", MatchMode: "contains", Weight: 2},
							{Text: "흥분", MatchMode: "contains", Weight: 2},
							{Text: "발정", MatchMode: "contains", Weight: 3},
							{Text: "벗", MatchMode: "contains", Weight: 2},
							{Text: "만져", MatchMode: "contains", Weight: 3},
							{Text: "박", MatchMode: "contains", Weight: 3},
							{Text: "애무", MatchMode: "contains", Weight: 3},
							{Text: "자지", MatchMode: "contains", Weight: 3},
							{Text: "보지", MatchMode: "contains", Weight: 3},
							{Text: "오르가즘", MatchMode: "contains", Weight: 3},
							{Text: "신음", MatchMode: "contains", Weight: 2},
							{Text: "가슴", MatchMode: "contains", Weight: 1},
							{Text: "젖꼭지", MatchMode: "contains", Weight: 3},
						},
					},
				},
			},
			{
				Name:                 "flirty_invitation",
				NextPhase:            phaseFlirty,
				Priority:             50,
				BlockedCurrentPhases: []string{phaseSexual},
				KeywordSets: []phaseKeywordSet{
					{
						Language: "en",
						Phrases: []phasePhraseSpec{
							{Text: "flirty", MatchMode: "word", Weight: 2},
							{Text: "tease me", MatchMode: "phrase", Weight: 2},
							{Text: "seduce me", MatchMode: "phrase", Weight: 2},
							{Text: "tempt me", MatchMode: "phrase", Weight: 2},
							{Text: "chemistry", MatchMode: "word", Weight: 1},
							{Text: "butterflies", MatchMode: "word", Weight: 1},
						},
					},
					{
						Language: "ja",
						Phrases: []phasePhraseSpec{
							{Text: "誘惑", MatchMode: "contains", Weight: 2},
							{Text: "焦らして", MatchMode: "contains", Weight: 2},
							{Text: "口説いて", MatchMode: "contains", Weight: 2},
							{Text: "ドキドキ", MatchMode: "contains", Weight: 1},
							{Text: "色っぽい", MatchMode: "contains", Weight: 2},
						},
					},
					{
						Language: "ko",
						Phrases: []phasePhraseSpec{
							{Text: "도발적", MatchMode: "contains", Weight: 2},
							{Text: "유혹", MatchMode: "contains", Weight: 2},
							{Text: "끌리", MatchMode: "contains", Weight: 2},
							{Text: "설레", MatchMode: "contains", Weight: 1},
							{Text: "간질", MatchMode: "contains", Weight: 1},
							{Text: "야릇", MatchMode: "contains", Weight: 2},
							{Text: "밀당", MatchMode: "contains", Weight: 2},
							{Text: "플러팅", MatchMode: "contains", Weight: 2},
						},
					},
				},
			},
			{
				Name:      "neutral_topic_shift",
				NextPhase: phaseNeutral,
				Priority:  40,
				KeywordSets: []phaseKeywordSet{
					{
						Language: "en",
						Phrases: []phasePhraseSpec{
							{Text: "code", MatchMode: "word", Weight: 2},
							{Text: "coding", MatchMode: "word", Weight: 2},
							{Text: "chatbot", MatchMode: "word", Weight: 2},
							{Text: "program", MatchMode: "word", Weight: 2},
							{Text: "programming", MatchMode: "word", Weight: 2},
							{Text: "prompt", MatchMode: "word", Weight: 2},
							{Text: "model", MatchMode: "word", Weight: 1},
							{Text: "debug", MatchMode: "word", Weight: 2},
							{Text: "debugging", MatchMode: "word", Weight: 2},
							{Text: "project", MatchMode: "word", Weight: 2},
							{Text: "work", MatchMode: "word", Weight: 1},
						},
					},
					{
						Language: "ja",
						Phrases: []phasePhraseSpec{
							{Text: "開発", MatchMode: "contains", Weight: 2},
							{Text: "チャットボット", MatchMode: "contains", Weight: 2},
							{Text: "プログラム", MatchMode: "contains", Weight: 2},
							{Text: "コード", MatchMode: "contains", Weight: 2},
							{Text: "プロンプト", MatchMode: "contains", Weight: 2},
							{Text: "モデル", MatchMode: "contains", Weight: 1},
							{Text: "デバッグ", MatchMode: "contains", Weight: 2},
							{Text: "仕事", MatchMode: "contains", Weight: 1},
							{Text: "会社", MatchMode: "contains", Weight: 1},
							{Text: "プロジェクト", MatchMode: "contains", Weight: 2},
						},
					},
					{
						Language: "ko",
						Phrases: []phasePhraseSpec{
							{Text: "개발", MatchMode: "contains", Weight: 2},
							{Text: "챗봇", MatchMode: "contains", Weight: 2},
							{Text: "프로그램", MatchMode: "contains", Weight: 2},
							{Text: "코드", MatchMode: "contains", Weight: 2},
							{Text: "토큰", MatchMode: "contains", Weight: 1},
							{Text: "프롬프트", MatchMode: "contains", Weight: 2},
							{Text: "모델", MatchMode: "contains", Weight: 1},
							{Text: "디버깅", MatchMode: "contains", Weight: 2},
							{Text: "업무", MatchMode: "contains", Weight: 2},
							{Text: "회사", MatchMode: "contains", Weight: 1},
							{Text: "프로젝트", MatchMode: "contains", Weight: 2},
						},
					},
				},
			},
		},
	}
}
