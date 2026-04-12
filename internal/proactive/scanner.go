package proactive

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

type Scanner struct {
	repo   Repository
	cfg    Config
	clock  Clock
	logger *slog.Logger
}

func NewScanner(repo Repository, cfg Config, logger *slog.Logger) *Scanner {
	cfg = cfg.normalized()
	return &Scanner{
		repo:   repo,
		cfg:    cfg,
		clock:  realClock{},
		logger: logger,
	}
}

func (s *Scanner) ScanOnce(ctx context.Context) ([]ScannedCandidate, error) {
	return s.ScanAt(ctx, s.clock.Now())
}

func (s *Scanner) ScanReminders(ctx context.Context) ([]ScannedCandidate, error) {
	return s.scanAt(ctx, s.clock.Now(), true, false)
}

func (s *Scanner) ScanStandard(ctx context.Context) ([]ScannedCandidate, error) {
	return s.scanAt(ctx, s.clock.Now(), false, true)
}

func (s *Scanner) ScanAt(ctx context.Context, now time.Time) ([]ScannedCandidate, error) {
	return s.scanAt(ctx, now, true, true)
}

func (s *Scanner) scanAt(ctx context.Context, now time.Time, includeReminders bool, includeStandard bool) ([]ScannedCandidate, error) {
	if s.repo == nil || !s.cfg.Enabled {
		return nil, nil
	}

	sessions, err := s.repo.ListSessionsForProactive(ctx, now)
	if err != nil {
		return nil, err
	}

	events, err := s.repo.ListDueMemoryEvents(ctx, now, 200)
	if err != nil {
		return nil, err
	}

	candidates := make([]ScannedCandidate, 0, len(sessions)+len(events))
	sessionByID := make(map[int64]SessionSnapshot, len(sessions))
	for _, session := range sessions {
		sessionByID[session.SessionID] = session
	}

	loadedProfiles := make(map[int64]ProactiveProfile)
	loadedMessages := make(map[int64][]ConversationMessage)
	loadedProactives := make(map[int64][]ProactiveMessageRecord)

	loadContext := func(sessionID int64, userID int64) (ProactiveProfile, []ConversationMessage, []ProactiveMessageRecord) {
		profile, ok := loadedProfiles[userID]
		if !ok {
			profile = profileForSession(ctx, s.repo, userID)
			loadedProfiles[userID] = profile
		}

		msgs, ok := loadedMessages[sessionID]
		if !ok {
			msgs = recentMessagesForSession(ctx, s.repo, sessionID)
			loadedMessages[sessionID] = msgs
		}

		pro, ok := loadedProactives[sessionID]
		if !ok {
			pro = recentProactivesForSession(ctx, s.repo, sessionID)
			loadedProactives[sessionID] = pro
		}

		return profile, msgs, pro
	}

	getProfile := func(userID int64) ProactiveProfile {
		profile, ok := loadedProfiles[userID]
		if !ok {
			profile = profileForSession(ctx, s.repo, userID)
			loadedProfiles[userID] = profile
		}
		return profile
	}

	for _, event := range events {
		session := sessionByID[event.SessionID]
		if session.SessionID == 0 {
			continue
		}
		if candidate, ok := s.eventCandidate(session, event, now, includeReminders, includeStandard); ok {
			profile, recentMessages, recentProactives := loadContext(session.SessionID, session.UserID)
			candidates = append(candidates, ScannedCandidate{
				Candidate:        candidate,
				Session:          session,
				Profile:          profile,
				RecentMessages:   recentMessages,
				RecentProactives: recentProactives,
				Event:            &event,
			})
		}
	}

	for _, session := range sessions {
		if !includeStandard {
			continue
		}
		if candidate, ok := s.reconnectCandidate(session, now); ok {
			profile, recentMessages, recentProactives := loadContext(session.SessionID, session.UserID)
			candidates = append(candidates, ScannedCandidate{
				Candidate:        candidate,
				Session:          session,
				Profile:          profile,
				RecentMessages:   recentMessages,
				RecentProactives: recentProactives,
			})
		}
		if candidate, ok := s.moodRepairCandidate(session, now); ok {
			profile, recentMessages, recentProactives := loadContext(session.SessionID, session.UserID)
			candidates = append(candidates, ScannedCandidate{
				Candidate:        candidate,
				Session:          session,
				Profile:          profile,
				RecentMessages:   recentMessages,
				RecentProactives: recentProactives,
			})
		}
		
		profile := getProfile(session.UserID)
		if candidate, ok := s.habitPingCandidate(session, profile, now); ok {
			_, recentMessages, recentProactives := loadContext(session.SessionID, session.UserID)
			candidates = append(candidates, ScannedCandidate{
				Candidate:        candidate,
				Session:          session,
				Profile:          profile,
				RecentMessages:   recentMessages,
				RecentProactives: recentProactives,
			})
		}
	}

	candidates = dedupeCandidates(candidates)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Candidate.Priority == candidates[j].Candidate.Priority {
			return candidates[i].Candidate.TriggeredAt.Before(candidates[j].Candidate.TriggeredAt)
		}
		return candidates[i].Candidate.Priority > candidates[j].Candidate.Priority
	})

	return candidates, nil
}

