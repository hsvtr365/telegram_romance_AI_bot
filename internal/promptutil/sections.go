package promptutil

import (
	"fmt"
	"strings"
)

type MessageLine struct {
	Role    string
	Content string
}

func WriteSection(builder *strings.Builder, title string, body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}

	if strings.TrimSpace(title) != "" {
		builder.WriteString("[")
		builder.WriteString(strings.TrimSpace(title))
		builder.WriteString("]\n")
	}
	builder.WriteString(body)
	builder.WriteString("\n\n")
	return true
}

func WriteLinesSection(builder *strings.Builder, title string, lines []string) bool {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	if len(filtered) == 0 {
		return false
	}
	return WriteSection(builder, title, strings.Join(filtered, "\n"))
}

func WriteConversation(builder *strings.Builder, title string, messages []MessageLine) bool {
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, content))
	}
	return WriteLinesSection(builder, title, lines)
}

func WriteRawBlock(builder *strings.Builder, body string) bool {
	return WriteSection(builder, "", body)
}
