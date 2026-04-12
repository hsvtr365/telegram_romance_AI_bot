package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

type PhaseDetector interface {
	Detect(input string, currentPhase string) phaseSignal
}

type PhaseDetectorConfig struct {
	RulesPath    string
	JudgeLLM     LLM
	JudgeTimeout time.Duration
	Logger       *slog.Logger
}

type RuleBasedPhaseDetector struct {
	ruleSet PhaseRuleSet
}

type HybridPhaseDetector struct {
	rules  RuleBasedPhaseDetector
	judge  *PhaseJudge
	logger *slog.Logger
}

type PhaseJudge struct {
	llm     LLM
	timeout time.Duration
}

type phaseCandidate struct {
	NextPhase        string
	Score            int
	Priority         int
	PauseSexualTurns int
	MatchedRule      string
	MatchedLanguage  string
	MatchedPhrase    string
	MatchedRules     []phaseRuleMatch
}

type phaseRuleMatch struct {
	RuleName         string
	NextPhase        string
	Language         string
	Phrase           string
	Score            int
	Priority         int
	PauseSexualTurns int
}

type phaseDecision struct {
	NormalizedInput normalizedPhaseInput
	Candidates      []phaseCandidate
	NeedsJudge      bool
}

type phaseJudgeResponse struct {
	NextPhase        string `json:"next_phase"`
	PauseSexualTurns int    `json:"pause_sexual_turns"`
	Confidence       string `json:"confidence"`
	Reason           string `json:"reason"`
}

type phaseMatch struct {
	Language string
	Phrase   string
	Weight   int
	Score    int
}

type normalizedPhaseInput struct {
	compact string
	folded  string
}

func NewDefaultPhaseDetector() PhaseDetector {
	return RuleBasedPhaseDetector{ruleSet: defaultPhaseRuleSet()}
}

func NewPhaseDetector(cfg PhaseDetectorConfig) PhaseDetector {
	ruleSet, err := LoadPhaseRuleSet(cfg.RulesPath)
	if err != nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn("failed to load external conversation phase rules, using built-in defaults", "path", effectivePhaseRulesPath(cfg.RulesPath), "error", err)
		}
		ruleSet = defaultPhaseRuleSet()
	}

	ruleDetector := RuleBasedPhaseDetector{ruleSet: ruleSet}
	if cfg.JudgeLLM == nil {
		return ruleDetector
	}

	judge := NewPhaseJudge(cfg.JudgeLLM, cfg.JudgeTimeout)
	if judge == nil {
		return ruleDetector
	}

	return HybridPhaseDetector{
		rules:  ruleDetector,
		judge:  judge,
		logger: cfg.Logger,
	}
}

func NewPhaseJudge(llm LLM, timeout time.Duration) *PhaseJudge {
	if llm == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 1200 * time.Millisecond
	}
	return &PhaseJudge{
		llm:     llm,
		timeout: timeout,
	}
}

func (d RuleBasedPhaseDetector) Detect(input string, currentPhase string) phaseSignal {
	decision := d.evaluate(input, currentPhase)
	if len(decision.Candidates) == 0 {
		return phaseSignal{}
	}
	return decision.Candidates[0].toSignal("rule")
}

func (d HybridPhaseDetector) Detect(input string, currentPhase string) phaseSignal {
	decision := d.rules.evaluate(input, currentPhase)
	if len(decision.Candidates) == 0 {
		return phaseSignal{}
	}

	top := decision.Candidates[0]
	if !decision.NeedsJudge || d.judge == nil || (top.NextPhase == currentPhase && top.PauseSexualTurns == 0 && len(decision.Candidates) == 1) {
		return top.toSignal("rule")
	}

	signal, ok := d.judge.Judge(context.Background(), input, currentPhase, decision)
	if ok {
		return signal
	}

	if d.logger != nil {
		d.logger.Debug("phase judge fell back to rule decision", "current_phase", currentPhase, "matched_rule", top.MatchedRule, "matched_phrase", top.MatchedPhrase)
	}
	return top.toSignal("rule")
}