func (s *Scanner) eventCandidate(session SessionSnapshot, event MemoryEvent, now time.Time, includeReminders bool, includeStandard bool) (TriggerCandidate, bool) {
	if event.EventType == "reminder" {
		if !includeReminders {
			return TriggerCandidate{}, false
		}
		return s.reminderCandidate(session, event, now)
	}
	if !includeStandard {
		return TriggerCandidate{}, false
	}
	return s.eventFollowupCandidate(session, event, now)
}

func (s *Scanner) reminderCandidate(session SessionSnapshot, event MemoryEvent, now time.Time) (TriggerCandidate, bool) {
	if event.UsedForProactive {
		return TriggerCandidate{}, false
	}
	if !withinEventWindow(event, now) {
		return TriggerCandidate{}, false
	}

	refID := fmt.Sprintf("reminder:%d:%d", session.SessionID, event.EventTime.Unix())
	return TriggerCandidate{
		CandidateID:  buildCandidateID(TriggerReminder, refID),
		SessionID:    session.SessionID,
		UserID:       session.UserID,
		TriggerType:  TriggerReminder,
		TriggerRefID: refID,
		Priority:     120,
		TriggeredAt:  now,
		DueAt:        now,
		Source:       "memory_event_reminder",
		Metadata: map[string]string{
			"event_type":    event.EventType,
			"event_subtype": event.EventSubtype,
		},
		ScoreHints: map[string]float64{
			"bonus": 30,
		},
	}, true
}

func (s *Scanner) eventFollowupCandidate(session SessionSnapshot, event MemoryEvent, now time.Time) (TriggerCandidate, bool) {
	if !s.cfg.EnableEventFollowup {
		return TriggerCandidate{}, false
	}
	if event.UsedForProactive {
		return TriggerCandidate{}, false
	}
	if !withinEventWindow(event, now) {
		return TriggerCandidate{}, false
	}

	refID := fmt.Sprintf("%s:%s:%d", event.EventType, event.EventSubtype, event.EventTime.Unix())
	return TriggerCandidate{
		CandidateID:  buildCandidateID(TriggerEventFollowup, refID),
		SessionID:    session.SessionID,
		UserID:       session.UserID,
		TriggerType:  TriggerEventFollowup,
		TriggerRefID: refID,
		Priority:     100,
		TriggeredAt:  now,
		DueAt:        now,
		Source:       "memory_event",
		Metadata: map[string]string{
			"event_type":    event.EventType,
			"event_subtype": event.EventSubtype,
		},
		ScoreHints: map[string]float64{
			"bonus": float64(maxInt(10, event.Priority/5)),
		},
	}, true
}

func (s *Scanner) reconnectCandidate(session SessionSnapshot, now time.Time) (TriggerCandidate, bool) {
	if !s.cfg.EnableReconnect {
		return TriggerCandidate{}, false
	}
	if !shouldReconnect(session, now, s.cfg) {
		return TriggerCandidate{}, false
	}

	refID := fmt.Sprintf("reconnect:%d:%s", session.SessionID, now.Format("2006-01-02"))
	return TriggerCandidate{
		CandidateID:  buildCandidateID(TriggerReconnect, refID),
		SessionID:    session.SessionID,
		UserID:       session.UserID,
		TriggerType:  TriggerReconnect,
		TriggerRefID: refID,
		Priority:     40,
		TriggeredAt:  now,
		DueAt:        now,
		Source:       "session_gap",
		Metadata: map[string]string{
			"last_user_message_at": session.LastUserMessageAt.Format(time.RFC3339),
		},
	}, true
}

