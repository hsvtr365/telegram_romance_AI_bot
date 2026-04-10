package proactive

import (
	"context"
	"math"
	"strings"
	"time"
)

func AssessFeedback(record ProactiveMessageRecord, reply ConversationMessage, followupMessages []ConversationMessage, profile ProactiveProfile, now time.Time) FeedbackResult {
	if record.ID == 0 || record.SentAt.IsZero() || reply.CreatedAt.IsZero() {
		return FeedbackResult{Matched: false}
	}

	delay := int(reply.CreatedAt.Sub(record.SentAt).Seconds())
	if delay < 0 {
		delay = 0
	}

	replyLength := len([]rune(strings.TrimSpace(reply.Content)))
	sentiment := classifyReplySentiment(reply.Content)
	followupTurns := countFollowupTurns(followupMessages, reply.CreatedAt)

	var relationshipDelta float64
	switch {
	case delay <= 600:
		relationshipDelta += 3
	case delay <= 3600:
		relationshipDelta += 2
	default:
		relationshipDelta += 1
	}

	switch sentiment {
	case "warm":
		relationshipDelta += 2
	case "neutral":
		relationshipDelta += 0.5
	case "cold":
		relationshipDelta -= 2
	}

	if followupTurns >= 4 {
		relationshipDelta += 3
	}

	if replyLength > 0 && replyLength <= 6 {
		relationshipDelta -= 1
	}

	typeBias := typeBiasDelta(record.TriggerType, sentiment, followupTurns)
	timeWindowBias := timeWindowDelta(now, profile)

	return FeedbackResult{
		Matched:               true,
		ReplyDelaySec:         delay,
		ReplyLength:           replyLength,
		ReplySentiment:        sentiment,
		FollowupTurnCount:     followupTurns,
		RelationshipDelta:     clamp(relationshipDelta, -6, 6),
		TypeBiasDelta:         typeBias,
		TimeWindowBiasDelta:   timeWindowBias,
		ShouldMarkUserReplied: true,
		Reasons: []string{
			"feedback_assessed",
		},
	}
}

func ApplyFeedback(ctx context.Context, repo Repository, record ProactiveMessageRecord, reply ConversationMessage, followupMessages []ConversationMessage, profile ProactiveProfile, now time.Time) (FeedbackResult, error) {
	result := AssessFeedback(record, reply, followupMessages, profile, now)
	if !result.Matched || repo == nil {
		return result, nil
	}

	record.UserReplied = result.ShouldMarkUserReplied
	record.ReplyDelaySec = result.ReplyDelaySec
	record.ReplyLength = result.ReplyLength
	record.ReplySentiment = result.ReplySentiment
	record.FollowupTurnCount = result.FollowupTurnCount
	record.UpdatedAt = now

	if err := repo.UpdateProactiveMessage(ctx, record); err != nil {
		return result, err
	}

	ignored := 0
	if !record.UserReplied {
		ignored = 1
	}
	relationship := result.RelationshipDelta
	if relationship != 0 {
		if err := repo.UpdateSessionProactiveState(ctx, record.SessionID, SessionProactivePatch{
			ConsecutiveProactiveIgnored: &ignored,
			RelationshipScore:           float64Ptr(clamp(profile.ProactiveSuccessScore*100+relationship, 0, 100)),
			LastUserReplyToProactiveAt:  &now,
		}); err != nil {
			return result, err
		}
	}

	if err := repo.UpdateProactiveProfile(ctx, record.UserID, ProactiveProfilePatch{
		ProactiveSuccessScore: float64Ptr(clamp(profile.ProactiveSuccessScore+typeBiasToProfileDelta(result.TypeBiasDelta), 0, 1)),
		AvgReplyDelaySec:      intPtr(averageReplyDelay(profile.AvgReplyDelaySec, result.ReplyDelaySec)),
	}); err != nil {
		return result, err
	}

	return result, nil
}

func classifyReplySentiment(text string) string {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return "neutral"
	}

	warmMarkers := []string{"좋", "보고 싶", "ㅎㅎ", "ㅋㅋ", "기다렸", "설레", "좋아"}
	coldMarkers := []string{"몰라", "귀찮", "됐", "싫", "왜 그래", "안 해", "ㄴㄴ"}
	for _, marker := range warmMarkers {
		if strings.Contains(text, marker) {
			return "warm"
		}
	}
	for _, marker := range coldMarkers {
		if strings.Contains(text, marker) {
			return "cold"
		}
	}
	if len([]rune(text)) <= 6 {
		return "cold"
	}
	return "neutral"
}

func countFollowupTurns(messages []ConversationMessage, after time.Time) int {
	count := 0
	for _, msg := range messages {
		if msg.CreatedAt.After(after) {
			count++
		}
	}
	return count
}

func typeBiasDelta(trigger TriggerType, sentiment string, followupTurns int) float64 {
	score := 0.0
	switch trigger {
	case TriggerEventFollowup:
		score += 0.5
	case TriggerReconnect:
		score += 0.2
	case TriggerMoodRepair:
		score += 0.4
	case TriggerHabitPing:
		score -= 0.3
	}

	switch sentiment {
	case "warm":
		score += 0.8
	case "cold":
		score -= 0.8
	}

	if followupTurns >= 4 {
		score += 0.5
	}

	return clamp(score, -2, 2)
}

func timeWindowDelta(now time.Time, profile ProactiveProfile) float64 {
	if matchedBestWindow(now, profile.BestTimeWindows, profile.TimezoneName) {
		return 0.6
	}
	if lateNight(now, profile.TimezoneName) {
		return -0.4
	}
	return 0.1
}

func typeBiasToProfileDelta(value float64) float64 {
	return clamp(value/10, -0.1, 0.1)
}

func averageReplyDelay(current int, newDelay int) int {
	if current <= 0 {
		return newDelay
	}
	return int(math.Round(float64(current*3+newDelay) / 4))
}

func float64Ptr(v float64) *float64 {
	return &v
}

func intPtr(v int) *int {
	return &v
}
