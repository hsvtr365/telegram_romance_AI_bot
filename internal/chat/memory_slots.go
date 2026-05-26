package chat

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/promptutil"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/textutil"
)

type MemoryPromptSections struct {
	ActiveTopicsText             string
	ConversationStateText        string
	ConversationStateMachineText string
	OpenLoopsText                string
	MemorySummary                string
}

func BuildMemoryPromptSections(slots []model.TopicSlot, state model.ConversationStateSlot) MemoryPromptSections {
	visibleTopics := filterPromptTopicSlots(slots)

	var topicLines []string
	for _, slot := range visibleTopics {
		line := buildTopicPromptLine(slot)
		if line != "" {
			topicLines = append(topicLines, line)
		}
	}

	var stateLines []string
	if shouldInjectConversationState(state) {
		if value := strings.TrimSpace(state.CurrentStage); value != "" {
			stateLines = append(stateLines, "stage="+value)
		}
		if value := strings.TrimSpace(state.StageDirection); value != "" {
			stateLines = append(stateLines, "direction="+value)
		}
		if value := strings.TrimSpace(state.EmotionalTone); value != "" {
			stateLines = append(stateLines, "tone="+value)
		}
		if value := strings.TrimSpace(state.InteractionMode); value != "" {
			stateLines = append(stateLines, "mode="+value)
		}
		if value := strings.TrimSpace(state.FocusTopicKey); value != "" {
			stateLines = append(stateLines, "focus_topic="+value)
		}
		if value := formatPromptTimeKV("updated_at", state.UpdatedAt); value != "" {
			stateLines = append(stateLines, value)
		}
	}

	openLoopText := ""
	if shouldInjectConversationState(state) {
		openLoopText = strings.TrimSpace(state.OpenLoopSummary)
	}

	activeTopicsText := strings.Join(topicLines, "\n")
	conversationStateText := strings.Join(stateLines, "\n")
	sections := MemoryPromptSections{
		ActiveTopicsText:      activeTopicsText,
		ConversationStateText: conversationStateText,
		OpenLoopsText:         openLoopText,
	}
	sections.MemorySummary = buildMemorySummaryText(sections)
	return sections
}

func BuildMemorySummary(slots []model.TopicSlot, state model.ConversationStateSlot) string {
	return BuildMemoryPromptSections(slots, state).MemorySummary
}

func BuildMemoryPromptSectionsFromStateMachine(slots []model.TopicSlot, state model.ConversationStateMachine) MemoryPromptSections {
	visibleTopics := filterPromptTopicSlots(slots)

	var topicLines []string
	for _, slot := range visibleTopics {
		line := buildTopicPromptLine(slot)
		if line != "" {
			topicLines = append(topicLines, line)
		}
	}

	stateLines := buildConversationStateMachineLines(state)
	sections := MemoryPromptSections{
		ActiveTopicsText:             strings.Join(topicLines, "\n"),
		ConversationStateMachineText: strings.Join(stateLines, "\n"),
		OpenLoopsText:                strings.TrimSpace(state.OpenLoopSummary),
	}
	sections.MemorySummary = buildMemorySummaryText(sections)
	return sections
}

func BuildMemorySummaryFromStateMachine(slots []model.TopicSlot, state model.ConversationStateMachine) string {
	return BuildMemoryPromptSectionsFromStateMachine(slots, state).MemorySummary
}

