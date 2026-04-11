package app

import (
	"encoding/json"
	"testing"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/proactive"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

func TestMapProactiveProfile_DecodesJSONAndFallbacks(t *testing.T) {
	profile := model.ProactiveProfile{
		UserID:                7,
		PreferredTypesJSON:    json.RawMessage(`["reconnect"]`),
		DislikedTypesJSON:     json.RawMessage(`["mood_repair"]`),
		BestTimeWindowsJSON:   json.RawMessage(`[{"weekdays":["mon"],"start":"19:00","end":"21:00","score":0.7}]`),
		MaxPerDay:             2,
		MaxPerWeek:            5,
		MinGapHours:           18,
		AvgReplyDelaySec:      120,
		ProactiveSuccessScore: 0.8,
		TimezoneName:          "",
		WeightOverridesJSON:   json.RawMessage(`{"reconnect":1.2}`),
	}

	got := mapProactiveProfile(profile)

	if got.UserID != 7 || got.TimezoneName != proactive.DefaultTimezoneName {
		t.Fatalf("unexpected mapped profile header: %#v", got)
	}
	if len(got.PreferredTypes) != 1 || got.PreferredTypes[0] != "reconnect" {
		t.Fatalf("expected preferred types to decode, got %#v", got.PreferredTypes)
	}
	if len(got.BestTimeWindows) != 1 || got.BestTimeWindows[0].Start != "19:00" {
		t.Fatalf("expected time windows to decode, got %#v", got.BestTimeWindows)
	}
	if got.WeightOverrides["reconnect"] != 1.2 {
		t.Fatalf("expected weight override to decode, got %#v", got.WeightOverrides)
	}
}

func TestMergeProactiveProfilePatch_AppliesOverridesAndKeepsDefaults(t *testing.T) {
	current := model.ProactiveProfile{
		UserID:                11,
		PreferredTypesJSON:    json.RawMessage(`["habit_ping"]`),
		DislikedTypesJSON:     json.RawMessage(`["reconnect"]`),
		BestTimeWindowsJSON:   json.RawMessage(`[{"weekdays":["fri"],"start":"20:00","end":"22:00","score":0.9}]`),
		MaxPerDay:             1,
		MaxPerWeek:            4,
		MinGapHours:           20,
		AvgReplyDelaySec:      300,
		ProactiveSuccessScore: 0.4,
		TimezoneName:          proactive.DefaultTimezoneName,
		WeightOverridesJSON:   json.RawMessage(`{"habit_ping":0.8}`),
	}

	newMaxPerDay := 3
	newTimezone := "Asia/Tokyo"
	params, err := mergeProactiveProfilePatch(11, current, proactive.ProactiveProfilePatch{
		PreferredTypes: []string{"event_followup"},
		MaxPerDay:      &newMaxPerDay,
		TimezoneName:   &newTimezone,
	})
	if err != nil {
		t.Fatalf("mergeProactiveProfilePatch returned error: %v", err)
	}

	if params.UserID != 11 || params.MaxPerDay != 3 || params.MaxPerWeek != 4 {
		t.Fatalf("unexpected scalar merge result: %#v", params)
	}
	if params.TimezoneName != "Asia/Tokyo" {
		t.Fatalf("expected timezone override, got %q", params.TimezoneName)
	}

	var preferred []string
	if err := json.Unmarshal(params.PreferredTypesJSON, &preferred); err != nil {
		t.Fatalf("decode preferred types: %v", err)
	}
	if len(preferred) != 1 || preferred[0] != "event_followup" {
		t.Fatalf("expected overridden preferred types, got %#v", preferred)
	}

	var disliked []string
	if err := json.Unmarshal(params.DislikedTypesJSON, &disliked); err != nil {
		t.Fatalf("decode disliked types: %v", err)
	}
	if len(disliked) != 1 || disliked[0] != "reconnect" {
		t.Fatalf("expected existing disliked types to persist, got %#v", disliked)
	}
}
