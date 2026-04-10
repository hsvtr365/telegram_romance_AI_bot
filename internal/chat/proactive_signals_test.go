package chat

import (
	"testing"
	"time"
)

func TestExtractEventHints_ReminderAtSpecificHour(t *testing.T) {
	base := time.Date(2026, 4, 9, 15, 20, 0, 0, time.FixedZone("KST", 9*60*60))
	hints := extractEventHints("오늘 6시에 알려줘", base.UTC())
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].EventType != "reminder" {
		t.Fatalf("expected reminder event, got %+v", hints[0])
	}
	local := hints[0].EventTime.In(time.FixedZone("KST", 9*60*60))
	if local.Hour() != 18 || local.Day() != 9 {
		t.Fatalf("expected reminder at 18:00 same day, got %v", local)
	}
}

func TestExtractEventHints_ReminderRelativeMinutes(t *testing.T) {
	base := time.Date(2026, 4, 9, 15, 20, 0, 0, time.FixedZone("KST", 9*60*60))
	hints := extractEventHints("30분 뒤에 알려줘", base.UTC())
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	local := hints[0].EventTime.In(time.FixedZone("KST", 9*60*60))
	if local.Hour() != 15 || local.Minute() != 50 {
		t.Fatalf("expected reminder at 15:50, got %v", local)
	}
}