func (s *Scanner) moodRepairCandidate(session SessionSnapshot, now time.Time) (TriggerCandidate, bool) {
	if !s.cfg.EnableMoodRepair {
		return TriggerCandidate{}, false
	}
	if session.CurrentMood == "" || session.CurrentMood == "neutral" {
		return TriggerCandidate{}, false
	}
	if session.LastUserMessageAt.IsZero() {
		return TriggerCandidate{}, false
	}
	if now.Sub(session.LastUserMessageAt) < 30*time.Minute {
		return TriggerCandidate{}, false
	}

	refID := fmt.Sprintf("mood:%d:%s", session.SessionID, session.CurrentMood)
	return TriggerCandidate{
		CandidateID:  buildCandidateID(TriggerMoodRepair, refID),
		SessionID:    session.SessionID,
		UserID:       session.UserID,
		TriggerType:  TriggerMoodRepair,
		TriggerRefID: refID,
		Priority:     80,
		TriggeredAt:  now,
		DueAt:        now,
		Source:       "session_mood",
		Metadata: map[string]string{
			"current_mood": session.CurrentMood,
		},
	}, true
}

func (s *Scanner) habitPingCandidate(session SessionSnapshot, profile ProactiveProfile, now time.Time) (TriggerCandidate, bool) {
	if !s.cfg.EnableHabitPing {
		return TriggerCandidate{}, false
	}
	if len(profile.BestTimeWindows) == 0 {
		return TriggerCandidate{}, false
	}
	if !matchedBestWindow(now, profile.BestTimeWindows, profile.TimezoneName) {
		return TriggerCandidate{}, false
	}

	refID := fmt.Sprintf("habit:%d:%s", session.SessionID, now.Format("2006-01-02T15"))
	return TriggerCandidate{
		CandidateID:  buildCandidateID(TriggerHabitPing, refID),
		SessionID:    session.SessionID,
		UserID:       session.UserID,
		TriggerType:  TriggerHabitPing,
		TriggerRefID: refID,
		Priority:     60,
		TriggeredAt:  now,
		DueAt:        now,
		Source:       "habit_window",
		Metadata: map[string]string{
			"timezone": profile.TimezoneName,
		},
	}, true
}

func shouldReconnect(session SessionSnapshot, now time.Time, cfg Config) bool {
	if session.LastUserMessageAt.IsZero() {
		return true
	}

	age := now.Sub(session.LastUserMessageAt)
	if age < 48*time.Hour {
		return false
	}
	if age > 10*24*time.Hour {
		return true
	}

	return session.RelationshipScore >= 35 && session.ConsecutiveProactiveIgnored == 0
}

func withinEventWindow(event MemoryEvent, now time.Time) bool {
	eventTime := event.EventTime
	if eventTime.IsZero() {
		return false
	}

	if event.EventType == "reminder" {
		delta := now.Sub(eventTime)
		return delta >= 0 && delta <= 30*time.Minute
	}

	delta := now.Sub(eventTime)
	if delta < -3*time.Hour {
		return false
	}
	return delta <= 6*time.Hour
}

func buildCandidateID(trigger TriggerType, refID string) string {
	return fmt.Sprintf("%s:%s", trigger, refID)
}

func dedupeCandidates(candidates []ScannedCandidate) []ScannedCandidate {
	seen := make(map[string]struct{}, len(candidates))
	out := make([]ScannedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := string(candidate.Candidate.TriggerType) + "|" + candidate.Candidate.TriggerRefID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func profileForSession(ctx context.Context, repo Repository, userID int64) ProactiveProfile {
	if repo == nil {
		return ProactiveProfile{}
	}
	profile, err := repo.GetProactiveProfile(ctx, userID)
	if err != nil {
		return ProactiveProfile{UserID: userID, TimezoneName: DefaultTimezoneName, MinGapHours: DefaultMinGapHours, MaxPerDay: DefaultMaxPerDay, MaxPerWeek: DefaultMaxPerWeek}
	}
	if profile.TimezoneName == "" {
		profile.TimezoneName = DefaultTimezoneName
	}
	if profile.MinGapHours <= 0 {
		profile.MinGapHours = DefaultMinGapHours
	}
	if profile.MaxPerDay <= 0 {
		profile.MaxPerDay = DefaultMaxPerDay
	}
	if profile.MaxPerWeek <= 0 {
		profile.MaxPerWeek = DefaultMaxPerWeek
	}
	return profile
}

func recentMessagesForSession(ctx context.Context, repo Repository, sessionID int64) []ConversationMessage {
	if repo == nil {
		return nil
	}
	messages, err := repo.ListRecentMessages(ctx, sessionID, DefaultRecentConversationLimit)
	if err != nil {
		return nil
	}
	return messages
}

func recentProactivesForSession(ctx context.Context, repo Repository, sessionID int64) []ProactiveMessageRecord {
	if repo == nil {
		return nil
	}
	records, err := repo.ListRecentProactiveMessages(ctx, sessionID, DefaultRecentConversationLimit)
	if err != nil {
		return nil
	}
	return records
}
