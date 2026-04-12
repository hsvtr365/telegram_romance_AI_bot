package promptutil

import "time"

var promptTimeLocation = time.FixedZone("KST", 9*60*60)

func PromptTimeLocation() *time.Location {
	return promptTimeLocation
}

func FormatPromptTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(promptTimeLocation).Format("2006-01-02 15:04 MST")
}

func FormatPromptCurrentTime(value time.Time) string {
	weekdays := []string{"일", "월", "화", "수", "목", "금", "토"}
	local := value.In(promptTimeLocation)
	return "지금 시각은 " + local.Format("2006-01-02") + "(" + weekdays[local.Weekday()] + ") " + local.Format("15:04 MST") + " 이다."
}
