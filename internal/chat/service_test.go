package chat

import (
	"testing"
	"time"
)

func TestDefaultBestTimeWindows_ReturnsMorningLunchDinner(t *testing.T) {
	windows := defaultBestTimeWindows(time.Date(2026, 4, 10, 9, 0, 0, 0, time.FixedZone("KST", 9*60*60)))
	if len(windows) != 3 {
		t.Fatalf("expected 3 default windows, got %d", len(windows))
	}

	expected := []struct {
		start string
		end   string
	}{
		{start: "08:00", end: "10:00"},
		{start: "12:00", end: "14:00"},
		{start: "19:00", end: "21:30"},
	}

	for i, window := range windows {
		if window["start"] != expected[i].start {
			t.Fatalf("window %d start mismatch: got %v want %s", i, window["start"], expected[i].start)
		}
		if window["end"] != expected[i].end {
			t.Fatalf("window %d end mismatch: got %v want %s", i, window["end"], expected[i].end)
		}
		weekdays, ok := window["weekdays"].([]string)
		if !ok || len(weekdays) != 7 {
			t.Fatalf("window %d weekdays mismatch: got %#v", i, window["weekdays"])
		}
	}
}

