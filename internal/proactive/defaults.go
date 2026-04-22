package proactive

import "time"

const (
	DefaultRecentConversationLimit = 14
	DefaultSummaryLineLimit        = 6
	DefaultMessageQuestionLimit    = 1
	DefaultMessageSentenceLimit    = 2
	DefaultMessageRuneLimit        = 110

	DefaultSendThreshold  = 75.0
	DefaultDeferThreshold = 55.0

	DefaultReconnectCooldownHours     = 72
	DefaultEventFollowupCooldownHours = 0
	DefaultMoodRepairCooldownHours    = 18
	DefaultHabitPingCooldownHours     = 24
	DefaultMinGapHours                = 20
	DefaultMaxPerDay                  = 1
	DefaultMaxPerWeek                 = 4

	DefaultDecisionScanInterval  = 15 * time.Second
	DefaultReminderScanInterval  = 5 * time.Second
	DefaultReconnectScanInterval = 5 * time.Minute
	DefaultEventScanInterval     = 1 * time.Minute
	DefaultMoodScanInterval      = 1 * time.Minute
	DefaultHabitScanInterval     = 5 * time.Minute
	DefaultFeedbackInterval      = 10 * time.Minute
	DefaultSenderInterval        = 1 * time.Second
	DefaultWorkerConcurrency     = 1
	DefaultRewardTTSMinFollowups = 1

	DefaultTimezoneName = "Asia/Seoul"
)

type Config struct {
	Enabled                    bool
	EnableReconnect            bool
	EnableEventFollowup        bool
	EnableMoodRepair           bool
	EnableHabitPing            bool
	RecentConversationLimit    int
	SummaryLineLimit           int
	MessageQuestionLimit       int
	MessageSentenceLimit       int
	MessageRuneLimit           int
	SendThreshold              float64
	DeferThreshold             float64
	ReconnectCooldownHours     int
	EventFollowupCooldownHours int
	MoodRepairCooldownHours    int
	HabitPingCooldownHours     int
	MinGapHours                int
	MaxPerDay                  int
	MaxPerWeek                 int
	DecisionScanInterval       time.Duration
	ReminderScanInterval       time.Duration
	ReconnectScanInterval      time.Duration
	EventScanInterval          time.Duration
	MoodScanInterval           time.Duration
	HabitScanInterval          time.Duration
	FeedbackInterval           time.Duration
	SenderConcurrency          int
	TimezoneName               string
	RewardTTSEnabled           bool
	RewardTTSMinFollowups      int
}

func DefaultConfig() Config {
	return Config{
		Enabled:                    true,
		EnableReconnect:            true,
		EnableEventFollowup:        true,
		EnableMoodRepair:           true,
		EnableHabitPing:            true,
		RecentConversationLimit:    DefaultRecentConversationLimit,
		SummaryLineLimit:           DefaultSummaryLineLimit,
		MessageQuestionLimit:       DefaultMessageQuestionLimit,
		MessageSentenceLimit:       DefaultMessageSentenceLimit,
		MessageRuneLimit:           DefaultMessageRuneLimit,
		SendThreshold:              DefaultSendThreshold,
		DeferThreshold:             DefaultDeferThreshold,
		ReconnectCooldownHours:     DefaultReconnectCooldownHours,
		EventFollowupCooldownHours: DefaultEventFollowupCooldownHours,
		MoodRepairCooldownHours:    DefaultMoodRepairCooldownHours,
		HabitPingCooldownHours:     DefaultHabitPingCooldownHours,
		MinGapHours:                DefaultMinGapHours,
		MaxPerDay:                  DefaultMaxPerDay,
		MaxPerWeek:                 DefaultMaxPerWeek,
		DecisionScanInterval:       DefaultDecisionScanInterval,
		ReminderScanInterval:       DefaultReminderScanInterval,
		ReconnectScanInterval:      DefaultReconnectScanInterval,
		EventScanInterval:          DefaultEventScanInterval,
		MoodScanInterval:           DefaultMoodScanInterval,
		HabitScanInterval:          DefaultHabitScanInterval,
		FeedbackInterval:           DefaultFeedbackInterval,
		SenderConcurrency:          DefaultWorkerConcurrency,
		TimezoneName:               DefaultTimezoneName,
		RewardTTSMinFollowups:      DefaultRewardTTSMinFollowups,
	}
}

func (c Config) normalized() Config {
	if c.RecentConversationLimit <= 0 {
		c.RecentConversationLimit = DefaultRecentConversationLimit
	}
	if c.SummaryLineLimit <= 0 {
		c.SummaryLineLimit = DefaultSummaryLineLimit
	}
	if c.MessageQuestionLimit <= 0 {
		c.MessageQuestionLimit = DefaultMessageQuestionLimit
	}
	if c.MessageSentenceLimit <= 0 {
		c.MessageSentenceLimit = DefaultMessageSentenceLimit
	}
	if c.MessageRuneLimit <= 0 {
		c.MessageRuneLimit = DefaultMessageRuneLimit
	}
	if c.SendThreshold == 0 {
		c.SendThreshold = DefaultSendThreshold
	}
	if c.DeferThreshold == 0 {
		c.DeferThreshold = DefaultDeferThreshold
	}
	if c.ReconnectCooldownHours <= 0 {
		c.ReconnectCooldownHours = DefaultReconnectCooldownHours
	}
	if c.EventFollowupCooldownHours < 0 {
		c.EventFollowupCooldownHours = DefaultEventFollowupCooldownHours
	}
	if c.MoodRepairCooldownHours <= 0 {
		c.MoodRepairCooldownHours = DefaultMoodRepairCooldownHours
	}
	if c.HabitPingCooldownHours <= 0 {
		c.HabitPingCooldownHours = DefaultHabitPingCooldownHours
	}
	if c.MinGapHours <= 0 {
		c.MinGapHours = DefaultMinGapHours
	}
	if c.MaxPerDay <= 0 {
		c.MaxPerDay = DefaultMaxPerDay
	}
	if c.MaxPerWeek <= 0 {
		c.MaxPerWeek = DefaultMaxPerWeek
	}
	if c.DecisionScanInterval <= 0 {
		c.DecisionScanInterval = DefaultDecisionScanInterval
	}
	if c.ReminderScanInterval <= 0 {
		c.ReminderScanInterval = DefaultReminderScanInterval
	}
	if c.ReconnectScanInterval <= 0 {
		c.ReconnectScanInterval = DefaultReconnectScanInterval
	}
	if c.EventScanInterval <= 0 {
		c.EventScanInterval = DefaultEventScanInterval
	}
	if c.MoodScanInterval <= 0 {
		c.MoodScanInterval = DefaultMoodScanInterval
	}
	if c.HabitScanInterval <= 0 {
		c.HabitScanInterval = DefaultHabitScanInterval
	}
	if c.FeedbackInterval <= 0 {
		c.FeedbackInterval = DefaultFeedbackInterval
	}
	if c.SenderConcurrency <= 0 {
		c.SenderConcurrency = DefaultWorkerConcurrency
	}
	if c.TimezoneName == "" {
		c.TimezoneName = DefaultTimezoneName
	}
	if c.RewardTTSMinFollowups <= 0 {
		c.RewardTTSMinFollowups = DefaultRewardTTSMinFollowups
	}
	return c
}
