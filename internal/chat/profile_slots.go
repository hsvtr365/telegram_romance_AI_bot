package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
	pgstore "github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
)

const (
	profileSlotName         = "name"
	profileSlotGender       = "gender"
	profileSlotAge          = "age"
	profileSlotJob          = "job"
	profileSlotCurrentFocus = "current_focus"
	profileSlotHobby        = "hobby"
	profileSlotLocation     = "location"
	profileSlotAffiliation  = "affiliation"
)

const profilePromptTurnCooldown = 8   // Same slot cooldown
const profilePromptGlobalCooldown = 4 // Any profile slot cooldown

var jobKeywords = []string{
	"개발자", "디자이너", "간호사", "교사", "강사", "마케터", "회사원", "직장인", "대학생", "대학원생", "학생", "프리랜서", "자영업", "서비스직", "연구원", "기획자", "공무원", "영업", "회계", "변호사", "의사",
}

type profilePromptContext struct {
	Enabled     bool
	TargetSlot  string
	Instruction string
}

type pendingSlotData struct {
	CandidateValue string    `json:"candidate"`
	Attempts       int       `json:"attempts"`
	LastAskedAt    time.Time `json:"last_asked_at"`
}

func parsePendingSlots(data []byte) map[string]pendingSlotData {
	res := make(map[string]pendingSlotData)
	if len(data) == 0 || string(data) == "{}" {
		return res
	}
	_ = json.Unmarshal(data, &res)
	return res
}

func encodePendingSlots(slots map[string]pendingSlotData) []byte {
	if len(slots) == 0 {
		return []byte("{}")
	}
	res, _ := json.Marshal(slots)
	return res
}

func hasProfileSlotUpdate(params pgstore.UpsertUserProfileParams) bool {
	return strings.TrimSpace(params.NameValue) != "" ||
		strings.TrimSpace(params.GenderValue) != "" ||
		strings.TrimSpace(params.AgeValue) != "" ||
		strings.TrimSpace(params.JobValue) != "" ||
		strings.TrimSpace(params.CurrentFocusValue) != "" ||
		strings.TrimSpace(params.HobbyValue) != "" ||
		strings.TrimSpace(params.LocationValue) != "" ||
		strings.TrimSpace(params.AffiliationValue) != ""
}

func buildProfilePromptContext(profile model.UserProfile, recentConversationLen int, userInput string, userTurnCount int, now time.Time) profilePromptContext {
	firstConversationLike := recentConversationLen == 0
	richContext := recentConversationLen >= 20

	if profile.CollectionPausedUntilTurn > 0 && userTurnCount < profile.CollectionPausedUntilTurn {
		return profilePromptContext{}
	}

	missingSlots := missingProfileSlots(profile)
	expiredSlot, expiredValue := firstExpiredMutableSlot(profile, now)

	if richContext && !firstConversationLike {
		return profilePromptContext{}
	}

	targetSlot := chooseProfileTargetSlot(profile, userInput, missingSlots, expiredSlot, userTurnCount)

	if targetSlot == "" {
		return profilePromptContext{}
	}

	var instruction strings.Builder
	instruction.WriteString("[Profile Collection Guidance]\n")
	instruction.WriteString("지금은 사용자 정보를 자연스럽게 알아가는 흐름으로 반응한다.\n")
	instruction.WriteString("한 번에 하나의 슬롯만 노리고, 질문처럼 캐묻지 말고 흐름에 가볍게 섞는다.\n")
	instruction.WriteString("이미 저장된 정보만 사실처럼 쓰고, 없는 정보는 아는 척하지 않는다.\n")
	if len(missingSlots) > 0 {
		instruction.WriteString(fmt.Sprintf("비어 있는 슬롯: %s\n", strings.Join(missingSlots, ", ")))
	}
	if expiredSlot != "" && expiredValue != "" {
		instruction.WriteString(fmt.Sprintf("오래된 정보 재확인 후보: %s=%s\n", expiredSlot, expiredValue))
	}
	instruction.WriteString(fmt.Sprintf("이번 턴 목표 슬롯: %s\n", targetSlot))

	instruction.WriteString(profileSlotInstruction(targetSlot, profile, expiredSlot, expiredValue))

	return profilePromptContext{
		Enabled:     true,
		TargetSlot:  targetSlot,
		Instruction: strings.TrimSpace(instruction.String()),
	}
}

func missingProfileSlots(profile model.UserProfile) []string {
	slots := []struct {
		name  string
		value string
	}{
		{name: profileSlotName, value: profile.NameValue},
		{name: profileSlotGender, value: profile.GenderValue},
		{name: profileSlotAge, value: profile.AgeValue},
		{name: profileSlotJob, value: profile.JobValue},
		{name: profileSlotCurrentFocus, value: profile.CurrentFocusValue},
		{name: profileSlotHobby, value: profile.HobbyValue},
		{name: profileSlotAffiliation, value: profile.AffiliationValue},
		{name: profileSlotLocation, value: profile.LocationValue},
	}

	missing := make([]string, 0, len(slots))
	for _, slot := range slots {
		if normalizedProfileValue(slot.value) == "" {
			missing = append(missing, slot.name)
		}
	}
	return missing
}

