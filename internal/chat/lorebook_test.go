package chat

import (
	"testing"
)

func TestLorebook_Match(t *testing.T) {
	lb := &Lorebook{
		Entries: []LoreEntry{
			{
				Keys:    []string{"민트초코", "민초"},
				Content: "태규는 민트초코를 싫어하며, 치약 맛이 난다고 생각합니다.",
			},
			{
				Keys:    []string{"취향"},
				Content: "태규는 상대의 눈빛과 분위기에 강렬하게 끌립니다.",
			},
		},
	}

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "난 민트초코가 세상에서 제일 좋아!",
			expected: "태규는 민트초코를 싫어하며, 치약 맛이 난다고 생각합니다.",
		},
		{
			input:    "오늘 민초 먹을래?",
			expected: "태규는 민트초코를 싫어하며, 치약 맛이 난다고 생각합니다.",
		},
		{
			input:    "너 취향이 어떻게 돼?",
			expected: "태규는 상대의 눈빛과 분위기에 강렬하게 끌립니다.",
		},
		{
			input:    "민트초코에 특별한 취향이 있니?",
			expected: "태규는 민트초코를 싫어하며, 치약 맛이 난다고 생각합니다.\n태규는 상대의 눈빛과 분위기에 강렬하게 끌립니다.",
		},
		{
			input:    "안녕하세요 반가워요",
			expected: "",
		},
	}

	for _, tt := range tests {
		got := lb.Match(tt.input)
		if got != tt.expected {
			t.Errorf("Match(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}
