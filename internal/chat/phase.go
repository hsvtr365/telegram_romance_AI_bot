package chat

import "strings"

const (
	phaseNeutral = "neutral"
	phaseFlirty  = "flirty"
	phaseSexual  = "sexual"
)

type phaseSignal struct {
	NextPhase        string
	PauseSexualTurns int
}

func detectConversationPhaseSignal(input string, currentPhase string) phaseSignal {
	normalized := normalizeInput(strings.ToLower(input))
	if normalized == "" {
		return phaseSignal{}
	}

	if detectPhaseDeescalation(normalized) {
		return phaseSignal{NextPhase: phaseNeutral, PauseSexualTurns: 4}
	}
	if detectSexualSignal(normalized) {
		return phaseSignal{NextPhase: phaseSexual}
	}
	if detectFlirtySignal(normalized) {
		if currentPhase == phaseSexual {
			return phaseSignal{}
		}
		return phaseSignal{NextPhase: phaseFlirty}
	}

	if detectNeutralTopic(normalized) {
		return phaseSignal{NextPhase: phaseNeutral}
	}

	return phaseSignal{}
}

func detectPhaseDeescalation(input string) bool {
	return hasAnyKeyword(input,
		"부담", "과해", "뜬금", "갑자기 왜", "왜 갑자기", "문맥 이상", "별걸 다", "멍청아", "따라하지마", "그게 왜", "잘 모르겠", "솔직해지는건 잘 모르",
	)
}

func detectSexualSignal(input string) bool {
	return hasAnyKeyword(input,
		"섹스톡", "꼴리", "야하게", "야한", "흥분", "발정", "벗", "만져", "박", "애무", "자지", "보지", "오르가즘", "신음", "가슴", "젖꼭지",
	)
}

func detectFlirtySignal(input string) bool {
	return hasAnyKeyword(input,
		"도발적", "유혹", "끌리", "설레", "간질", "야릇", "밀당", "플러팅",
	)
}

func detectNeutralTopic(input string) bool {
	return hasAnyKeyword(input,
		"개발", "챗봇", "프로그램", "코드", "토큰", "프롬프트", "모델", "디버깅", "일", "업무", "회사", "프로젝트",
	)
}
