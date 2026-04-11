package proactive

import (
	"context"
	"log/slog"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/holiday"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

type Scheduler struct {
	cfg      Config
	repo     Repository
	llm      LLM
	reminder LLM
	bot      Messenger
	scanner  *Scanner
	composer *Composer
	sender   *Sender
	logger   *slog.Logger
	clock    Clock
}

func NewScheduler(cfg Config, repo Repository, bot Messenger, llm LLM, reminderLLM LLM, logger *slog.Logger) *Scheduler {
	cfg = cfg.normalized()
	scanner := NewScanner(repo, cfg, logger)
	composer := NewComposer(cfg, llm, reminderLLM, nil, logger)
	sender := NewSender(repo, bot, logger)
	return &Scheduler{
		cfg:      cfg,
		repo:     repo,
		llm:      llm,
		reminder: reminderLLM,
		bot:      bot,
		scanner:  scanner,
		composer: composer,
		sender:   sender,
		logger:   logger,
		clock:    realClock{},
	}
}

func (s *Scheduler) SetHolidayResolver(resolver holiday.ContextResolver) {
	if s == nil {
		return
	}
	if s.composer != nil {
		s.composer.SetHolidayResolver(resolver)
	}
	if s.sender != nil {
		s.sender.SetHolidayResolver(resolver)
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	if !s.cfg.Enabled {
		<-ctx.Done()
		return ctx.Err()
	}

	standardTicker := time.NewTicker(s.cfg.DecisionScanInterval)
	defer standardTicker.Stop()
	reminderTicker := time.NewTicker(s.cfg.ReminderScanInterval)
	defer reminderTicker.Stop()

	if err := s.RunOnce(ctx); err != nil && s.logger != nil {
		s.logger.Warn("proactive initial run failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-reminderTicker.C:
			if err := s.RunReminderPass(ctx); err != nil && s.logger != nil {
				s.logger.Warn("proactive reminder run failed", "error", err)
			}
		case <-standardTicker.C:
			if err := s.RunStandardPass(ctx); err != nil && s.logger != nil {
				s.logger.Warn("proactive standard run failed", "error", err)
			}
		}
	}
}

func (s *Scheduler) RunOnce(ctx context.Context) error {
	if err := s.RunReminderPass(ctx); err != nil {
		return err
	}
	if err := s.RunStandardPass(ctx); err != nil {
		return err
	}
	if err := s.runFeedbackSweep(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Scheduler) RunReminderPass(ctx context.Context) error {
	if s.scanner == nil || s.composer == nil || s.sender == nil {
		return nil
	}

	scanned, err := s.scanner.ScanReminders(ctx)
	if err != nil {
		return err
	}

	for _, item := range s.selectSessionWinners(scanned) {
		if err := s.handleScannedCandidate(ctx, item); err != nil && s.logger != nil {
			s.logger.Warn("proactive candidate handling failed", "session_id", item.Session.SessionID, "trigger_type", item.Candidate.TriggerType, "error", err)
		}
	}

	return nil
}

func (s *Scheduler) RunStandardPass(ctx context.Context) error {
	if s.scanner == nil || s.composer == nil || s.sender == nil {
		return nil
	}

	scanned, err := s.scanner.ScanStandard(ctx)
	if err != nil {
		return err
	}

	for _, item := range s.selectSessionWinners(scanned) {
		if err := s.handleScannedCandidate(ctx, item); err != nil && s.logger != nil {
			s.logger.Warn("proactive candidate handling failed", "session_id", item.Session.SessionID, "trigger_type", item.Candidate.TriggerType, "error", err)
		}
	}

	return nil
}

func (s *Scheduler) handleScannedCandidate(ctx context.Context, item ScannedCandidate) error {
	eligibility := EvaluateEligibility(item.Session, item.Candidate, s.clock.Now(), item.Profile, item.RecentProactives)
	if !eligibility.Eligible {
		return nil
	}

	score := ScoreCandidate(item.Session, item.Profile, item.Candidate, item.RecentMessages, item.RecentProactives, s.clock.Now())
	strategy := ChooseStrategy(item.Candidate, score, item.Session, item.Profile)
	decision := Decision{
		Candidate: item.Candidate,
		Eligible:  eligibility,
		Score:     score,
		Strategy:  strategy,
	}

	switch {
	case item.Candidate.TriggerType == TriggerReminder:
		decision.Action = DecisionSend
		decision.Reason = "requested_reminder"
	case score.Score >= s.cfg.SendThreshold:
		decision.Action = DecisionSend
		decision.Reason = "threshold_send"
	case score.Score >= s.cfg.DeferThreshold:
		decision.Action = DecisionDefer
		decision.Reason = "threshold_defer"
	default:
		decision.Action = DecisionDrop
		decision.Reason = "threshold_drop"
	}

	if decision.Action != DecisionSend {
		return nil
	}

	if s.repo != nil && item.Event != nil {
		_ = s.repo.MarkMemoryEventUsedForProactive(ctx, item.Event.ID)
	}

	if s.composer == nil {
		return nil
	}

	composeResult, ok := preparedReminderComposeResult(item.Event)
	if !ok {
		memorySummary := ""
		if s.repo != nil {
			summary, err := s.repo.GetMemorySummary(ctx, item.Session.SessionID, item.Session.RecentTurnLimit)
			if err != nil {
				if s.logger != nil {
					s.logger.Debug("failed to load proactive memory summary", "session_id", item.Session.SessionID, "error", err)
				}
			} else {
				memorySummary = summary
			}
		}

		var err error
		composeResult, err = s.composer.Compose(ctx, ComposeInput{
			Session:            item.Session,
			Profile:            item.Profile,
			Candidate:          item.Candidate,
			Strategy:           strategy,
			RecentConversation: toOllamaMessages(item.RecentMessages),
			RecentMessages:     item.RecentMessages,
			RecentProactives:   item.RecentProactives,
			MemorySummary:      memorySummary,
			EventNote:          eventNote(item.Event),
		})
		if err != nil {
			return err
		}
	}

	_, err := s.sender.Send(ctx, item.Session, decision, composeResult)
	return err
}

func (s *Scheduler) selectSessionWinners(scanned []ScannedCandidate) []ScannedCandidate {
	if len(scanned) <= 1 {
		return scanned
	}

	bySession := make(map[int64][]ScannedCandidate)
	order := make([]int64, 0, len(scanned))
	for _, item := range scanned {
		if _, ok := bySession[item.Session.SessionID]; !ok {
			order = append(order, item.Session.SessionID)
		}
		bySession[item.Session.SessionID] = append(bySession[item.Session.SessionID], item)
	}

	winners := make([]ScannedCandidate, 0, len(bySession))
	now := s.clock.Now()
	for _, sessionID := range order {
		items := bySession[sessionID]
		best, ok := s.pickBestCandidate(items, now)
		if ok {
			winners = append(winners, best)
		}
	}

	return winners
}

func (s *Scheduler) pickBestCandidate(items []ScannedCandidate, now time.Time) (ScannedCandidate, bool) {
	var (
		best      ScannedCandidate
		bestScore float64
		found     bool
	)

	for _, item := range items {
		eligibility := EvaluateEligibility(item.Session, item.Candidate, now, item.Profile, item.RecentProactives)
		if !eligibility.Eligible {
			continue
		}

		score := ScoreCandidate(item.Session, item.Profile, item.Candidate, item.RecentMessages, item.RecentProactives, now)
		if score.Score < s.cfg.SendThreshold {
			continue
		}

		if !found || candidateOutranks(item, score.Score, best, bestScore) {
			best = item
			bestScore = score.Score
			found = true
		}
	}

	return best, found
}

func candidateOutranks(next ScannedCandidate, nextScore float64, current ScannedCandidate, currentScore float64) bool {
	if next.Candidate.Priority != current.Candidate.Priority {
		return next.Candidate.Priority > current.Candidate.Priority
	}
	if nextScore != currentScore {
		return nextScore > currentScore
	}
	return next.Candidate.TriggeredAt.After(current.Candidate.TriggeredAt)
}

func (s *Scheduler) runFeedbackSweep(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}

	now := s.clock.Now()
	sessions, err := s.repo.ListSessionsForProactive(ctx, now)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		proactives, err := s.repo.ListRecentProactiveMessages(ctx, session.SessionID, 8)
		if err != nil || len(proactives) == 0 {
			continue
		}
		recentMessages, err := s.repo.ListRecentMessages(ctx, session.SessionID, 8)
		if err != nil || len(recentMessages) == 0 {
			continue
		}

		latestProactive := proactives[0]
		reply, ok := findFirstUserReplyAfter(recentMessages, latestProactive.SentAt)
		if !ok {
			continue
		}

		profile, err := s.repo.GetProactiveProfile(ctx, session.UserID)
		if err != nil {
			profile = ProactiveProfile{UserID: session.UserID, TimezoneName: s.cfg.TimezoneName}
		}

		result := AssessFeedback(latestProactive, reply, followupMessagesAfter(recentMessages, reply.CreatedAt), profile, now)
		if !result.Matched {
			continue
		}

		updated := latestProactive
		updated.UserReplied = true
		updated.ReplyDelaySec = result.ReplyDelaySec
		updated.ReplyLength = result.ReplyLength
		updated.ReplySentiment = result.ReplySentiment
		updated.FollowupTurnCount = result.FollowupTurnCount
		updated.UpdatedAt = now
		_ = s.repo.UpdateProactiveMessage(ctx, updated)

		if err := s.repo.UpdateSessionProactiveState(ctx, session.SessionID, SessionProactivePatch{
			LastUserReplyToProactiveAt:  &reply.CreatedAt,
			ConsecutiveProactiveIgnored: intPtr(0),
			RelationshipScore:           float64Ptr(clamp(session.RelationshipScore+result.RelationshipDelta, 0, 100)),
		}); err != nil && s.logger != nil {
			s.logger.Warn("failed to update session after feedback", "session_id", session.SessionID, "error", err)
		}

		if err := s.repo.UpdateProactiveProfile(ctx, session.UserID, ProactiveProfilePatch{
			AvgReplyDelaySec:      intPtr(averageReplyDelay(profile.AvgReplyDelaySec, result.ReplyDelaySec)),
			ProactiveSuccessScore: float64Ptr(clamp(profile.ProactiveSuccessScore+typeBiasToProfileDelta(result.TypeBiasDelta), 0, 1)),
		}); err != nil && s.logger != nil {
			s.logger.Warn("failed to update proactive profile after feedback", "user_id", session.UserID, "error", err)
		}
	}

	return nil
}

func toOllamaMessages(messages []ConversationMessage) []ollama.Message {
	out := make([]ollama.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, ollama.Message{Role: msg.Role, Content: msg.Content})
	}
	return out
}

