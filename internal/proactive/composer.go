package proactive

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/holiday"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

type ComposeInput struct {
	Session            SessionSnapshot
	Profile            ProactiveProfile
	Candidate          TriggerCandidate
	Strategy           Strategy
	RecentConversation []ollama.Message
	RecentMessages     []ConversationMessage
	RecentProactives   []ProactiveMessageRecord
	MemorySummary      string
	RelationshipNote   string
	EventNote          string
}

type ComposeResult struct {
	Message           string
	Seed              Seed
	HolidayTopic      *holiday.Selection
	HolidayPromptUsed bool
}

type Composer struct {
	cfg           Config
	llm           LLM
	reminderLLM   LLM
	promptBuilder *PromptBuilder
	seeds         SeedLibrary
	holiday       holiday.ContextResolver
	logger        *slog.Logger
}

func NewComposer(cfg Config, llm LLM, reminderLLM LLM, seeds SeedLibrary, logger *slog.Logger) *Composer {
	cfg = cfg.normalized()
	if seeds == nil {
		seeds = NewDefaultSeedLibrary()
	}

	return &Composer{
		cfg:           cfg,
		llm:           llm,
		reminderLLM:   reminderLLM,
		promptBuilder: NewPromptBuilder(),
		seeds:         seeds,
		logger:        logger,
	}
}

func (c *Composer) SetHolidayResolver(resolver holiday.ContextResolver) {
	if c == nil {
		return
	}
	c.holiday = resolver
}

func (c *Composer) Compose(ctx context.Context, input ComposeInput) (ComposeResult, error) {
	seed := c.seeds.Pick(input.Candidate, input.Strategy, input.Profile)
	llm := c.selectLLM(input.Candidate)
	if llm == nil {
		fallback := normalizeProactiveText(seed.Text, c.cfg)
		fallback = postProcessText(fallback)
		fallback = capQuestions(fallback, c.questionLimit(input.Candidate))
		fallback = capSentences(fallback, c.cfg.MessageSentenceLimit)
		fallback = limitRunes(fallback, c.cfg.MessageRuneLimit)
		return ComposeResult{Message: fallback, Seed: seed}, nil
	}

	var holidaySelection *holiday.Selection
	if c.holiday != nil {
		selected, err := c.holiday.Resolve(ctx, holiday.ResolveInput{
			SessionID: input.Session.SessionID,
			Now:       resolveHolidayNow(input.Profile.TimezoneName, input.Session.TimezoneName),
			GateSeed:  input.Candidate.CandidateID,
		})
		if err != nil {
			if c.logger != nil {
				c.logger.Warn("failed to resolve holiday context for proactive", "session_id", input.Session.SessionID, "candidate_id", input.Candidate.CandidateID, "error", err)
			}
		} else {
			holidaySelection = selected
		}
	}

	holidayText := ""
	if holidaySelection != nil {
		holidayText = holidaySelection.PromptText
	}

	promptMessages := c.promptBuilder.Build(PromptInput{
		Candidate:          input.Candidate,
		Strategy:           input.Strategy,
		Seed:               seed,
		MemorySummary:      input.MemorySummary,
		RelationshipNote:   input.RelationshipNote,
		EventNote:          input.EventNote,
		HolidayContextText: holidayText,
		RecentConversation: input.RecentConversation,
	})

	reply, err := llm.Chat(ctx, promptMessages)
	if err != nil {
		if c.logger != nil {
			c.logger.Warn("proactive llm failed, using seed fallback", "session_id", input.Session.SessionID, "trigger_type", input.Candidate.TriggerType, "error", err)
		}
		reply = seed.Text
		holidaySelection = nil
	}

	reply = normalizeProactiveText(reply, c.cfg)
	if reply == "" {
		reply = normalizeProactiveText(seed.Text, c.cfg)
	}

	reply = postProcessText(reply)
	reply = capQuestions(reply, c.questionLimit(input.Candidate))
	reply = capSentences(reply, c.cfg.MessageSentenceLimit)
	reply = limitRunes(reply, c.cfg.MessageRuneLimit)

	if reply == "" {
		reply = seed.Text
	}

	return ComposeResult{
		Message:           reply,
		Seed:              seed,
		HolidayTopic:      holidaySelection,
		HolidayPromptUsed: holidaySelection != nil,
	}, nil
}

func (c *Composer) selectLLM(candidate TriggerCandidate) LLM {
	if candidate.TriggerType == TriggerReminder && c.reminderLLM != nil {
		return c.reminderLLM
	}
	return c.llm
}

func (c *Composer) questionLimit(candidate TriggerCandidate) int {
	if candidate.TriggerType == TriggerReminder {
		return 1
	}
	return c.cfg.MessageQuestionLimit
}
func normalizeProactiveText(text string, _ Config) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	replacer := strings.NewReplacer(
		"정리하자면", "",
		"설명하자면", "",
		"결론적으로", "",
		"내가 먼저 연락한 이유는", "",
		"그냥 확인차", "",
	)
	text = replacer.Replace(text)
	text = strings.TrimSpace(text)
	return text
}

func capQuestions(text string, maxQuestions int) string {
	if maxQuestions <= 0 {
		return text
	}

	questionCount := 0
	var builder strings.Builder
	for _, r := range text {
		builder.WriteRune(r)
		if r == '?' || r == '？' {
			questionCount++
			if questionCount >= maxQuestions {
				break
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func capSentences(text string, maxSentences int) string {
	if maxSentences <= 0 {
		return text
	}

	sentences := splitReplyForTelegram(text)
	if len(sentences) <= maxSentences {
		return text
	}
	return strings.Join(sentences[:maxSentences], " ")
}

func limitRunes(text string, limit int) string {
	if limit <= 0 {
		return text
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func (c *Composer) ComposePreview(candidate TriggerCandidate, strategy Strategy, profile ProactiveProfile) Seed {
	return c.seeds.Pick(candidate, strategy, profile)
}

func resolveHolidayNow(names ...string) time.Time {
	now := time.Now()
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		location, err := time.LoadLocation(name)
		if err == nil {
			return now.In(location)
		}
	}
	return now.In(time.FixedZone("KST", 9*60*60))
}
