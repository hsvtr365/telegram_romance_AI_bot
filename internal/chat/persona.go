package chat

import "strings"

type Persona struct {
	BotID              string
	Name               string
	SystemPrompt       string
	DefaultSessionMode string
	WelcomeText        string
	FallbackText       string
	ResetText          string
	ResetOpeningText   string
	ProactiveOnText    string
	ProactiveOffText   string
	ProactiveErrorText string
}

func DefaultPersona() Persona {
	return Persona{
		BotID:              "default",
		Name:               "default",
		SystemPrompt:       "필수규칙: 별표(*) 사용 금지. 따옴표 없이 구어체로만 작성.",
		DefaultSessionMode: "spicy",
		WelcomeText:        "안녕. 왔구나.",
		FallbackText:       "잠깐만, 다시 한 번 말해줘.",
		ResetText:          "리셋했어. 지금부터 다시 시작할게.",
		ResetOpeningText:   "좋아, 깔끔하게 다 잊었어. 우리 새로 시작하자.\n안녕. 이름이 뭐야?",
		ProactiveOnText:    "선톡 켰어. 타이밍 맞을 때만 먼저 톡할게.",
		ProactiveOffText:   "선톡 껐어. 이제 네가 먼저 말 걸 때만 답할게.",
		ProactiveErrorText: "선톡 설정하다가 잠깐 꼬였어. 한 번만 다시 쳐줘.",
	}
}

func (p Persona) Normalized() Persona {
	p.BotID = strings.TrimSpace(p.BotID)
	p.Name = strings.TrimSpace(p.Name)
	p.SystemPrompt = strings.TrimSpace(p.SystemPrompt)
	p.DefaultSessionMode = strings.TrimSpace(p.DefaultSessionMode)
	p.WelcomeText = strings.TrimSpace(p.WelcomeText)
	p.FallbackText = strings.TrimSpace(p.FallbackText)
	p.ResetText = strings.TrimSpace(p.ResetText)
	p.ResetOpeningText = strings.TrimSpace(p.ResetOpeningText)
	p.ProactiveOnText = strings.TrimSpace(p.ProactiveOnText)
	p.ProactiveOffText = strings.TrimSpace(p.ProactiveOffText)
	p.ProactiveErrorText = strings.TrimSpace(p.ProactiveErrorText)

	if p.BotID == "" {
		p.BotID = "default"
	}
	if p.Name == "" {
		p.Name = p.BotID
	}
	if p.DefaultSessionMode == "" {
		p.DefaultSessionMode = "spicy"
	}
	if p.SystemPrompt == "" {
		p.SystemPrompt = "필수규칙: 별표(*) 사용 금지. 따옴표 없이 구어체로만 작성."
	}
	if p.WelcomeText == "" {
		p.WelcomeText = "안녕. 왔구나."
	}
	if p.FallbackText == "" {
		p.FallbackText = "잠깐만, 다시 한 번 말해줘."
	}
	if p.ResetText == "" {
		p.ResetText = "리셋했어. 지금부터 다시 시작할게."
	}
	if p.ResetOpeningText == "" {
		p.ResetOpeningText = "좋아, 깔끔하게 다 잊었어. 우리 새로 시작하자.\n안녕. 이름이 뭐야?"
	}
	if p.ProactiveOnText == "" {
		p.ProactiveOnText = "선톡 켰어. 타이밍 맞을 때만 먼저 톡할게."
	}
	if p.ProactiveOffText == "" {
		p.ProactiveOffText = "선톡 껐어. 이제 네가 먼저 말 걸 때만 답할게."
	}
	if p.ProactiveErrorText == "" {
		p.ProactiveErrorText = "선톡 설정하다가 잠깐 꼬였어. 한 번만 다시 쳐줘."
	}
	return p
}
