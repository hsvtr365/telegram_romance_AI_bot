package promptutil

import (
	"strings"
	"testing"
	"time"
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

func TestWriteConversation_PreservesTimestampsWithoutMerging(t *testing.T) {
	var builder strings.Builder
	first := time.Date(2026, 4, 11, 13, 4, 0, 0, time.UTC)
	second := first.Add(2 * time.Minute)

	WriteConversation(&builder, "Recent Conversation", []MessageLine{
		{Role: "user", Content: "안녕", CreatedAt: first},
		{Role: "user", Content: "뭐 해", CreatedAt: second},
	})

	got := builder.String()
	if !strings.Contains(got, "2026-04-11 22:04 KST user: 안녕") {
		t.Fatalf("expected first timestamped message, got %q", got)
	}
	if !strings.Contains(got, "2026-04-11 22:06 KST user: 뭐 해") {
		t.Fatalf("expected second timestamped message, got %q", got)
	}
	if strings.Contains(got, "안녕\n뭐 해") {
		t.Fatalf("expected timestamped messages to stay separate, got %q", got)
	}
}
