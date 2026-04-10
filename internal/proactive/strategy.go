package proactive

func ChooseStrategy(candidate TriggerCandidate, score ScoreResult, session SessionSnapshot, profile ProactiveProfile) Strategy {
	strategy := defaultStrategyForTrigger(candidate.TriggerType)

	switch {
	case score.Score >= 90 && session.RelationshipScore >= 80:
		strategy.Intensity = IntensityBold
		strategy.Length = LengthMedium
	case score.Score >= 75:
		if strategy.Intensity == IntensityLight {
			strategy.Intensity = IntensityMedium
		}
	case score.Score < 60:
		strategy.Intensity = IntensityLight
	}

	if session.ConsecutiveProactiveIgnored > 0 {
		strategy.Intensity = IntensityLight
	}

	if profile.ProactiveSuccessScore < 0.35 && strategy.Tone == ToneRough {
		strategy.Tone = ToneSpicy
	}

	if candidate.TriggerType == TriggerMoodRepair {
		strategy.Tone = ToneSoft
		strategy.Purpose = PurposeRepair
	}

	if candidate.TriggerType == TriggerEventFollowup && score.Score >= 85 {
		strategy.Purpose = PurposeFollowup
	}

	if candidate.TriggerType == TriggerHabitPing {
		strategy.Purpose = PurposeTease
	}

	if candidate.TriggerType == TriggerReconnect {
		strategy.Purpose = PurposeCheckin
	}

	if score.Score < 65 {
		strategy.Length = LengthShort
	}

	if profileBias(profile, candidate.TriggerType) < 0 {
		strategy.Tone = ToneSoft
	}

	return strategy
}

func defaultStrategyForTrigger(trigger TriggerType) Strategy {
	switch trigger {
	case TriggerReminder:
		return Strategy{
			Type:      string(trigger),
			Intensity: IntensityMedium,
			Tone:      ToneSoft,
			Purpose:   PurposeFollowup,
			Length:    LengthShort,
		}
	case TriggerEventFollowup:
		return Strategy{
			Type:      string(trigger),
			Intensity: IntensityMedium,
			Tone:      ToneSpicy,
			Purpose:   PurposeFollowup,
			Length:    LengthShort,
		}
	case TriggerMoodRepair:
		return Strategy{
			Type:      string(trigger),
			Intensity: IntensityLight,
			Tone:      ToneSoft,
			Purpose:   PurposeRepair,
			Length:    LengthShort,
		}
	case TriggerHabitPing:
		return Strategy{
			Type:      string(trigger),
			Intensity: IntensityLight,
			Tone:      ToneSpicy,
			Purpose:   PurposeTease,
			Length:    LengthShort,
		}
	case TriggerReconnect:
		return Strategy{
			Type:      string(trigger),
			Intensity: IntensityLight,
			Tone:      ToneSpicy,
			Purpose:   PurposeCheckin,
			Length:    LengthShort,
		}
	default:
		return Strategy{
			Type:      string(trigger),
			Intensity: IntensityLight,
			Tone:      ToneSpicy,
			Purpose:   PurposeCheckin,
			Length:    LengthShort,
		}
	}
}
