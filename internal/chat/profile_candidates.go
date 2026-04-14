package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

const (
	profileBatchReviewInterval    = 5
	profileBatchReviewWindowTurns = 15
	profileBatchReviewTimeout     = 300 * time.Second
	profileBatchReviewWorkers     = 1
	profileBatchReviewQueueSize   = 16
)

type extractedProfileCandidate struct {
	SlotName        string
	CandidateValue  string
	NormalizedValue string
	EvidenceText    string
	EvidenceType    string
	Confidence      string
}

func extractProfileCandidatesFromStructuredProfile(profile structuredProfile, input string, history []model.Message) []extractedProfileCandidate {
	candidates := make([]extractedProfileCandidate, 0, 8)

	push := func(slot string, field structuredField) {
		if candidate, ok := profileCandidateFromStructuredField(slot, field, input, history); ok {
			candidates = append(candidates, candidate)
		}
	}

	push(profileSlotName, profile.Name)
	push(profileSlotGender, profile.Gender)
	push(profileSlotAge, profile.Age)
	push(profileSlotJob, profile.Job)
	push(profileSlotCurrentFocus, profile.CurrentFocus)
	push(profileSlotHobby, profile.Hobby)
	push(profileSlotLocation, profile.Location)
	push(profileSlotAffiliation, profile.Affiliation)

	return candidates
}

func profileCandidateFromStructuredField(slot string, field structuredField, input string, history []model.Message) (extractedProfileCandidate, bool) {
	value := normalizeStructuredProfileValue(slot, field)
	if value == "" {
		return extractedProfileCandidate{}, false
	}

	evidenceType := normalizeStructuredEvidenceType(field.EvidenceType)
	if evidenceType == "none" || evidenceType == "" {
		// FALLBACK: If value is explicitly found in current input, treat as explicit
		if value != "" && hasStructuredEvidenceText(input, value) {
			evidenceType = "explicit"
		} else {
			return extractedProfileCandidate{}, false
		}
	}

	evidenceText := strings.TrimSpace(field.EvidenceText)
	evidenceFound := false
	if evidenceText != "" && hasStructuredEvidenceTextInContext(input, history, evidenceText) {
		evidenceFound = true
	}
	// Fallback to searching for the value itself if explicit evidence text was not found
	if !evidenceFound && value != "" && hasStructuredEvidenceTextInContext(input, history, value) {
		evidenceText = value
		evidenceFound = true
	}

	if !evidenceFound {
		return extractedProfileCandidate{}, false
	}

	return extractedProfileCandidate{
		SlotName:        slot,
		CandidateValue:  value,
		NormalizedValue: normalizeProfileCandidateValue(value),
		EvidenceText:    evidenceText,
		EvidenceType:    evidenceType,
		Confidence:      normalizeConfidence(field.Confidence),
	}, true
}

func hasStructuredEvidenceTextInContext(input string, history []model.Message, evidenceText string) bool {
	// 1. Check current input
	if hasStructuredEvidenceText(input, evidenceText) {
		return true
	}

	// 2. Check history
	var combined strings.Builder
	for _, m := range history {
		if m.Role == "user" {
			if hasStructuredEvidenceText(m.Content, evidenceText) {
				return true
			}
			combined.WriteString(m.Content)
			combined.WriteString(" ")
		}
	}

	// 3. Check combined context
	combined.WriteString(input)
	if hasStructuredEvidenceText(combined.String(), evidenceText) {
		return true
	}

	return false
}

func hasStructuredEvidenceText(input string, evidenceText string) bool {
	input = normalizeInput(input)
	evidenceText = normalizeInput(evidenceText)
	if input == "" || evidenceText == "" {
		return false
	}
	
	inNoSpace := strings.ReplaceAll(input, " ", "")
	evNoSpace := strings.ReplaceAll(evidenceText, " ", "")
	return strings.Contains(inNoSpace, evNoSpace)
}

func normalizeStructuredEvidenceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "explicit":
		return "explicit"
	case "tentative":
		return "tentative"
	case "inferred":
		return "inferred"
	default:
		return "none"
	}
}

func normalizeProfileCandidateValue(value string) string {
	value = normalizedProfileValue(value)
	if value == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func orderedProfileSlots() []string {
	return []string{
		profileSlotName,
		profileSlotGender,
		profileSlotAge,
		profileSlotJob,
		profileSlotLocation,
		profileSlotAffiliation,
		profileSlotHobby,
		profileSlotCurrentFocus,
	}
}

func profileSlotLabel(slot string) string {
	switch slot {
	case profileSlotName:
		return "이름"
	case profileSlotGender:
		return "성별"
	case profileSlotAge:
		return "나이"
	case profileSlotJob:
		return "직업"
	case profileSlotLocation:
		return "거주지"
	case profileSlotAffiliation:
		return "소속"
	case profileSlotHobby:
		return "취미"
	case profileSlotCurrentFocus:
		return "현재 관심사"
	default:
		return slot
	}
}

func confirmedProfileValueBySlot(profile model.UserProfile, slot string) string {
	switch slot {
	case profileSlotName:
		return normalizedProfileValue(profile.NameValue)
	case profileSlotGender:
		return normalizedProfileValue(profile.GenderValue)
	case profileSlotAge:
		return normalizedProfileValue(profile.AgeValue)
	case profileSlotJob:
		return normalizedProfileValue(profile.JobValue)
	case profileSlotCurrentFocus:
		return normalizedProfileValue(profile.CurrentFocusValue)
	case profileSlotHobby:
		return normalizedProfileValue(profile.HobbyValue)
	case profileSlotLocation:
		return normalizedProfileValue(profile.LocationValue)
	case profileSlotAffiliation:
		return normalizedProfileValue(profile.AffiliationValue)
	default:
		return ""
	}
}

func topProfileCandidateBySlot(candidates []model.ProfileCandidate) map[string]model.ProfileCandidate {
	res := make(map[string]model.ProfileCandidate, len(candidates))
	for _, candidate := range candidates {
		if candidate.Status != model.ProfileCandidateStatusActive {
			continue
		}
		if normalizedProfileValue(candidate.CandidateValue) == "" {
			continue
		}
		res[candidate.SlotName] = candidate
	}
	return res
}

func candidateProfileSummary(profile model.UserProfile, candidates []model.ProfileCandidate) string {
	if len(candidates) == 0 {
		return ""
	}

	top := topProfileCandidateBySlot(candidates)
	lines := make([]string, 0, len(top))
	for _, slot := range orderedProfileSlots() {
		candidate, ok := top[slot]
		if !ok {
			continue
		}
		value := normalizedProfileValue(candidate.CandidateValue)
		if value == "" {
			continue
		}
		if normalizeProfileCandidateValue(confirmedProfileValueBySlot(profile, slot)) == candidate.NormalizedValue {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s=%s (후보)", slot, value))
	}
	return strings.Join(lines, "\n")
}

func renderProfileFieldWithCandidate(profile model.UserProfile, candidatesBySlot map[string]model.ProfileCandidate, slot string) string {
	confirmed := confirmedProfileValueBySlot(profile, slot)
	candidate, hasCandidate := candidatesBySlot[slot]
	candidateValue := ""
	if hasCandidate {
		candidateValue = normalizedProfileValue(candidate.CandidateValue)
		if normalizeProfileCandidateValue(confirmed) == candidate.NormalizedValue {
			candidateValue = ""
		}
	}

	switch {
	case confirmed != "" && candidateValue != "":
		return fmt.Sprintf("%s [확정] / %s [후보]", confirmed, candidateValue)
	case confirmed != "":
		return confirmed + " [확정]"
	case candidateValue != "":
		return candidateValue + " [후보]"
	default:
		return "(알 수 없음)"
	}
}
