package holiday

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

const (
	SourceChat      = "chat"
	SourceProactive = "proactive"
)

type ContextResolver interface {
	Resolve(ctx context.Context, input ResolveInput) (*Selection, error)
	MarkUsed(ctx context.Context, sessionID int64, source string, selection Selection) error
}

type ResolverStore interface {
	ListSpecialDays(ctx context.Context, from time.Time, to time.Time) ([]model.SpecialDay, error)
	HasSessionPromptTopic(ctx context.Context, sessionID int64, topicKey string) (bool, error)
	MarkSessionPromptTopicUsed(ctx context.Context, topic model.SessionPromptTopic) error
}

type ResolverConfig struct {
	LookaheadDays   int
	TodayPercent    int
	UpcomingPercent int
}

type ResolveInput struct {
	SessionID int64
	Now       time.Time
	GateSeed  string
}

type Selection struct {
	TopicKey   string
	TopicType  string
	TopicDate  time.Time
	PromptText string
}

type Resolver struct {
	store  ResolverStore
	cfg    ResolverConfig
	logger *slog.Logger
}

type topicCandidate struct {
	key        string
	topicType  string
	day        time.Time
	promptText string
	percent    int
	priority   int
}

func NewResolver(store ResolverStore, cfg ResolverConfig, logger *slog.Logger) *Resolver {
	if cfg.LookaheadDays <= 0 {
		cfg.LookaheadDays = 7
	}
	if cfg.TodayPercent <= 0 {
		cfg.TodayPercent = 40
	}
	if cfg.UpcomingPercent <= 0 {
		cfg.UpcomingPercent = 25
	}

	return &Resolver{
		store:  store,
		cfg:    cfg,
		logger: logger,
	}
}

func (r *Resolver) Resolve(ctx context.Context, input ResolveInput) (*Selection, error) {
	if r == nil || r.store == nil || input.SessionID == 0 {
		return nil, nil
	}

	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}

	today := normalizeDay(now)
	until := today.AddDate(0, 0, r.cfg.LookaheadDays)
	days, err := r.store.ListSpecialDays(ctx, today, until)
	if err != nil {
		return nil, err
	}

	candidates := r.buildCandidates(today, days)
	for _, candidate := range candidates {
		used, err := r.store.HasSessionPromptTopic(ctx, input.SessionID, candidate.key)
		if err != nil {
			return nil, err
		}
		if used {
			continue
		}
		if !passesProbabilityGate(input.SessionID, candidate.key, input.GateSeed, candidate.percent) {
			continue
		}

		return &Selection{
			TopicKey:   candidate.key,
			TopicType:  candidate.topicType,
			TopicDate:  candidate.day,
			PromptText: candidate.promptText,
		}, nil
	}

	return nil, nil
}

func (r *Resolver) MarkUsed(ctx context.Context, sessionID int64, source string, selection Selection) error {
	if r == nil || r.store == nil || sessionID == 0 || strings.TrimSpace(selection.TopicKey) == "" {
		return nil
	}

	if source != SourceChat && source != SourceProactive {
		source = SourceChat
	}

	return r.store.MarkSessionPromptTopicUsed(ctx, model.SessionPromptTopic{
		SessionID: sessionID,
		TopicKey:  selection.TopicKey,
		TopicType: selection.TopicType,
		TopicDate: normalizeDay(selection.TopicDate),
		Source:    source,
		UsedAt:    time.Now().UTC(),
	})
}

func (r *Resolver) buildCandidates(today time.Time, days []model.SpecialDay) []topicCandidate {
	today = normalizeDay(today)
	candidates := make([]topicCandidate, 0, 4)

	todayDays := make([]model.SpecialDay, 0, len(days))
	upcomingByGroup := make(map[string]model.SpecialDay)
	for _, day := range days {
		normalizedDay := normalizeDay(day.Day)
		if normalizedDay.Equal(today) && day.IsHoliday {
			todayDays = append(todayDays, day)
		}

		if !day.IsMajorHoliday || day.MajorHolidayGroup == "" || !normalizedDay.After(today) {
			continue
		}
		current, ok := upcomingByGroup[day.MajorHolidayGroup]
		if !ok || normalizedDay.Before(normalizeDay(current.Day)) || (normalizedDay.Equal(normalizeDay(current.Day)) && compareSpecialDay(day, current) < 0) {
			upcomingByGroup[day.MajorHolidayGroup] = day
		}
	}

	sort.SliceStable(todayDays, func(i, j int) bool {
		return compareSpecialDay(todayDays[i], todayDays[j]) < 0
	})
	for _, day := range todayDays {
		candidates = append(candidates, topicCandidate{
			key:        fmt.Sprintf("today_holiday:%s:%s", normalizeDay(day.Day).Format("2006-01-02"), day.Name),
			topicType:  "today_holiday",
			day:        normalizeDay(day.Day),
			promptText: buildTodayPrompt(day),
			percent:    r.cfg.TodayPercent,
			priority:   200,
		})
	}

	groups := make([]string, 0, len(upcomingByGroup))
	for group := range upcomingByGroup {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	for _, group := range groups {
		day := upcomingByGroup[group]
		candidates = append(candidates, topicCandidate{
			key:        fmt.Sprintf("upcoming_major_holiday:%s:%s", normalizeDay(day.Day).Format("2006-01-02"), group),
			topicType:  "upcoming_major_holiday",
			day:        normalizeDay(day.Day),
			promptText: buildUpcomingPrompt(today, day),
			percent:    r.cfg.UpcomingPercent,
			priority:   100,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority > candidates[j].priority
		}
		if !candidates[i].day.Equal(candidates[j].day) {
			return candidates[i].day.Before(candidates[j].day)
		}
		return candidates[i].key < candidates[j].key
	})

	return candidates
}

func buildTodayPrompt(day model.SpecialDay) string {
	return fmt.Sprintf("[Holiday Context]\n오늘은 %s이다. 대화 흐름에 맞을 때만 아주 짧게 스쳐가듯 반영하고, 억지 축하나 일정 추정은 하지 않는다.", strings.TrimSpace(day.Name))
}

func buildUpcomingPrompt(today time.Time, day model.SpecialDay) string {
	holidayName := "명절"
	switch day.MajorHolidayGroup {
	case "seollal":
		holidayName = "설날"
	case "chuseok":
		holidayName = "추석"
	}

	daysLeft := int(normalizeDay(day.Day).Sub(normalizeDay(today)).Hours() / 24)
	if daysLeft < 1 {
		daysLeft = 1
	}

	return fmt.Sprintf("[Holiday Context]\n%s까지 %d일 남았다. 대화와 맞을 때만 한 번 가볍게 언급할 수 있고, 반복하거나 과장하지 않는다.", holidayName, daysLeft)
}

func compareSpecialDay(left model.SpecialDay, right model.SpecialDay) int {
	if left.IsMajorHoliday != right.IsMajorHoliday {
		if left.IsMajorHoliday {
			return -1
		}
		return 1
	}
	if left.Seq != right.Seq {
		if left.Seq < right.Seq {
			return -1
		}
		return 1
	}
	return strings.Compare(left.Name, right.Name)
}

func normalizeDay(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func passesProbabilityGate(sessionID int64, topicKey string, gateSeed string, percent int) bool {
	if percent >= 100 {
		return true
	}
	if percent <= 0 {
		return false
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(fmt.Sprintf("%d:%s:%s", sessionID, topicKey, strings.TrimSpace(gateSeed))))
	return int(hasher.Sum32()%100) < percent
}
