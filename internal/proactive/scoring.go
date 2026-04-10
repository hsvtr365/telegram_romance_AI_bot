package proactive

import (
	"math"
	"strings"
	"time"
)

func ScoreCandidate(session SessionSnapshot, profile ProactiveProfile, candidate TriggerCandidate, recentMessages []ConversationMessage, recentProactives []ProactiveMessageRecord, now time.Time) ScoreResult {
	result := ScoreResult{
		Base:    baseScore(candidate.TriggerType),
		Bonus:   map[string]float64{},
		Penalty: map[string]float64{},
	}

	result.Score = result.Base

	if conversationWithin(recentMessages, now, 3*24*time.Hour) {
		result.Bonus["recent_3d_conversation"] = 12
		result.Score += 12
	}

	if conversationFrequency(recentMessages, now, 7*24*time.Hour) >= 4 {
		result.Bonus["recent_7d_frequency"] = 8
		result.Score += 8
	}

	if candidate.TriggerType == TriggerEventFollowup {
		result.Bonus["event_exists"] = 15
		result.Score += 15
	}

	if strings.TrimSpace(session.CurrentMood) != "" && !strings.EqualFold(session.CurrentMood, "neutral") {
		result.Bonus["unresolved_mood"] = 14
		result.Score += 14
	}

	if matchedBestWindow(now, profile.BestTimeWindows, profile.TimezoneName) {
		result.Bonus["activity_window_match"] = 10
		result.Score += 10
	}

	if session.RelationshipScore >= 70 {
		result.Bonus["relationship_score_high"] = 8
		result.Score += 8
	}

	if profile.ProactiveSuccessScore >= 0.7 {
		result.Bonus["profile_success"] = 4
		result.Score += 4
	}

	if lastProactiveIgnored(recentProactives) {
		result.Penalty["recent_proactive_ignored"] = 12
		result.Score -= 12
	}

	if consecutiveIgnored(session, recentProactives) {
		result.Penalty["consecutive_ignored"] = 18
		result.Score -= 18
	}

	if coldRecentReply(recentMessages, now) {
		result.Penalty["cold_recent_reply"] = 10
		result.Score -= 10
	}

	if lateNight(now, profile.TimezoneName) {
		result.Penalty["late_night"] = 8
		result.Score -= 8
	}

	if recentBotOveruse(recentMessages, now) {
		result.Penalty["bot_overuse"] = 6
		result.Score -= 6
	}

	if sameTriggerRecently(recentProactives, candidate.TriggerType, candidate.TriggerRefID, now) {
		result.Penalty["same_trigger_recent"] = 10
		result.Score -= 10
	}

	if sentRecently(recentProactives, now, 24*time.Hour) {
		result.Penalty["recent_proactive_within_24h"] = 15
		result.Score -= 15
	}

	if profileBias := profileBias(profile, candidate.TriggerType); profileBias != 0 {
		if profileBias > 0 {
			result.Bonus["profile_bias"] = profileBias
			result.Score += profileBias
		} else {
			result.Penalty["profile_bias"] = -profileBias
			result.Score += profileBias
		}
	}

	if value, ok := candidate.ScoreHints["bonus"]; ok {
		result.Bonus["candidate_bonus_hint"] = value
		result.Score += value
	}
	if value, ok := candidate.ScoreHints["penalty"]; ok {
		result.Penalty["candidate_penalty_hint"] = value
		result.Score -= value
	}

	result.Score = clamp(result.Score, 0, 100)
	result.Reasons = append(result.Reasons, resultReason(candidate.TriggerType, result.Score, profile))
	return result
}

func baseScore(trigger TriggerType) float64 {
	switch trigger {
	case TriggerReminder:
		return 95
	case TriggerEventFollowup:
		return 55
	case TriggerMoodRepair:
		return 48
	case TriggerHabitPing:
		return 38
	case TriggerReconnect:
		return 34
	default:
		return 30
	}
}

func conversationWithin(messages []ConversationMessage, now time.Time, window time.Duration) bool {
	for _, msg := range messages {
		if strings.TrimSpace(msg.Role) != "user" {
			continue
		}
		if now.Sub(msg.CreatedAt) <= window {
			return true
		}
	}
	return false
}

func conversationFrequency(messages []ConversationMessage, now time.Time, window time.Duration) int {
	count := 0
	for _, msg := range messages {
		if now.Sub(msg.CreatedAt) <= window && strings.TrimSpace(msg.Role) == "user" {
			count++
		}
	}
	return count
}

