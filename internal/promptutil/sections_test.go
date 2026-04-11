package promptutil

import (
	"strings"
	"testing"
)

func TestWriteSectionAndRawBlock_FormatConsistently(t *testing.T) {
	var builder strings.Builder

	WriteSection(&builder, "Alpha", "  one  ")
	WriteRawBlock(&builder, "  [Holiday Context]\nkeep this  ")

	got := builder.String()
	if !strings.Contains(got, "[Alpha]\none\n\n") {
		t.Fatalf("expected titled section formatting, got %q", got)
	}
	if !strings.Contains(got, "[Holiday Context]\nkeep this\n\n") {
		t.Fatalf("expected raw block formatting, got %q", got)
	}
}

func TestWriteConversation_SkipsBlankMessages(t *testing.T) {
	var builder strings.Builder

	WriteConversation(&builder, "Recent Conversation", []MessageLine{
		{Role: "user", Content: " 안녕 "},
		{Role: "", Content: "skip"},
		{Role: "assistant", Content: " "},
	})

	got := builder.String()
	if !strings.Contains(got, "[Recent Conversation]\nuser: 안녕\n\n") {
		t.Fatalf("expected single normalized conversation line, got %q", got)
	}
}
