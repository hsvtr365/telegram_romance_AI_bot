package proactive

import (
	"fmt"
	"strings"
	"time"
)

func EvaluateEligibility(session SessionSnapshot, candidate TriggerCandidate, now time.Time, profile ProactiveProfile, recentProactives []ProactiveMessageRecord) EligibilityResult {
	reasons := make([]string, 0, 8)

	if candidate.TriggerType == TriggerReminder {
		if hasTriggerRefBeenSent(recentProactives, candidate.TriggerType, candidate.TriggerRefID) {
			reasons = append(reasons, "reminder_already_sent")
			return EligibilityResult{Eligible: false, HardBlock: true, Reasons: reasons}
		}
		return EligibilityResult{Eligible: true, HardBlock: false, Reasons: reasons}
	}

	if !session.ProactiveOptIn {
		reasons = append(reasons, "proactive_opt_in_disabled")
		return EligibilityResult{Eligible: false, HardBlock: true, Reasons: reasons}
	}

	if withinCooldown(session.LastProactiveAt, now, cooldownHoursForTrigger(candidate.TriggerType, profile)) {
		reasons = append(reasons, "trigger_cooldown_active")
		return EligibilityResult{Eligible: false, HardBlock: true, Reasons: reasons}
	}

	if withinGap(session.LastProactiveAt, now, profile.MinGapHours) {
		reasons = append(reasons, "session_min_gap_active")
		return EligibilityResult{Eligible: false, HardBlock: true, Reasons: reasons}
	}

	if reachedSendCap(recentProactives, now, 24*time.Hour, profile.MaxPerDay) {
		reasons = append(reasons, "daily_send_limit_reached")
		return EligibilityResult{Eligible: false, HardBlock: true, Reasons: reasons}
	}

	if reachedSendCap(recentProactives, now, 7*24*time.Hour, profile.MaxPerWeek) {
		reasons = append(reasons, "weekly_send_limit_reached")
		return EligibilityResult{Eligible: false, HardBlock: true, Reasons: reasons}
	}

	if session.ConsecutiveProactiveIgnored >= 2 && candidate.TriggerType != TriggerEventFollowup {
		reasons = append(reasons, "ignored_proactive_too_many_times")
		return EligibilityResult{Eligible: false, HardBlock: true, Reasons: reasons}
	}

	if !session.LastUserMessageAt.IsZero() && recentlyActive(session.LastUserMessageAt, now, 30*time.Minute) {
		reasons = append(reasons, "user_recently_active")
		return EligibilityResult{Eligible: false, HardBlock: false, Reasons: reasons}
	}

	if inQuietHours(now, session.TimezoneName, session.QuietHours, profile.BestTimeWindows) {
		reasons = append(reasons, "quiet_hours_blocked")
		return EligibilityResult{Eligible: false, HardBlock: false, Reasons: reasons}
	}

	if candidate.TriggerType == TriggerReconnect && !looksLikeReconnectWindow(session, now) {
		reasons = append(reasons, "reconnect_window_not_ready")
		return EligibilityResult{Eligible: false, HardBlock: false, Reasons: reasons}
	}

	if candidate.TriggerType == TriggerHabitPing && profile.MaxPerDay <= 0 {
		reasons = append(reasons, "habit_ping_disabled_by_profile")
		return EligibilityResult{Eligible: false, HardBlock: false, Reasons: reasons}
	}

	if candidate.TriggerType == TriggerMoodRepair && strings.EqualFold(session.CurrentMood, "neutral") {
		reasons = append(reasons, "mood_not_disturbed")
		return EligibilityResult{Eligible: false, HardBlock: false, Reasons: reasons}
	}

	return EligibilityResult{Eligible: true, HardBlock: false, Reasons: reasons}
}

func reachedSendCap(records []ProactiveMessageRecord, now time.Time, window time.Duration, limit int) bool {
	if limit <= 0 {
		return false
	}

	count := 0
	for _, record := range records {
		if record.DeliveryStatus != DeliverySent || record.SentAt.IsZero() {
			continue
		}
		if now.Sub(record.SentAt) <= window {
			count++
			if count >= limit {
				return true
			}
		}
	}
	return false
}

