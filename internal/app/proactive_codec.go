package app

import (
	"encoding/json"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/proactive"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/textutil"
)

func parseQuietWindows(raw []byte) []proactive.QuietWindow {
	return decodeJSON[[]proactive.QuietWindow](raw)
}

func parseTimeWindows(raw []byte) []proactive.TimeWindow {
	return decodeJSON[[]proactive.TimeWindow](raw)
}

func parseStringSlice(raw []byte) []string {
	return decodeJSON[[]string](raw)
}

func parseFloatMap(raw []byte) map[string]float64 {
	return decodeJSON[map[string]float64](raw)
}

func parseMap(raw []byte) map[string]any {
	return decodeJSON[map[string]any](raw)
}

func fallbackString(value string, fallback string) string {
	return textutil.FirstNonBlank(value, fallback)
}

func marshalJSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

func decodeJSON[T any](raw []byte) T {
	var value T
	if len(raw) == 0 {
		return value
	}
	_ = json.Unmarshal(raw, &value)
	return value
}
