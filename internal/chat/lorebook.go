package chat

import (
	"encoding/json"
	"os"
	"strings"
)

type LoreEntry struct {
	Keys    []string `json:"keys"`
	Content string   `json:"content"`
}

type Lorebook struct {
	Entries []LoreEntry `json:"entries"`
}

func LoadLorebook(path string) (*Lorebook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lb Lorebook
	if err := json.Unmarshal(data, &lb); err != nil {
		return nil, err
	}
	return &lb, nil
}

func (l *Lorebook) Match(input string) string {
	if l == nil || len(l.Entries) == 0 {
		return ""
	}
	normalizedInput := strings.ToLower(input)
	var matched []string
	seen := make(map[string]bool)

	for _, entry := range l.Entries {
		for _, key := range entry.Keys {
			normalizedKey := strings.ToLower(key)
			if strings.Contains(normalizedInput, normalizedKey) {
				if !seen[entry.Content] {
					seen[entry.Content] = true
					matched = append(matched, entry.Content)
				}
				break // Stop checking keys for this entry once matched
			}
		}
	}

	if len(matched) == 0 {
		return ""
	}
	return strings.Join(matched, "\n")
}