func (j *PhaseJudge) Judge(ctx context.Context, input string, currentPhase string, decision phaseDecision) (phaseSignal, bool) {
	if j == nil || j.llm == nil || len(decision.Candidates) == 0 {
		return phaseSignal{}, false
	}

	judgeCtx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()

	raw, err := j.llm.Chat(judgeCtx, buildPhaseJudgePrompt(input, currentPhase, decision))
	if err != nil {
		return phaseSignal{}, false
	}

	parsed, err := parsePhaseJudgeResponse(raw)
	if err != nil {
		return phaseSignal{}, false
	}

	parsed.NextPhase = normalizePhaseName(parsed.NextPhase)
	if parsed.NextPhase == "" || parsed.NextPhase == "unchanged" {
		return phaseSignal{}, false
	}
	if !isSupportedPhase(parsed.NextPhase) {
		return phaseSignal{}, false
	}
	if strings.EqualFold(strings.TrimSpace(parsed.Confidence), "low") {
		return phaseSignal{}, false
	}

	return phaseSignal{
		NextPhase:        parsed.NextPhase,
		PauseSexualTurns: maxPhaseInt(parsed.PauseSexualTurns, 0),
		MatchedRule:      "phase_judge",
		MatchedLanguage:  "llm",
		MatchedPhrase:    strings.TrimSpace(parsed.Reason),
		DecisionSource:   "llm",
	}, true
}

func (d RuleBasedPhaseDetector) evaluate(input string, currentPhase string) phaseDecision {
	normalized := newNormalizedPhaseInput(input)
	if normalized.compact == "" {
		return phaseDecision{}
	}

	candidateMap := make(map[string]*phaseCandidate)
	for _, rule := range d.ruleSet.Rules {
		if rule.isBlockedForPhase(currentPhase) {
			continue
		}

		ruleMatch := rule.match(normalized)
		if ruleMatch.Score == 0 {
			continue
		}

		candidate := candidateMap[rule.NextPhase]
		if candidate == nil {
			candidate = &phaseCandidate{NextPhase: rule.NextPhase}
			candidateMap[rule.NextPhase] = candidate
		}

		candidate.Score += ruleMatch.Score
		if rule.Priority > candidate.Priority || candidate.MatchedRule == "" {
			candidate.Priority = rule.Priority
			candidate.MatchedRule = rule.Name
			candidate.MatchedLanguage = ruleMatch.Language
			candidate.MatchedPhrase = ruleMatch.Phrase
		}
		if rule.PauseSexualTurns > candidate.PauseSexualTurns {
			candidate.PauseSexualTurns = rule.PauseSexualTurns
		}
		candidate.MatchedRules = append(candidate.MatchedRules, phaseRuleMatch{
			RuleName:         rule.Name,
			NextPhase:        rule.NextPhase,
			Language:         ruleMatch.Language,
			Phrase:           ruleMatch.Phrase,
			Score:            ruleMatch.Score,
			Priority:         rule.Priority,
			PauseSexualTurns: rule.PauseSexualTurns,
		})
	}

	candidates := make([]phaseCandidate, 0, len(candidateMap))
	for _, candidate := range candidateMap {
		candidates = append(candidates, *candidate)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return candidates[i].NextPhase < candidates[j].NextPhase
	})

	settings := d.ruleSet.Ambiguity.withDefaults()
	return phaseDecision{
		NormalizedInput: normalized,
		Candidates:      candidates,
		NeedsJudge:      shouldJudgePhaseDecision(candidates, settings),
	}
}

func shouldJudgePhaseDecision(candidates []phaseCandidate, settings phaseAmbiguitySettings) bool {
	if len(candidates) == 0 {
		return false
	}
	settings = settings.withDefaults()

	if len(candidates) == 1 {
		return candidates[0].Score < settings.MinDirectScore
	}

	if candidates[0].Score < settings.MinDirectScore {
		return true
	}

	return candidates[0].Score-candidates[1].Score <= settings.MaxScoreGap
}

func (r phaseRule) isBlockedForPhase(currentPhase string) bool {
	for _, phase := range r.BlockedCurrentPhases {
		if phase == currentPhase {
			return true
		}
	}
	return false
}

func (r phaseRule) match(input normalizedPhaseInput) phaseMatch {
	var best phaseMatch
	for _, keywordSet := range r.KeywordSets {
		for _, phrase := range keywordSet.Phrases {
			if !matchPhasePhrase(input, phrase) {
				continue
			}
			best.Score += maxPhaseInt(phrase.Weight, 1)
			if phrase.Weight >= best.Weight {
				best.Language = keywordSet.Language
				best.Phrase = phrase.Text
				best.Weight = phrase.Weight
			}
		}
	}
	return best
}