func firstExpiredMutableSlot(profile model.UserProfile, now time.Time) (string, string) {
	type slotExpiry struct {
		name        string
		value       string
		confirmedAt time.Time
		maxAge      time.Duration
	}

	candidates := []slotExpiry{
		{name: profileSlotCurrentFocus, value: profile.CurrentFocusValue, confirmedAt: profile.CurrentFocusConfirmedAt, maxAge: 14 * 24 * time.Hour},
		{name: profileSlotHobby, value: profile.HobbyValue, confirmedAt: profile.HobbyConfirmedAt, maxAge: 45 * 24 * time.Hour},
		{name: profileSlotJob, value: profile.JobValue, confirmedAt: profile.JobConfirmedAt, maxAge: 60 * 24 * time.Hour},
		{name: profileSlotAffiliation, value: profile.AffiliationValue, confirmedAt: profile.AffiliationConfirmedAt, maxAge: 60 * 24 * time.Hour},
		{name: profileSlotLocation, value: profile.LocationValue, confirmedAt: profile.LocationConfirmedAt, maxAge: 90 * 24 * time.Hour},
	}

	for _, candidate := range candidates {
		candidate.value = normalizedProfileValue(candidate.value)
		if candidate.value == "" || candidate.confirmedAt.IsZero() {
			continue
		}
		if now.Sub(candidate.confirmedAt) > candidate.maxAge {
			return candidate.name, candidate.value
		}
	}

	return "", ""
}

func chooseProfileTargetSlot(profile model.UserProfile, userInput string, missingSlots []string, expiredSlot string, userTurnCount int) string {
	keywordSlot := detectTopicPreferredSlot(userInput)
	if keywordSlot != "" && slotEligibleForPrompt(profile, keywordSlot, userTurnCount) {
		if slotValue(profile, keywordSlot) == "" || keywordSlot == expiredSlot {
			return keywordSlot
		}
	}

	priority := []string{
		profileSlotName,
		profileSlotGender,
		profileSlotAge,
		profileSlotJob,
		profileSlotCurrentFocus,
		profileSlotHobby,
		profileSlotAffiliation,
		profileSlotLocation,
	}

	for _, slot := range priority {
		if containsString(missingSlots, slot) && slotEligibleForPrompt(profile, slot, userTurnCount) {
			return slot
		}
	}

	if expiredSlot != "" && slotEligibleForPrompt(profile, expiredSlot, userTurnCount) {
		return expiredSlot
	}

	return ""
}

func slotEligibleForPrompt(profile model.UserProfile, slot string, userTurnCount int) bool {
	if slot == "" {
		return false
	}
	// Global cooldown: don't ask anything if we asked something recently
	if profile.LastRequestedUserTurnCount > 0 && userTurnCount-profile.LastRequestedUserTurnCount < profilePromptGlobalCooldown {
		// Exception: allow asking gender immediately after name
		if slot == profileSlotGender && profile.LastRequestedSlot == profileSlotName {
			return true
		}
		return false
	}
	// Per-slot cooldown: don't ask the same thing too soon
	if profile.LastRequestedSlot == slot && userTurnCount-profile.LastRequestedUserTurnCount < profilePromptTurnCooldown {
		return false
	}
	return true
}

func detectTopicPreferredSlot(input string) string {
	normalized := normalizeInput(input)
	switch {
	case hasAnyKeyword(normalized, "남자", "여자", "남성", "여성", "성별"):
		return profileSlotGender
	case hasAnyKeyword(normalized, "회사", "일", "출근", "업무", "알바", "퇴근", "프로젝트", "직장"):
		return profileSlotJob
	case hasAnyKeyword(normalized, "요즘", "준비", "공부", "바빠", "연습", "작업"):
		return profileSlotCurrentFocus
	case hasAnyKeyword(normalized, "주말", "운동", "게임", "영화", "음악", "헬스", "산책", "취미"):
		return profileSlotHobby
	case hasAnyKeyword(normalized, "학교", "대학", "회사원", "대학생", "직장인", "동아리"):
		return profileSlotAffiliation
	case hasAnyKeyword(normalized, "서울", "부산", "수원", "인천", "동네", "자취", "집"):
		return profileSlotLocation
	default:
		return ""
	}
}

