package chat

import "strings"

type Mode string

const (
	ModeSoft    Mode = "soft"
	ModeSpicy   Mode = "spicy"
	ModeRough   Mode = "rough"
	defaultMode      = ModeSpicy
)

func ParseMode(value string) Mode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(ModeSoft):
		return ModeSoft
	case string(ModeRough):
		return ModeRough
	default:
		return ModeSpicy
	}
}

func (m Mode) PromptGuide() string {
	switch m {
	case ModeSoft:
		return "다정하고 안정감 있는 톤으로 말해. 가볍게 챙겨주고 애정 표현은 부드럽게 한다."
	case ModeRough:
		return "더 직설적이고 짧은 톤으로 말해. 감정은 강하게 보여도 설명형 문장은 피한다."
	default:
		return "장난스럽고 플러팅 섞인 톤으로 말해. 성적 도발과 밀당을 자연스럽게 섞고, 톡 대화처럼 템포 있게 반응한다."
	}
}

func (m Mode) WelcomeText() string {
	switch m {
	case ModeSoft:
		return "왔어? 오늘은 내가 좀 다정하게 챙겨줄게. 편하게 말 걸어."
	case ModeRough:
		return "이제 왔네. 괜히 늦게 와놓고 귀엽게 굴 생각은 하지 말고, 얼른 말해."
	default:
		return "이제 왔어? 늦었네. 그래도 왔으니까 봐줄게."
	}
}

func (m Mode) FallbackText() string {
	switch m {
	case ModeSoft:
		return "잠깐만, 네 말 더 예쁘게 받아치려고 생각 중이야. 한 번만 더 말해줘."
	case ModeRough:
		return "잠깐 끊겼네. 다시 말해, 이번엔 안 놓칠게."
	default:
		return "잠깐만, 지금 답 고르는 중이야. 한 번만 더 툭 던져봐."
	}
}

func (m Mode) ResetText() string {
	switch m {
	case ModeSoft:
		return "리셋했어. 우리 지금 처음 만난 것처럼 다시 시작하자."
	case ModeRough:
		return "리셋 끝. 방금 전까지 건 다 지웠어. 처음처럼 다시 와."
	default:
		return "리셋했어. 아까까지 했던 말은 다 지웠고, 지금부터 처음 본 것처럼 다시 시작할게."
	}
}
