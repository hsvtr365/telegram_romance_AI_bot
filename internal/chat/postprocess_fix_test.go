package chat

import (
	"testing"
)

func TestPostProcess(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Remove Meta Commentary",
			input: "제시해주신 대화의 흐름과 분위기를 이어받아 작성해 드립니다.\n**AI:** \"진짜라니까...\"\n어떻게 할까?",
			expected: "\"진짜라니까...\"\n어떻게 할까?",
		},
		{
			name: "Remove Role Markers",
			input: "서태규: 안녕\n**서태규:** 뭐해?\n**AI:** 진짜?",
			expected: "안녕\n뭐해?\n진짜?",
		},
		{
			name: "Skip Long OK Phrases",
			input: "알겠습니다. 사용자의 요청에 따라 페르소나를 유지하여 답변을 작성하겠습니다.\n진짜 개쩔었는데 씨발",
			expected: "진짜 개쩔었는데 씨발",
		},
		{
			name: "Keep Natural OK",
			input: "알겠습니다. 이따가 봐.",
			expected: "알겠습니다. 이따가 봐.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PostProcess(tt.input, 0)
			if got != tt.expected {
				t.Errorf("PostProcess(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
