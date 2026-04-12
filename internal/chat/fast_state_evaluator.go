package chat

import "log/slog"

const fastStateReviewMinChars = 6

type FastStateResult struct {
	Signal           phaseSignal
	NeedsAsyncReview bool
}

type FastStateEvaluator struct {
	detector RuleBasedPhaseDetector
}

func NewFastStateEvaluator(rulesPath string, logger *slog.Logger) *FastStateEvaluator {
	ruleSet, err := LoadPhaseRuleSet(rulesPath)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to load fast state rules, using built-in defaults", "path", effectivePhaseRulesPath(rulesPath), "error", err)
		}
		ruleSet = defaultPhaseRuleSet()
	}

	return &FastStateEvaluator{
		detector: RuleBasedPhaseDetector{ruleSet: ruleSet},
	}
}

func (e *FastStateEvaluator) Evaluate(input string, currentTonePhase string) FastStateResult {
	if e == nil {
		return FastStateResult{}
	}

	decision := e.detector.evaluate(input, currentTonePhase)
	result := FastStateResult{
		NeedsAsyncReview: len([]rune(decision.NormalizedInput.compact)) >= fastStateReviewMinChars,
	}
	if len(decision.Candidates) == 0 {
		return result
	}

	top := decision.Candidates[0]
	result.Signal = top.toSignal("rule")
	return result
}
