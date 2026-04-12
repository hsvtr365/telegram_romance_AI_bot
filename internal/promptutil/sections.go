package promptutil

import (
	"fmt"
	"strings"
	"time"
)

type MessageLine struct {
	Role      string
	Content   string
	CreatedAt time.Time
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
	if len(messages) == 0 {
		return false
	}

	merged := make([]MessageLine, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}

		// Preserve per-turn timing when timestamps are present.
		if len(merged) > 0 && merged[len(merged)-1].Role == role && merged[len(merged)-1].CreatedAt.IsZero() && msg.CreatedAt.IsZero() {
			merged[len(merged)-1].Content += "\n" + content
		} else {
			merged = append(merged, MessageLine{Role: role, Content: content, CreatedAt: msg.CreatedAt})
		}
	}

	lines := make([]string, 0, len(merged))
	for _, msg := range merged {
		if msg.CreatedAt.IsZero() {
			lines = append(lines, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", FormatPromptTimestamp(msg.CreatedAt), msg.Role, msg.Content))
	}
	return WriteLinesSection(builder, title, lines)
}

func WriteRawBlock(builder *strings.Builder, body string) bool {
	return WriteSection(builder, "", body)
}