func eventNote(event *MemoryEvent) string {
	if event == nil {
		return ""
	}
	return event.EventType + "/" + event.EventSubtype
}

func preparedReminderComposeResult(event *MemoryEvent) (ComposeResult, bool) {
	if event == nil || event.EventType != "reminder" {
		return ComposeResult{}, false
	}

	message := event.PreparedMessage()
	if message == "" {
		message = fallbackReminderMessage(event)
	}
	if message == "" {
		return ComposeResult{}, false
	}

	return ComposeResult{
		Message: message,
		Seed: Seed{
			Key:  "prepared_reminder",
			Text: message,
		},
	}, true
}

func fallbackReminderMessage(event *MemoryEvent) string {
	if event == nil {
		return ""
	}
	if event.EventSubtype == "oneoff" {
		return "약속한 시간이라 알려주러 왔어."
	}
	return "시간 돼서 톡 남겨."
}

func findFirstUserReplyAfter(messages []ConversationMessage, after time.Time) (ConversationMessage, bool) {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		if msg.CreatedAt.After(after) {
			return msg, true
		}
	}
	return ConversationMessage{}, false
}

func followupMessagesAfter(messages []ConversationMessage, after time.Time) []ConversationMessage {
	out := make([]ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.CreatedAt.After(after) {
			out = append(out, msg)
		}
	}
	return out
}
