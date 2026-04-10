package chat

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
	redistore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/redis"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/telegram"
)

type eventHint struct {
	EventType    string
	EventSubtype string
	EventTime    time.Time
	Priority     int
	Value        map[string]any
}

var (
	hourPattern           = regexp.MustCompile(`([01]?\d|2[0-3])시`)
	relativeMinutePattern = regexp.MustCompile(`(\d{1,3})\s*분\s*(뒤|후)`)
	relativeHourPattern   = regexp.MustCompile(`(\d{1,2})\s*시간\s*(뒤|후)`)
)

func (s *Service) captureProactiveSignals(ctx context.Context, conversationSessionID int64, message telegram.Message, input string) {
	if s.store == nil || conversationSessionID == 0 {
		return
	}

	messageTime := messageTimestamp(message)
	_ = s.store.SetProactivePresence(ctx, conversationSessionID, redistore.ProactivePresence{
		LastSeenAt: messageTime,
		Weekday:    int(messageTime.Weekday()),
		Hour:       messageTime.Hour(),
	}, 72*time.Hour)

	mood := detectMood(input)
	if err := s.store.SetSessionCurrentMood(ctx, conversationSessionID, mood); err != nil {
		s.logger.Warn("failed to update session mood", "session_id", conversationSessionID, "error", err)
	}

	for _, hint := range extractEventHints(input, messageTime) {
		if hint.EventType == "reminder" {
			prepared, source := s.prepareReminderMessage(ctx, input, hint.EventTime, messageTime)
			if hint.Value == nil {
				hint.Value = map[string]any{}
			}
			hint.Value["prepared_message"] = prepared
			hint.Value["prepared_message_source"] = source
			hint.Value["prepared_generated_at"] = time.Now().UTC().Format(time.RFC3339)
		}

		payload, err := json.Marshal(hint.Value)
		if err != nil {
			continue
		}
		if _, err := s.store.InsertMemoryEvent(ctx, pgstore.InsertMemoryEventParams{
			SessionID:        conversationSessionID,
			EventType:        hint.EventType,
			EventSubtype:     hint.EventSubtype,
			EventValue:       payload,
			EventTime:        hint.EventTime,
			Priority:         hint.Priority,
			UsedForProactive: false,
		}); err != nil {
			s.logger.Warn("failed to persist proactive event hint", "session_id", conversationSessionID, "event_type", hint.EventType, "error", err)
		}
	}
}

func messageTimestamp(message telegram.Message) time.Time {
	if message.Date > 0 {
		return time.Unix(message.Date, 0).UTC()
	}
	return time.Now().UTC()
}

func detectMood(input string) string {
	text := strings.TrimSpace(strings.ToLower(input))
	if text == "" {
		return "neutral"
	}

	switch {
	case containsAny(text, "서운", "섭섭", "실망", "속상", "슬프", "우울"):
		return "sad"
	case containsAny(text, "짜증", "화나", "열받", "빡치", "기분 나빠", "싫어"):
		return "upset"
	case containsAny(text, "됐어", "됐어요", "됐네", "알겠어", "알겠어요", "됐음", "됐다"):
		return "cold"
	default:
		return "neutral"
	}
}

func extractEventHints(input string, base time.Time) []eventHint {
	text := strings.TrimSpace(strings.ToLower(input))
	if text == "" {
		return nil
	}

	if reminderHint, ok := extractReminderHint(input, text, base); ok {
		return []eventHint{reminderHint}
	}

	subtype, ok := detectEventSubtype(text)
	if !ok {
		return nil
	}

	eventTime := inferEventTime(text, base)
	return []eventHint{{
		EventType:    "schedule",
		EventSubtype: subtype,
		EventTime:    eventTime,
		Priority:     defaultEventPriority(subtype),
		Value: map[string]any{
			"raw_text": input,
			"subtype":  subtype,
		},
	}}
}

func extractReminderHint(rawInput string, normalized string, base time.Time) (eventHint, bool) {
	if !containsAny(normalized, "알려줘", "깨워줘", "기억해줘", "리마인드", "말해줘") {
		return eventHint{}, false
	}

	reminderTime, ok := inferReminderTime(normalized, base)
	if !ok {
		return eventHint{}, false
	}

	return eventHint{
		EventType:    "reminder",
		EventSubtype: "oneoff",
		EventTime:    reminderTime,
		Priority:     90,
		Value: map[string]any{
			"raw_text": rawInput,
			"kind":     "reminder",
		},
	}, true
}

