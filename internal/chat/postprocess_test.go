package chat

import (
	"reflect"
	"testing"
)

func TestSplitReplyForTelegram_AttachesEmojiToPreviousSentence(t *testing.T) {
	got := SplitReplyForTelegram("오늘 왜 이렇게 늦었어요? 😘 보고 싶었잖아요.")
	want := []string{
		"오늘 왜 이렇게 늦었어요? 😘",
		"보고 싶었잖아요.",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected split result\nwant=%v\ngot=%v", want, got)
	}
}

func TestSplitReplyForTelegram_MergesShortSentenceWithNext(t *testing.T) {
	got := SplitReplyForTelegram("왜요? 보고 싶었잖아요. 빨리 말해요.")
	want := []string{
		"왜요? 보고 싶었잖아요.",
		"빨리 말해요.",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected split result\nwant=%v\ngot=%v", want, got)
	}
}

func TestSplitReplyForTelegram_ChainsShortSentences(t *testing.T) {
	got := SplitReplyForTelegram("응. 왜? 몰라요. 그래도 와요.")
	want := []string{
		"응. 왜? 몰라요. 그래도 와요.",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected split result\nwant=%v\ngot=%v", want, got)
	}
}

func TestSanitizeByConversationPhase_PreservesText(t *testing.T) {
	input := "일 얼른 끝내놓고 기다리고 있어.\n이따 봐."
	got := sanitizeByConversationPhase(input, phaseNeutral)
	if got != input {
		t.Fatalf("expected phase sanitizer to preserve text, got %q", got)
	}
}
