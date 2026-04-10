package chat

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

const reminderPrecomputeTimeout = 1500 * time.Millisecond

func (s *Service) prepareReminderMessage(ctx context.Context, rawInput string, dueAt time.Time, requestedAt time.Time) (string, string) {
	fallback := fallbackReminderMessage(rawInput, dueAt)
	if s == nil || s.reminderLLM == nil {
		return fallback, "fallback"
	}

	genCtx, cancel := context.WithTimeout(ctx, reminderPrecomputeTimeout)
	defer cancel()

	reply, err := s.reminderLLM.Chat(genCtx, buildReminderPrecomputePrompt(rawInput, dueAt, requestedAt, fallback))
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to precompute reminder message", "error", err)
		}
		return fallback, "fallback"
	}

	text := normalizeReminderMessage(reply)
	if text == "" {
		return fallback, "fallback"
	}
	return text, "generated"
}

func buildReminderPrecomputePrompt(rawInput string, dueAt time.Time, requestedAt time.Time, fallback string) []ollama.Message {
	loc := time.FixedZone("KST", 9*60*60)
	dueLocal := dueAt.In(loc)
	requestedLocal := requestedAt.In(loc)

	systemPrompt := strings.TrimSpace(`
역할: 텔레그램 리마인드 문구 생성기.
목표: 사용자가 부탁한 알림 시간을 위한 짧은 한 문장을 만든다.
규칙:
- 반드시 한 문장만 쓴다.
- 12자~36자 안쪽으로 쓴다.
- 질문하지 않는다.
- "보고 싶어서", "잘 다녀왔어", "뭐 해", "먼저 말 걸었어" 같은 일반 선톡 표현은 금지한다.
- 약속한 알림이라는 느낌이 분명해야 한다.
- 설명이나 꾸밈말 없이 바로 말한다.
- 답변에는 문장 하나만 출력한다.
`)

	userPrompt := fmt.Sprintf(
		"사용자 요청: %s\n요청 시각: %s\n알림 시각: %s\nfallback: %s",
		strings.TrimSpace(rawInput),
		requestedLocal.Format("2006-01-02 15:04"),
		dueLocal.Format("2006-01-02 15:04"),
		fallback,
	)

	return []ollama.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
}

func normalizeReminderMessage(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	replacer := strings.NewReplacer(
		"assistant:", "",
		"Assistant:", "",
		"\n", " ",
		"\r", " ",
		"\"", "",
		"“", "",
		"”", "",
	)
	text = strings.TrimSpace(replacer.Replace(text))
	if text == "" {
		return ""
	}

	text = strings.ReplaceAll(text, "?", ".")
	text = strings.ReplaceAll(text, "？", ".")
	for _, sep := range []string{". ", "!", "！"} {
		if idx := strings.Index(text, sep); idx >= 0 {
			text = strings.TrimSpace(text[:idx+1])
			break
		}
	}

	runes := []rune(text)
	if len(runes) > 40 {
		text = strings.TrimSpace(string(runes[:40]))
	}
	return strings.TrimSpace(text)
}

func fallbackReminderMessage(rawInput string, dueAt time.Time) string {
	options := []string{
		"약속한 시간이라 알려주러 왔어.",
		"시간 돼서 바로 톡 남겨.",
		"말해달라고 한 시간이라 왔어.",
	}
	if strings.Contains(rawInput, "깨워") {
		options = []string{
			"약속한 시간이라 깨우러 왔어.",
			"이제 슬슬 일어날 시간이야.",
			"말해달라고 한 시간이라 깨워.",
		}
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(rawInput))
	_, _ = hasher.Write([]byte(dueAt.UTC().Format(time.RFC3339)))
	idx := int(hasher.Sum32() % uint32(len(options)))
	return options[idx]
}
