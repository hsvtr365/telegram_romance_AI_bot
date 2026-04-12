package chat

import (
	"sort"
	"strings"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

const (
	userTraitLike    = "like"
	userTraitAvoid   = "avoid"
	userTraitAllergy = "allergy"
	userTraitDelete  = "delete"
)

type extractedTrait struct {
	TraitType string
	Value     string
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