func MergeMemorySlotAnalysis(existing []model.TopicSlot, existingState model.ConversationStateSlot, result model.MemorySlotAnalysisResult, sourceMessageID int64, at time.Time) ([]model.TopicSlot, model.ConversationStateSlot) {
	if at.IsZero() {
		at = time.Now().UTC()
	}

	byKey := make(map[string]model.TopicSlot, len(existing))
	for _, slot := range existing {
		key := normalizeTopicSlotKey(slot.SlotKey)
		if key == "" {
			key = topicKeyFromLabel(slot.TopicLabel)
		}
		if key == "" {
			continue
		}
		slot.SlotKey = key
		byKey[key] = slot
	}

	for _, label := range result.ResolvedTopics {
		key := topicKeyFromLabel(label)
		slot, ok := byKey[key]
		if !ok {
			continue
		}
		slot.Status = model.TopicSlotStatusResolved
		slot.LastSeenAt = at
		if sourceMessageID != 0 {
			slot.LastSourceMessageID = sourceMessageID
		}
		byKey[key] = slot
	}

	for _, analyzed := range result.Topics {
		label := cleanMemoryText(analyzed.Label)
		key := topicKeyFromLabel(label)
		if key == "" {
			continue
		}

		slot, exists := byKey[key]
		if !exists {
			slot = model.TopicSlot{
				SessionID:   existingState.SessionID,
				SlotKey:     key,
				FirstSeenAt: at,
			}
		}

		slot.SlotKey = key
		slot.TopicLabel = textutil.FirstNonBlank(label, slot.TopicLabel)
		slot.Summary = textutil.FirstNonBlank(cleanMemoryText(analyzed.Summary), slot.Summary, slot.TopicLabel)
		slot.Status = deriveTopicStatus(analyzed, slot.Status)
		slot.Importance = clampImportance(analyzed.Importance, slot.Importance)
		slot.Confidence = deriveConfidence(analyzed.Confidence, slot.Confidence)
		slot.SourceKind = "memory_slot_ai"
		if slot.FirstSeenAt.IsZero() {
			slot.FirstSeenAt = at
		}
		slot.LastSeenAt = at
		if sourceMessageID != 0 {
			slot.LastSourceMessageID = sourceMessageID
		}
		if slot.MentionCount <= 0 {
			slot.MentionCount = 1
		} else {
			slot.MentionCount++
		}
		slot.EvidenceJSON = evidenceJSON(analyzed.EvidenceTexts)

		byKey[key] = slot

		for _, supersede := range analyzed.Supersedes {
			superKey := topicKeyFromLabel(supersede)
			if superKey == "" || superKey == key {
				continue
			}
			superSlot, ok := byKey[superKey]
			if !ok {
				continue
			}
			superSlot.Status = model.TopicSlotStatusArchived
			superSlot.LastSeenAt = at
			if sourceMessageID != 0 {
				superSlot.LastSourceMessageID = sourceMessageID
			}
			byKey[superKey] = superSlot
		}
	}

	slots := make([]model.TopicSlot, 0, len(byKey))
	for _, slot := range byKey {
		if slot.Status == "" {
			slot.Status = model.TopicSlotStatusWatch
		}
		slots = append(slots, slot)
	}
	slots = enforceTopicSlotLimits(slots)

	state := mergeConversationStateSlot(existingState, result.ConversationState, sourceMessageID, at)
	return slots, state
}

func shouldAnalyzeMemorySlots(input string, minChars int) bool {
	if minChars <= 0 {
		minChars = 5
	}
	cleaned := cleanMemoryText(input)
	return len([]rune(cleaned)) >= minChars
}

func normalizeTopicSlotKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '가' && r <= '힣'):
			builder.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(builder.String(), "-")
}

func topicKeyFromLabel(label string) string {
	return normalizeTopicSlotKey(cleanMemoryText(label))
}

func filterPromptTopicSlots(slots []model.TopicSlot) []model.TopicSlot {
	filtered := make([]model.TopicSlot, 0, len(slots))
	for _, slot := range slots {
		status := normalizeTopicStatus(slot.Status)
		if status != model.TopicSlotStatusActive && status != model.TopicSlotStatusWatch {
			continue
		}
		confidence := normalizeConfidence(slot.Confidence)
		// Hide weak one-off topics from the prompt until they recur at least once.
		if confidence == model.ConfidenceLow && slot.MentionCount < 2 {
			continue
		}
		filtered = append(filtered, slot)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Importance != filtered[j].Importance {
			return filtered[i].Importance > filtered[j].Importance
		}
		return filtered[i].LastSeenAt.After(filtered[j].LastSeenAt)
	})

	if len(filtered) > 5 {
		filtered = filtered[:5]
	}
	return filtered
}

func shouldInjectConversationState(state model.ConversationStateSlot) bool {
	return state.CurrentStage != "" || state.EmotionalTone != ""
}

func shouldInjectConversationStateMachine(state model.ConversationStateMachine) bool {
	return strings.TrimSpace(state.TonePhase) != "" ||
		strings.TrimSpace(state.RelationalStage) != "" ||
		strings.TrimSpace(state.StageDirection) != "" ||
		strings.TrimSpace(state.EmotionalTone) != "" ||
		strings.TrimSpace(state.InteractionMode) != "" ||
		strings.TrimSpace(state.FocusTopicKey) != "" ||
		strings.TrimSpace(state.OpenLoopSummary) != "" ||
		strings.TrimSpace(state.Confidence) != "" ||
		state.Revision != 0 ||
		state.SafetyLockUntilTurn != 0 ||
		state.LastSourceMessageID != 0 ||
		strings.TrimSpace(state.LastDecisionSource) != "" ||
		len(state.EvidenceJSON) > 0
}