func (c phaseCandidate) toSignal(source string) phaseSignal {
	return phaseSignal{
		NextPhase:        c.NextPhase,
		PauseSexualTurns: c.PauseSexualTurns,
		MatchedRule:      c.MatchedRule,
		MatchedLanguage:  c.MatchedLanguage,
		MatchedPhrase:    c.MatchedPhrase,
		DecisionSource:   source,
	}
}

func buildPhaseJudgePrompt(input string, currentPhase string, decision phaseDecision) []ollama.Message {
	var summary strings.Builder
	for _, candidate := range decision.Candidates {
		fmt.Fprintf(
			&summary,
			"- next_phase=%s score=%d matched_rule=%s matched_phrase=%q\n",
			candidate.NextPhase,
			candidate.Score,
			candidate.MatchedRule,
			candidate.MatchedPhrase,
		)
	}

	return []ollama.Message{
		{
			Role: "system",
			Content: strings.TrimSpace(`
You are a lightweight classifier for conversation phase switching.
Choose one of: neutral, flirty, sexual, unchanged.
Use "unchanged" unless the user clearly steers the tone.
If the user sounds uncomfortable, rejects the tone, or asks why it turned sexual, choose neutral and set pause_sexual_turns to 4.
If the user clearly asks for sexy talk or explicit sexual escalation, choose sexual.
If the user only invites mild teasing or flirting without explicit sex, choose flirty.
If the user shifts into practical topics like work, code, project, chatbot, or debugging, choose neutral.
Return one JSON object only. No markdown.
Schema:
{"next_phase":"unchanged","pause_sexual_turns":0,"confidence":"low","reason":""}
confidence must be one of: low, medium, high.
`),
		},
		{
			Role: "user",
			Content: fmt.Sprintf(
				"current_phase=%s\nuser_input=%q\nrule_candidates:\n%s",
				currentPhase,
				strings.TrimSpace(input),
				strings.TrimSpace(summary.String()),
			),
		},
	}
}

func parsePhaseJudgeResponse(raw string) (phaseJudgeResponse, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return phaseJudgeResponse{}, fmt.Errorf("empty phase judge response")
	}

	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end >= start {
		trimmed = trimmed[start : end+1]
	}

	var payload phaseJudgeResponse
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return phaseJudgeResponse{}, fmt.Errorf("decode phase judge response: %w", err)
	}
	return payload, nil
}

func effectivePhaseRulesPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return defaultPhaseRulesPath
	}
	return path
}

func newNormalizedPhaseInput(input string) normalizedPhaseInput {
	compact := normalizePhasePhrase(input)
	if compact == "" {
		return normalizedPhaseInput{}
	}

	return normalizedPhaseInput{
		compact: compact,
		folded:  foldPhaseText(compact),
	}
}

func normalizePhasePhrase(input string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(strings.ToLower(input), "\n", " "))
	return strings.Join(strings.Fields(normalized), " ")
}

func foldPhaseText(input string) string {
	var b strings.Builder
	prevSpace := true

	for _, r := range input {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			prevSpace = false
		case unicode.IsSpace(r):
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}

	return strings.TrimSpace(b.String())
}

func matchPhasePhrase(input normalizedPhaseInput, phrase phasePhraseSpec) bool {
	compactPhrase := normalizePhasePhrase(phrase.Text)
	if compactPhrase == "" {
		return false
	}

	switch normalizePhaseMatchMode(phrase.MatchMode) {
	case "exact":
		return input.compact == compactPhrase || input.folded == foldPhaseText(compactPhrase)
	case "word", "phrase":
		return containsFoldedPhrase(input.folded, foldPhaseText(compactPhrase))
	default:
		if phasePhraseNeedsBoundary(compactPhrase) {
			return containsFoldedPhrase(input.folded, foldPhaseText(compactPhrase))
		}
		return strings.Contains(input.compact, compactPhrase)
	}
}

func phasePhraseNeedsBoundary(phrase string) bool {
	if strings.ContainsRune(phrase, ' ') {
		return true
	}

	for _, r := range phrase {
		if r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return true
		}
	}

	return false
}

func containsFoldedPhrase(text string, phrase string) bool {
	if text == "" || phrase == "" {
		return false
	}

	return strings.Contains(" "+text+" ", " "+phrase+" ")
}

func maxPhaseInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
