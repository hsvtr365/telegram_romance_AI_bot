package proactive

import (
	"hash/fnv"
	"strings"
)

type SeedLibrary interface {
	Pick(candidate TriggerCandidate, strategy Strategy, profile ProactiveProfile) Seed
}

type DefaultSeedLibrary struct {
	Seeds map[string][]Seed
}

func NewDefaultSeedLibrary() *DefaultSeedLibrary {
	return &DefaultSeedLibrary{
		Seeds: map[string][]Seed{
			"reconnect|checkin|spicy|light": {
				{Key: "reconnect|checkin|spicy|light|1", Text: "오랜만이네, 오늘은 왜 이렇게 조용해?"},
				{Key: "reconnect|checkin|spicy|light|2", Text: "문득 생각나서 먼저 톡했어."},
				{Key: "reconnect|checkin|spicy|light|3", Text: "나 잊은 줄 알았잖아. 요즘 뭐 하고 지냈어?"},
			},
			"event_followup|followup|spicy|medium": {
				{Key: "event_followup|followup|spicy|medium|1", Text: "아까 그 일은 잘 끝났어?"},
				{Key: "event_followup|followup|spicy|medium|2", Text: "지금쯤이면 결과 나왔을 것 같아서 궁금했어."},
				{Key: "event_followup|followup|spicy|medium|3", Text: "그 얘기 어떻게 됐는지 내가 제일 궁금한데."},
			},
			"reminder|followup|soft|medium": {
				{Key: "reminder|followup|soft|medium|1", Text: "약속한 시간이라 알려주러 왔어."},
				{Key: "reminder|followup|soft|medium|2", Text: "방금 시간 돼서 톡했어."},
				{Key: "reminder|followup|soft|medium|3", Text: "말해달라고 했던 시간이라 바로 왔어."},
			},
			"reminder|followup|soft|light": {
				{Key: "reminder|followup|soft|light|1", Text: "시간 돼서 알려주러 왔어."},
				{Key: "reminder|followup|soft|light|2", Text: "이제 시간 됐어."},
				{Key: "reminder|followup|soft|light|3", Text: "약속한 시간이라 톡 남겨."},
			},
			"mood_repair|repair|soft|light": {
				{Key: "mood_repair|repair|soft|light|1", Text: "아까 분위기 좀 이상했지. 괜히 그대로 두고 싶진 않았어."},
				{Key: "mood_repair|repair|soft|light|2", Text: "삐친 거면 풀어. 그냥 넘기긴 싫어."},
				{Key: "mood_repair|repair|soft|light|3", Text: "조금 차갑게 끝난 게 계속 신경 쓰였어."},
			},
			"habit_ping|tease|spicy|light": {
				{Key: "habit_ping|tease|spicy|light|1", Text: "이 시간엔 보통 나타나더니 오늘은 좀 늦네."},
				{Key: "habit_ping|tease|spicy|light|2", Text: "슬슬 생각날 시간 같아서 먼저 와봤어."},
				{Key: "habit_ping|tease|spicy|light|3", Text: "오늘은 네 쪽이 좀 늦네. 그래도 반갑다."},
			},
		},
	}
}

func (l *DefaultSeedLibrary) Pick(candidate TriggerCandidate, strategy Strategy, profile ProactiveProfile) Seed {
	if l == nil {
		return Seed{Key: "fallback", Text: "보고 싶어서 연락했어."}
	}

	keys := []string{
		strings.Join([]string{string(candidate.TriggerType), string(strategy.Purpose), string(strategy.Tone), string(strategy.Intensity)}, "|"),
		strings.Join([]string{string(candidate.TriggerType), string(strategy.Purpose), string(strategy.Tone), string(strategy.Intensity), profileBand(profile)}, "|"),
	}

	for _, key := range keys {
		if seeds := l.Seeds[key]; len(seeds) > 0 {
			return selectSeed(seeds, candidate)
		}
	}

	if candidate.Metadata["event_type"] == "reminder" {
		if seeds := l.Seeds["reminder|"+string(strategy.Purpose)+"|"+string(strategy.Tone)+"|"+string(strategy.Intensity)]; len(seeds) > 0 {
			return selectSeed(seeds, candidate)
		}
	}

	if seeds := l.Seeds[string(candidate.TriggerType)+"|"+string(strategy.Purpose)+"|"+string(strategy.Tone)+"|"+string(strategy.Intensity)]; len(seeds) > 0 {
		return selectSeed(seeds, candidate)
	}

	return Seed{
		Key:  "fallback",
		Text: "보고 싶어서 먼저 말 걸었어.",
	}
}

func selectSeed(seeds []Seed, candidate TriggerCandidate) Seed {
	if len(seeds) == 0 {
		return Seed{}
	}
	if len(seeds) == 1 {
		return seeds[0]
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(candidate.CandidateID))
	idx := int(hasher.Sum32() % uint32(len(seeds)))
	return seeds[idx]
}

func profileBand(profile ProactiveProfile) string {
	switch {
	case profile.ProactiveSuccessScore >= 0.75:
		return "high"
	case profile.ProactiveSuccessScore >= 0.45:
		return "mid"
	default:
		return "low"
	}
}