func buildMemorySummaryText(sections MemoryPromptSections) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(sections.ActiveTopicsText) != "" {
		parts = append(parts, "[Active Topic Slots]\n"+strings.TrimSpace(sections.ActiveTopicsText))
	}
	if strings.TrimSpace(sections.ConversationStateMachineText) != "" {
		parts = append(parts, "[Conversation State Machine]\n"+strings.TrimSpace(sections.ConversationStateMachineText))
	}
	if strings.TrimSpace(sections.ConversationStateText) != "" {
		parts = append(parts, "[Conversation State]\n"+strings.TrimSpace(sections.ConversationStateText))
	}
	if strings.TrimSpace(sections.OpenLoopsText) != "" {
		parts = append(parts, "[Open Loops]\n"+strings.TrimSpace(sections.OpenLoopsText))
	}
	return strings.Join(parts, "\n\n")
}

func buildConversationStateMachineLines(state model.ConversationStateMachine) []string {
	if !shouldInjectConversationStateMachine(state) {
		return nil
	}

	lines := make([]string, 0, 10)
	if value := strings.TrimSpace(state.TonePhase); value != "" {
		lines = append(lines, "tone_phase="+value)
	}
	if value := strings.TrimSpace(state.RelationalStage); value != "" {
		lines = append(lines, "relational_stage="+value)
	}
	if value := strings.TrimSpace(state.StageDirection); value != "" {
		lines = append(lines, "stage_direction="+value)
	}
	if value := strings.TrimSpace(state.EmotionalTone); value != "" {
		lines = append(lines, "emotional_tone="+value)
	}
	if value := strings.TrimSpace(state.InteractionMode); value != "" {
		lines = append(lines, "interaction_mode="+value)
	}
	if value := strings.TrimSpace(state.FocusTopicKey); value != "" {
		lines = append(lines, "focus_topic_key="+value)
	}
	if value := strings.TrimSpace(state.OpenLoopSummary); value != "" {
		lines = append(lines, "open_loop_summary="+value)
	}
	if state.SafetyLockUntilTurn != 0 {
		lines = append(lines, "safety_lock_until_turn="+strconv.FormatInt(int64(state.SafetyLockUntilTurn), 10))
	}
	if value := strings.TrimSpace(state.Confidence); value != "" {
		lines = append(lines, "confidence="+value)
	}
	if state.Revision != 0 {
		lines = append(lines, "revision="+strconv.FormatInt(state.Revision, 10))
	}
	if state.LastSourceMessageID != 0 {
		lines = append(lines, "last_source_message_id="+strconv.FormatInt(state.LastSourceMessageID, 10))
	}
	if value := strings.TrimSpace(state.LastDecisionSource); value != "" {
		lines = append(lines, "last_decision_source="+value)
	}
	if value := formatPromptTimeKV("updated_at", state.UpdatedAt); value != "" {
		lines = append(lines, value)
	}
	return lines
}

func buildTopicPromptLine(slot model.TopicSlot) string {
	line := strings.TrimSpace(slot.TopicLabel)
	if line == "" {
		return ""
	}
	if summary := strings.TrimSpace(slot.Summary); summary != "" {
		line += " | " + summary
	}
	if value := formatPromptTimeKV("last_seen_at", slot.LastSeenAt); value != "" {
		line += " | " + value
	}
	return line
}

func formatPromptTimeKV(label string, value time.Time) string {
	if strings.TrimSpace(label) == "" || value.IsZero() {
		return ""
	}
	return label + "=" + promptutil.FormatPromptTimestamp(value)
}