func hasTriggerRefBeenSent(records []ProactiveMessageRecord, trigger TriggerType, refID string) bool {
	for _, record := range records {
		if record.DeliveryStatus != DeliverySent {
			continue
		}
		if record.TriggerType == trigger && strings.TrimSpace(record.TriggerRefID) == strings.TrimSpace(refID) {
			return true
		}
	}
	return false
}

func cooldownHoursForTrigger(trigger TriggerType, profile ProactiveProfile) int {
	switch trigger {
	case TriggerReconnect:
		return maxInt(DefaultReconnectCooldownHours, profile.MinGapHours)
	case TriggerEventFollowup:
		return profile.MinGapHours
	case TriggerMoodRepair:
		return maxInt(DefaultMoodRepairCooldownHours, profile.MinGapHours)
	case TriggerHabitPing:
		return maxInt(DefaultHabitPingCooldownHours, profile.MinGapHours)
	default:
		return profile.MinGapHours
	}
}

func withinCooldown(last time.Time, now time.Time, hours int) bool {
	if last.IsZero() || hours <= 0 {
		return false
	}
	return now.Sub(last) < time.Duration(hours)*time.Hour
}

func withinGap(last time.Time, now time.Time, hours int) bool {
	if last.IsZero() || hours <= 0 {
		return false
	}
	return now.Sub(last) < time.Duration(hours)*time.Hour
}

func recentlyActive(last time.Time, now time.Time, window time.Duration) bool {
	if last.IsZero() {
		return false
	}
	return now.Sub(last) < window
}

func looksLikeReconnectWindow(session SessionSnapshot, now time.Time) bool {
	if session.LastUserMessageAt.IsZero() {
		return true
	}
	return now.Sub(session.LastUserMessageAt) >= 48*time.Hour
}

func inQuietHours(now time.Time, timezoneName string, windows []QuietWindow, preferred []TimeWindow) bool {
	loc := locationOrDefault(timezoneName)
	localNow := now.In(loc)

	if len(windows) == 0 && len(preferred) == 0 {
		hour := localNow.Hour()
		return hour < 11 || hour >= 23
	}

	for _, window := range windows {
		if containsWeekday(window.Weekdays, localNow.Weekday()) && inClockRange(localNow, window.Start, window.End) {
			return true
		}
	}

	for _, window := range preferred {
		if containsWeekday(window.Weekdays, localNow.Weekday()) && inClockRange(localNow, window.Start, window.End) {
			return false
		}
	}

	hour := localNow.Hour()
	return hour < 11 || hour >= 23
}

func containsWeekday(weekdays []string, weekday time.Weekday) bool {
	if len(weekdays) == 0 {
		return true
	}

	target := strings.ToLower(weekday.String()[:3])
	for _, item := range weekdays {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}

func inClockRange(t time.Time, start string, end string) bool {
	startHour, startMinute, ok := parseClock(start)
	if !ok {
		return false
	}
	endHour, endMinute, ok := parseClock(end)
	if !ok {
		return false
	}

	nowMinutes := t.Hour()*60 + t.Minute()
	startMinutes := startHour*60 + startMinute
	endMinutes := endHour*60 + endMinute

	if startMinutes <= endMinutes {
		return nowMinutes >= startMinutes && nowMinutes < endMinutes
	}

	return nowMinutes >= startMinutes || nowMinutes < endMinutes
}

func parseClock(value string) (hour int, minute int, ok bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, 0, false
	}

	hour, err := atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, false
	}
	minute, err = atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, false
	}

	return hour, minute, true
}

func locationOrDefault(name string) *time.Location {
	if strings.TrimSpace(name) == "" {
		name = DefaultTimezoneName
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone(DefaultTimezoneName, 9*60*60)
	}
	return loc
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func atoi(value string) (int, error) {
	var result int
	_, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &result)
	return result, err
}
