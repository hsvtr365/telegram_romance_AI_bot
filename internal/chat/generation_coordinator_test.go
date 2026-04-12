package chat

import (
	"context"
	"testing"
)

func TestGenerationCoordinator_RequestSafetyRestateBeforeSend(t *testing.T) {
	coordinator := NewGenerationCoordinator()
	handle, _ := coordinator.Begin(context.Background(), 100, 200, 300, 1, 0)

	if !coordinator.RequestSafetyRestate(100, 200, 2) {
		t.Fatalf("expected safety restate request to succeed")
	}

	decision := coordinator.BeforeSend(100, handle.GenerationID)
	if !decision.Regenerate || decision.Reason != interruptReasonSafetyRestate {
		t.Fatalf("expected regenerate decision, got %+v", decision)
	}
}

func TestGenerationCoordinator_DoesNotRestateAfterFirstPart(t *testing.T) {
	coordinator := NewGenerationCoordinator()
	handle, _ := coordinator.Begin(context.Background(), 100, 200, 300, 1, 0)
	coordinator.MarkPartSent(100, handle.GenerationID)

	if coordinator.RequestSafetyRestate(100, 200, 2) {
		t.Fatalf("did not expect safety restate after first part")
	}

	decision := coordinator.BeforeSend(100, handle.GenerationID)
	if !decision.Allow {
		t.Fatalf("expected send to remain allowed, got %+v", decision)
	}
}

func TestGenerationCoordinator_CancelActiveMarksNewInput(t *testing.T) {
	coordinator := NewGenerationCoordinator()
	handle, _ := coordinator.Begin(context.Background(), 100, 200, 300, 1, 0)

	coordinator.CancelActive(100, interruptReasonNewInput)

	if got := coordinator.InterruptReason(100, handle.GenerationID); got != interruptReasonNewInput {
		t.Fatalf("expected new_input interrupt, got %q", got)
	}
}