func profileSlotInstruction(targetSlot string, profile model.UserProfile, expiredSlot string, expiredValue string) string {
	switch targetSlot {
	case profileSlotName:
		return "아직 이름을 모른다. 인사 흐름 안에서 이름을 자연스럽게 알 수 있게 한 번만 가볍게 묻는다."
	case profileSlotGender:
		return "아직 성별을 모른다. 필요할 때만 짧고 부담 없게 확인한다. 캐묻거나 단정하지 않는다."
	case profileSlotAge:
		return "아직 나이를 모른다. 어색한 조사처럼 보이지 않게 자연스럽게 연령대를 알 수 있게 유도한다."
	case profileSlotJob:
		if targetSlot == expiredSlot && expiredValue != "" {
			return fmt.Sprintf("예전에 직업/역할 정보를 %q로 알고 있었다. 요즘도 그대로인지 짧게 확인한다.", expiredValue)
		}
		return "직업이나 주된 역할을 아직 모른다. 회사나 일상 얘기 속에 섞어서 알아본다."
	case profileSlotCurrentFocus:
		if targetSlot == expiredSlot && expiredValue != "" {
			return fmt.Sprintf("예전에 요즘 하는 일을 %q로 알고 있었다. 아직도 그 일로 바쁜지 가볍게 확인한다.", expiredValue)
		}
		return "요즘 뭘 하고 지내는지 아직 모른다. 최근 일상이나 집중하는 일을 자연스럽게 끌어낸다."
	case profileSlotHobby:
		if targetSlot == expiredSlot && expiredValue != "" {
			return fmt.Sprintf("예전에 취미를 %q로 알고 있었다. 요즘도 그걸 하는지 자연스럽게 확인한다.", expiredValue)
		}
		return "취미를 아직 모른다. 주말이나 쉬는 시간 얘기 속에 취미를 자연스럽게 물어본다."
	case profileSlotAffiliation:
		if targetSlot == expiredSlot && expiredValue != "" {
			return fmt.Sprintf("예전에 소속 정보를 %q로 알고 있었다. 아직도 같은 곳인지 짧게 확인한다.", expiredValue)
		}
		return "학교나 회사 같은 소속을 아직 모른다. 현재 속한 곳을 부드럽게 알아본다."
	case profileSlotLocation:
		if targetSlot == expiredSlot && expiredValue != "" {
			return fmt.Sprintf("예전에 생활권을 %q로 알고 있었다. 아직도 그쪽에서 지내는지 자연스럽게 확인한다.", expiredValue)
		}
		return "생활권이나 사는 지역을 아직 모른다. 동네나 이동 얘기 속에서 알아본다."
	default:
		return ""
	}
}

func slotValue(profile model.UserProfile, slot string) string {
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

func isPositiveAffirmation(input string) bool {
	normalized := strings.Trim(normalizeInput(input), " .!?")
	positives := []string{"응", "어", "웅", "맞아", "마자", "예스", "예", "그래", "조아", "좋아", "그럼", "그러면", "그르지", "마조"}
	for _, p := range positives {
		if normalized == p {
			return true
		}
	}
	return false
}

func cleanSlotValue(value string) string {
	cleaned := strings.TrimSpace(value)
	cleaned = strings.Trim(cleaned, ".,!? ")
	for _, suffix := range []string{"라고 불러", "라고 해", "이라구", "이야", "이에요", "예요", "야", "입니다", "은", "는", "이", "가"} {
		cleaned = strings.TrimSpace(strings.TrimSuffix(cleaned, suffix))
	}
	if isUnknownProfileValue(cleaned) {
		return ""
	}
	if len([]rune(cleaned)) < 1 {
		return ""
	}
	return cleaned
}

func normalizedProfileValue(value string) string {
	cleaned := strings.TrimSpace(value)
	if isUnknownProfileValue(cleaned) {
		return ""
	}
	return cleaned
}

func isUnknownProfileValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "미정", "모름", "모르겠음", "모르겠어", "알수없음", "알 수 없음", "없음", "없어", "none", "null", "unknown", "unk", "n/a", "na", "tbd":
		return true
	default:
		return false
	}
}

func normalizeInput(input string) string {
	return strings.TrimSpace(strings.ReplaceAll(input, "\n", " "))
}

func hasAnyKeyword(input string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(input, keyword) {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func looksLikeNonName(value string) bool {
	if hasAnyKeyword(value, jobKeywords...) {
		return true
	}
	return hasAnyKeyword(value,
		"학생", "회사원", "직장인", "프리랜서", "자영업", "백수",
		"피곤", "졸려", "심심", "바빠", "우울", "화나",
		"너", "나", "우리", "그", "그녀", "자기", "오빠", "언니", "누나", "형", "동생",
		"친구", "동료", "상사", "부장", "과장", "대리", "사원", "선생님", "교수님", "사장님",
		"고객", "손님", "자기야", "여보", "당신", "그대", "사람", "이름", "인간", "아저씨",
		"아줌마", "할아버지", "할머니", "행복", "기쁨", "슬픔", "사랑", "미움", "짜증",
	)
}

func detectProfilePromptDiscomfort(input string) bool {
	normalized := normalizeInput(input)
	return hasAnyKeyword(
		normalized,
		"부담", "과해", "너무 많이 물어", "별걸 다 물어", "캐묻", "너무 빠르", "천천히", "아직 대화도 별로",
	)
}