func mergeConversationStateSlot(existing model.ConversationStateSlot, analyzed model.AnalyzedConversationState, sourceMessageID int64, at time.Time) model.ConversationStateSlot {
	if at.IsZero() {
		at = time.Now().UTC()
	}

	next := existing
	if value := cleanMemoryText(analyzed.CurrentStage); value != "" {
		next.CurrentStage = normalizeCurrentStage(value)
	}
	if value := cleanMemoryText(analyzed.StageDirection); value != "" {
		next.StageDirection = normalizeStageDirection(value)
	}
	if value := cleanMemoryText(analyzed.EmotionalTone); value != "" {
		next.EmotionalTone = value
	}
	if value := cleanMemoryText(analyzed.InteractionMode); value != "" {
		next.InteractionMode = value
	}
	if value := cleanMemoryText(analyzed.OpenLoopSummary); value != "" {
		next.OpenLoopSummary = value
	}
	if value := topicKeyFromLabel(analyzed.FocusTopicLabel); value != "" {
		next.FocusTopicKey = value
	}

	next.Confidence = deriveConfidence(analyzed.Confidence, next.Confidence)
	if evidence := evidenceJSON(analyzed.EvidenceTexts); len(evidence) > 0 {
		next.EvidenceJSON = evidence
	}
	if sourceMessageID != 0 {
		next.LastSourceMessageID = sourceMessageID
	}
	if next.CreatedAt.IsZero() {
		next.CreatedAt = at
	}
	next.UpdatedAt = at
	return next
}

func deriveTopicStatus(analyzed model.AnalyzedTopicSlot, current string) string {
	status := normalizeTopicStatus(analyzed.Status)
	if status != "" {
		return status
	}

	confidence := normalizeConfidence(analyzed.Confidence)
	if analyzed.Importance >= 70 || confidence == model.ConfidenceHigh {
		return model.TopicSlotStatusActive
	}
	if current != "" {
		return normalizeTopicStatus(current)
	}
	return model.TopicSlotStatusWatch
}

func normalizeTopicStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case model.TopicSlotStatusActive:
		return model.TopicSlotStatusActive
	case model.TopicSlotStatusWatch:
		return model.TopicSlotStatusWatch
	case model.TopicSlotStatusResolved:
		return model.TopicSlotStatusResolved
	case model.TopicSlotStatusArchived:
		return model.TopicSlotStatusArchived
	default:
		return ""
	}
}

func normalizeCurrentStage(stage string) string {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "opener", "rapport", "flirting", "support", "conflict", "repair", "planning":
		return strings.ToLower(strings.TrimSpace(stage))
	default:
		return cleanMemoryText(stage)
	}
}

func normalizeStageDirection(direction string) string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "warming", "stable", "escalating", "cooling", "shifting":
		return strings.ToLower(strings.TrimSpace(direction))
	default:
		return cleanMemoryText(direction)
	}
}

func deriveConfidence(next string, current string) string {
	confidence := normalizeConfidence(next)
	if confidence != model.ConfidenceLow {
		return confidence
	}
	if current != "" {
		return normalizeConfidence(current)
	}
	return model.ConfidenceLow
}

func clampImportance(next int, current int) int {
	if next <= 0 {
		next = current
	}
	if next <= 0 {
		next = 50
	}
	if next > 100 {
		next = 100
	}
	return next
}

func enforceTopicSlotLimits(slots []model.TopicSlot) []model.TopicSlot {
	sort.SliceStable(slots, func(i, j int) bool {
		leftRank := topicStatusRank(slots[i].Status)
		rightRank := topicStatusRank(slots[j].Status)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if slots[i].Importance != slots[j].Importance {
			return slots[i].Importance > slots[j].Importance
		}
		return slots[i].LastSeenAt.After(slots[j].LastSeenAt)
	})

	activeCount := 0
	watchCount := 0
	for idx := range slots {
		switch slots[idx].Status {
		case model.TopicSlotStatusActive:
			activeCount++
			if activeCount > 5 {
				slots[idx].Status = model.TopicSlotStatusArchived
			}
		case model.TopicSlotStatusWatch:
			watchCount++
			if watchCount > 5 {
				slots[idx].Status = model.TopicSlotStatusArchived
			}
		}
	}
	return slots
}

func topicStatusRank(status string) int {
	switch status {
	case model.TopicSlotStatusActive:
		return 0
	case model.TopicSlotStatusWatch:
		return 1
	case model.TopicSlotStatusResolved:
		return 2
	default:
		return 3
	}
}

func evidenceJSON(texts []string) []byte {
	values := make([]string, 0, len(texts))
	seen := make(map[string]struct{}, len(texts))
	for _, text := range texts {
		cleaned := cleanMemoryText(text)
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		values = append(values, cleaned)
		if len(values) >= 3 {
			break
		}
	}
	if len(values) == 0 {
		return []byte("[]")
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return []byte("[]")
	}
	return payload
}

func cleanMemoryText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'`")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}
