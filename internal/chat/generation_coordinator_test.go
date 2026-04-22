package chat

import (
	"context"
	"testing"
)

func TestGenerationCoordinator_RequestSafetyRestateBeforeSend(t *testing.T) {
	coordinator := NewGenerationCoordinator()
	chatKey := "bot:telegram:100"
	handle, _ := coordinator.Begin(context.Background(), chatKey, 200, 300, 1, 0)

	if !coordinator.RequestSafetyRestate(chatKey, 200, 2) {
		t.Fatalf("expected safety restate request to succeed")
	}

	decision := coordinator.BeforeSend(chatKey, handle.GenerationID)
	if !decision.Regenerate || decision.Reason != interruptReasonSafetyRestate {
		t.Fatalf("expected regenerate decision, got %+v", decision)
	}
}

func TestGenerationCoordinator_DoesNotRestateAfterFirstPart(t *testing.T) {
	coordinator := NewGenerationCoordinator()
	chatKey := "bot:telegram:100"
	handle, _ := coordinator.Begin(context.Background(), chatKey, 200, 300, 1, 0)
	coordinator.MarkPartSent(chatKey, handle.GenerationID)

	if coordinator.RequestSafetyRestate(chatKey, 200, 2) {
		t.Fatalf("did not expect safety restate after first part")
	}

	decision := coordinator.BeforeSend(chatKey, handle.GenerationID)
	if !decision.Allow {
		t.Fatalf("expected send to remain allowed, got %+v", decision)
	}
}

func TestGenerationCoordinator_CancelActiveMarksNewInput(t *testing.T) {
	coordinator := NewGenerationCoordinator()
	chatKey := "bot:telegram:100"
	handle, _ := coordinator.Begin(context.Background(), chatKey, 200, 300, 1, 0)

	coordinator.CancelActive(chatKey, interruptReasonNewInput)

	if got := coordinator.InterruptReason(chatKey, handle.GenerationID); got != interruptReasonNewInput {
		t.Fatalf("expected new_input interrupt, got %q", got)
	}
}