func matchedBestWindow(now time.Time, windows []TimeWindow, timezoneName string) bool {
	loc := locationOrDefault(timezoneName)
	localNow := now.In(loc)

	if len(windows) == 0 {
		return localNow.Hour() >= 19 && localNow.Hour() <= 22
	}

	for _, window := range windows {
		if containsWeekday(window.Weekdays, localNow.Weekday()) && inClockRange(localNow, window.Start, window.End) {
			return true
		}
	}

	return false
}

func lastProactiveIgnored(records []ProactiveMessageRecord) bool {
	if len(records) == 0 {
		return false
	}
	last := records[0]
	return !last.UserReplied && last.DeliveryStatus == DeliverySent
}

func consecutiveIgnored(session SessionSnapshot, records []ProactiveMessageRecord) bool {
	if session.ConsecutiveProactiveIgnored >= 2 {
		return true
	}

	if len(records) < 2 {
		return false
	}

	ignored := 0
	for _, record := range records {
		if record.UserReplied {
			break
		}
		if record.DeliveryStatus != DeliverySent {
			continue
		}
		ignored++
		if ignored >= 2 {
			return true
		}
	}
	return false
}

func coldRecentReply(messages []ConversationMessage, now time.Time) bool {
	for _, msg := range messages {
		if strings.TrimSpace(msg.Role) != "user" {
			continue
		}
		if now.Sub(msg.CreatedAt) > 24*time.Hour {
			continue
		}
		content := strings.ToLower(strings.TrimSpace(msg.Content))
		if content == "" {
			continue
		}
		coldMarkers := []string{"ㅋ", "ㅎ", "뭐", "그래", "알았", "응", "네", "..."}
		for _, marker := range coldMarkers {
			if strings.Contains(content, marker) && len([]rune(content)) <= 12 {
				return true
			}
		}
	}
	return false
}

func lateNight(now time.Time, timezoneName string) bool {
	loc := locationOrDefault(timezoneName)
	hour := now.In(loc).Hour()
	return hour < 9 || hour >= 23
}

func recentBotOveruse(messages []ConversationMessage, now time.Time) bool {
	count := 0
	for _, msg := range messages {
		if strings.TrimSpace(msg.Role) != "assistant" {
			continue
		}
		if now.Sub(msg.CreatedAt) <= 12*time.Hour {
			count++
		}
	}
	return count >= 6
}

func sameTriggerRecently(records []ProactiveMessageRecord, trigger TriggerType, refID string, now time.Time) bool {
	for _, record := range records {
		if record.TriggerType != trigger {
			continue
		}
		if strings.TrimSpace(record.TriggerRefID) == strings.TrimSpace(refID) {
			return true
		}
		if now.Sub(record.SentAt) < 24*time.Hour {
			return true
		}
	}
	return false
}

func sentRecently(records []ProactiveMessageRecord, now time.Time, window time.Duration) bool {
	for _, record := range records {
		if record.DeliveryStatus != DeliverySent {
			continue
		}
		if now.Sub(record.SentAt) <= window {
			return true
		}
	}
	return false
}

func profileBias(profile ProactiveProfile, trigger TriggerType) float64 {
	base := profile.ProactiveSuccessScore * 6
	switch trigger {
	case TriggerReminder:
		base += 6
	case TriggerEventFollowup:
		base += 2
	case TriggerReconnect:
		base += 0
	case TriggerMoodRepair:
		base += 1
	case TriggerHabitPing:
		base -= 2
	}

	if contains(profile.PreferredTypes, string(trigger)) {
		base += 5
	}
	if contains(profile.DislikedTypes, string(trigger)) {
		base -= 8
	}

	return clamp(base-3, -8, 8)
}

func resultReason(trigger TriggerType, score float64, profile ProactiveProfile) string {
	if score >= 85 {
		return "high-confidence"
	}
	if score >= 75 {
		return "send-ready"
	}
	if score >= 55 {
		return "defer-and-review"
	}
	if profile.ProactiveSuccessScore > 0.5 {
		return "profile-supports-testing"
	}
	return string(trigger) + "-low-confidence"
}

func clamp(value, minValue, maxValue float64) float64 {
	return math.Max(minValue, math.Min(maxValue, value))
}

func contains(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}
