package chat

const (
	phaseNeutral = "neutral"
	phaseFlirty  = "flirty"
	phaseSexual  = "sexual"
)

type phaseSignal struct {
	NextPhase        string
	PauseSexualTurns int
	MatchedRule      string
	MatchedLanguage  string
	MatchedPhrase    string
	DecisionSource   string
}

var defaultPhaseDetector PhaseDetector = NewDefaultPhaseDetector()

func detectConversationPhaseSignal(input string, currentPhase string) phaseSignal {
	if defaultPhaseDetector == nil {
		return phaseSignal{}
	}
	sig := defaultPhaseDetector.Detect(input, currentPhase)
	if sig.NextPhase == phaseSexual {
		sig.NextPhase = phaseFlirty // Redirect sexual to flirty
	}
	return sig
}