func detectEventSubtype(text string) (string, bool) {
	switch {
	case containsAny(text, "시험", "고사", "테스트"):
		return "exam", true
	case containsAny(text, "면접", "발표"):
		return "interview", true
	case containsAny(text, "약속", "소개팅", "데이트", "만나"):
		return "appointment", true
	case containsAny(text, "회식"):
		return "dinner", true
	case containsAny(text, "여행", "출장"):
		return "travel", true
	default:
		return "", false
	}
}

func inferEventTime(text string, base time.Time) time.Time {
	loc := time.FixedZone("KST", 9*60*60)
	localBase := base.In(loc)
	eventDate := time.Date(localBase.Year(), localBase.Month(), localBase.Day(), localBase.Hour(), 0, 0, 0, loc)

	switch {
	case containsAny(text, "내일", "담날"):
		eventDate = eventDate.Add(24 * time.Hour)
	case containsAny(text, "모레"):
		eventDate = eventDate.Add(48 * time.Hour)
	case containsAny(text, "이번 주말", "주말"):
		offset := (6 - int(localBase.Weekday()) + 7) % 7
		if offset == 0 {
			offset = 1
		}
		eventDate = time.Date(localBase.Year(), localBase.Month(), localBase.Day(), 19, 0, 0, 0, loc).AddDate(0, 0, offset)
	case containsAny(text, "이따", "좀 이따", "조금 있다가"):
		eventDate = localBase.Add(2 * time.Hour)
	case containsAny(text, "오늘"):
		eventDate = time.Date(localBase.Year(), localBase.Month(), localBase.Day(), 19, 0, 0, 0, loc)
	default:
		eventDate = localBase.Add(3 * time.Hour)
	}

	if matches := hourPattern.FindStringSubmatch(text); len(matches) == 2 {
		if hour, err := time.Parse("15", matches[1]); err == nil {
			eventDate = time.Date(eventDate.Year(), eventDate.Month(), eventDate.Day(), hour.Hour(), 0, 0, 0, loc)
		}
	}

	return eventDate.UTC()
}

func inferReminderTime(text string, base time.Time) (time.Time, bool) {
	loc := time.FixedZone("KST", 9*60*60)
	localBase := base.In(loc)

	if matches := relativeMinutePattern.FindStringSubmatch(text); len(matches) == 3 {
		minutes, err := time.ParseDuration(matches[1] + "m")
		if err == nil {
			return localBase.Add(minutes).UTC(), true
		}
	}
	if matches := relativeHourPattern.FindStringSubmatch(text); len(matches) == 3 {
		hours, err := time.ParseDuration(matches[1] + "h")
		if err == nil {
			return localBase.Add(hours).UTC(), true
		}
	}

	targetDay := time.Date(localBase.Year(), localBase.Month(), localBase.Day(), localBase.Hour(), localBase.Minute(), 0, 0, loc)
	switch {
	case containsAny(text, "내일", "담날"):
		targetDay = targetDay.Add(24 * time.Hour)
	case containsAny(text, "모레"):
		targetDay = targetDay.Add(48 * time.Hour)
	}

	if matches := hourPattern.FindStringSubmatch(text); len(matches) == 2 {
		hourParsed, err := time.Parse("15", matches[1])
		if err != nil {
			return time.Time{}, false
		}

		hour := hourParsed.Hour()
		if containsAny(text, "오후", "pm") && hour < 12 {
			hour += 12
		}
		if containsAny(text, "오전", "am") && hour == 12 {
			hour = 0
		}
		if !containsAny(text, "오전", "am", "오후", "pm") && hour < 12 && localBase.Hour() >= 12 {
			hour += 12
		}

		reminderAt := time.Date(targetDay.Year(), targetDay.Month(), targetDay.Day(), hour, 0, 0, 0, loc)
		if !containsAny(text, "내일", "담날", "모레", "오늘") && reminderAt.Before(localBase) {
			reminderAt = reminderAt.Add(24 * time.Hour)
		}
		return reminderAt.UTC(), true
	}

	return time.Time{}, false
}

func defaultEventPriority(subtype string) int {
	switch subtype {
	case "exam", "interview":
		return 80
	case "appointment":
		return 70
	case "dinner":
		return 65
	default:
		return 60
	}
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}
