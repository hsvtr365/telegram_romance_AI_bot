package chat

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
)

const (
	userTraitLike    = "like"
	userTraitAvoid   = "avoid"
	userTraitAllergy = "allergy"
)

type extractedTrait struct {
	TraitType string
	Value     string
}

var (
	allergyRegexes = []*regexp.Regexp{
		regexp.MustCompile(`([가-힣A-Za-z0-9 ]{1,24})\s*알레르기\s*있(?:어|어요|음|습니다)`),
		regexp.MustCompile(`([가-힣A-Za-z0-9 ]{1,24})\s*못\s*먹(?:어|어요)`),
	}
	avoidRegexes = []*regexp.Regexp{
		regexp.MustCompile(`([가-힣A-Za-z0-9 ]{1,24})\s*안\s*마셔`),
		regexp.MustCompile(`([가-힣A-Za-z0-9 ]{1,24})\s*못\s*마셔`),
		regexp.MustCompile(`([가-힣A-Za-z0-9 ]{1,24})\s*안\s*먹(?:어|어요)`),
		regexp.MustCompile(`([가-힣A-Za-z0-9 ]{1,24})\s*싫어해`),
	}
	likeRegexes = []*regexp.Regexp{
		regexp.MustCompile(`([가-힣A-Za-z0-9 ]{1,24})\s*좋아해`),
		regexp.MustCompile(`([가-힣A-Za-z0-9 ]{1,24})\s*좋아하(?:는|고)`),
		regexp.MustCompile(`([가-힣A-Za-z0-9 ]{1,24})\s*즐겨\s*(?:먹|마시|해)`),
	}
)

func (s *Service) captureUserTraits(ctx context.Context, userID int64, input string, conversation *store.ConversationContext) {
	if s.store == nil || userID == 0 || conversation == nil {
		return
	}

	traits := extractUserTraits(input)
	if len(traits) == 0 {
		return
	}

	for _, trait := range traits {
		record, err := s.store.UpsertUserTrait(ctx, pgstore.UpsertUserTraitParams{
			UserID:          userID,
			TraitType:       trait.TraitType,
			NormalizedValue: normalizeTraitValue(trait.Value),
			DisplayValue:    cleanTraitValue(trait.Value),
			SourceText:      strings.TrimSpace(input),
		})
		if err != nil {
			s.logger.Warn("failed to upsert user trait", "user_id", userID, "trait_type", trait.TraitType, "value", trait.Value, "error", err)
			continue
		}
		conversation.UserTraits = mergeTrait(conversation.UserTraits, record)
	}
}

func extractUserTraits(input string) []extractedTrait {
	text := normalizeInput(input)
	if text == "" {
		return nil
	}

	out := make([]extractedTrait, 0, 4)
	out = append(out, extractTraitMatches(text, allergyRegexes, userTraitAllergy)...)
	out = append(out, extractTraitMatches(text, avoidRegexes, userTraitAvoid)...)
	out = append(out, extractTraitMatches(text, likeRegexes, userTraitLike)...)

	return dedupeExtractedTraits(out)
}

func extractTraitMatches(input string, patterns []*regexp.Regexp, traitType string) []extractedTrait {
	out := make([]extractedTrait, 0, len(patterns))
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(input)
		if len(match) < 2 {
			continue
		}
		value := cleanTraitValue(match[1])
		if value == "" {
			continue
		}
		out = append(out, extractedTrait{TraitType: traitType, Value: value})
	}
	return out
}

func cleanTraitValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, ".,!? ")
	value = strings.TrimPrefix(value, "나는 ")
	value = strings.TrimPrefix(value, "저는 ")
	value = strings.TrimPrefix(value, "전 ")
	value = strings.TrimSpace(value)
	if len([]rune(value)) < 1 {
		return ""
	}
	return value
}

func normalizeTraitValue(value string) string {
	value = strings.ToLower(cleanTraitValue(value))
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func dedupeExtractedTraits(values []extractedTrait) []extractedTrait {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]extractedTrait, len(values))
	order := make([]string, 0, len(values))
	for _, item := range values {
		key := item.TraitType + ":" + normalizeTraitValue(item.Value)
		if key == ":" || normalizeTraitValue(item.Value) == "" {
			continue
		}
		if _, ok := seen[key]; !ok {
			order = append(order, key)
		}
		seen[key] = item
	}

	out := make([]extractedTrait, 0, len(order))
	for _, key := range order {
		out = append(out, seen[key])
	}
	return out
}

func mergeTrait(traits []model.UserTrait, next model.UserTrait) []model.UserTrait {
	replaced := false
	for idx := range traits {
		if traits[idx].NormalizedValue == next.NormalizedValue {
			traits[idx] = next
			replaced = true
			break
		}
	}
	if !replaced {
		traits = append(traits, next)
	}

	sort.SliceStable(traits, func(i, j int) bool {
		return traits[i].UpdatedAt.After(traits[j].UpdatedAt)
	})

	return traits
}
