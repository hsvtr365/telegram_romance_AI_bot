//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

func main() {
	client := ollama.NewClient("http://127.0.0.1:11434", "gemma4-e2b-uncensored-q8kp:latest")
	messages := []ollama.Message{
		{
			Role: "system",
			Content: `Extract explicit user facts from the provided text, using the conversation history as context.
Return one JSON object only. No markdown.
Do not infer or guess. Use empty string when unclear.
IMPORTANT: Extract ONLY the core value (e.g., just the name "수지", not "수지라고 불러").

Rules for Profile Fields:
- name: The user's name. Extract ONLY the name itself, NO verbs or suffixes like "라고 불러", "이야", "입니다".
- gender: MUST be "남성" or "여성"
- age: Use digits like "25" or "1990년생"

Rules for Traits:
- 'value' MUST be the exact core noun/object. Do NOT include subjects or verbs.
- If the user explicitly states they don't like, avoid, or hate something, use 'avoid'.
- If the user explicitly states they like or enjoy something, use 'like'.
- If the user explicitly retracts a previous statement, denies liking/avoiding, or says they don't care anymore, use 'delete'.
- confidence: high | medium | low

Schema Template (MUST follow exactly):
{"profile":{"name":{"value":"","confidence":"low"},"gender":{"value":"","confidence":"low"},"age":{"value":"","confidence":"low"},"job":{"value":"","confidence":"low"},"current_focus":{"value":"","confidence":"low"},"hobby":{"value":"","confidence":"low"},"location":{"value":"","confidence":"low"},"affiliation":{"value":"","confidence":"low"}},"traits":[{"trait_type":"like","value":"","confidence":"low"}]}`,
		},
		{
			Role:    "user",
			Content: "이전 대화 (태규는 AI 캐릭터입니다):\n태규(AI): 좋아, 깔끔하게 다 잊었어! 우리 새로 시작하는 거다?\n안녕. 이름이 뭐야?\n\n최신 입력: 수지야",
		},
	}

	res, err := client.Chat(context.Background(), messages)
	fmt.Printf("Resp: %s\nErr: %v\n", res, err)
}
