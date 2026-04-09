package chat

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func PostProcess(text string, _ int) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "assistant:")
	text = strings.TrimPrefix(text, "Assistant:")
	text = strings.TrimSpace(text)

	replacer := strings.NewReplacer(
		"AI로서", "",
		"정리하자면", "",
		"설명하자면", "",
		"다음과 같다", "",
	)
	text = strings.TrimSpace(replacer.Replace(text))

	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	var prev string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == prev {
			continue
		}
		cleaned = append(cleaned, line)
		prev = line
	}

	text = strings.Join(cleaned, "\n")

	return strings.TrimSpace(text)
}

func SplitReplyForTelegram(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	runes := []rune(text)
	parts := make([]string, 0, 4)
	var current strings.Builder

	flush := func() {
		part := strings.TrimSpace(current.String())
		part = strings.Trim(part, "\n")
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
		current.Reset()
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if r == '\n' {
			flush()
			continue
		}

		current.WriteRune(r)

		if !isSentenceBoundary(r) {
			continue
		}

		// Keep repeated punctuation and closing marks attached to the same sentence.
		for i+1 < len(runes) {
			next := runes[i+1]
			if isSentenceBoundary(next) || isClosingMark(next) {
				i++
				current.WriteRune(next)
				continue
			}
			break
		}

		// If the sentence ends with trailing emojis, keep them attached to the
		// sentence so Telegram does not send them as their own message bubble.
		nextIndex, emojiSuffix := collectTrailingEmojiSuffix(runes, i+1)
		if emojiSuffix != "" {
			current.WriteString(emojiSuffix)
			i = nextIndex - 1
		}

		flush()
	}

	flush()

	if len(parts) == 0 {
		return []string{text}
	}

	return mergeShortTelegramParts(parts, 10)
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '.', '!', '?', '~', '…':
		return true
	default:
		return false
	}
}

func isClosingMark(r rune) bool {
	switch r {
	case '"', '\'', ')', ']', '}', '”', '’':
		return true
	default:
		return false
	}
}

func collectTrailingEmojiSuffix(runes []rune, start int) (int, string) {
	i := start
	for i < len(runes) && isInlineSpace(runes[i]) {
		i++
	}

	if i >= len(runes) || !isEmojiLike(runes[i]) {
		return start, ""
	}

	var suffix strings.Builder
	if i > start {
		suffix.WriteRune(' ')
	}

	spacePending := false
	for i < len(runes) {
		r := runes[i]

		if r == '\n' {
			break
		}

		if isInlineSpace(r) {
			spacePending = true
			i++
			continue
		}

		if !isEmojiLike(r) && !isClosingMark(r) {
			break
		}

		if spacePending && suffix.Len() > 0 {
			suffix.WriteRune(' ')
		}
		spacePending = false
		suffix.WriteRune(r)
		i++
	}

	return i, strings.TrimRight(suffix.String(), " \t")
}

func mergeShortTelegramParts(parts []string, maxLen int) []string {
	if len(parts) == 0 {
		return nil
	}

	merged := make([]string, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		current := strings.TrimSpace(parts[i])
		if current == "" {
			continue
		}

		for utf8.RuneCountInString(current) <= maxLen && i+1 < len(parts) {
			i++
			current = joinTelegramParts(current, parts[i])
		}

		merged = append(merged, current)
	}

	return merged
}

func joinTelegramParts(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	if left == "" {
		return right
	}
	if right == "" {
		return left
	}

	if strings.HasSuffix(left, "\n") {
		return left + right
	}

	return left + " " + right
}

func isInlineSpace(r rune) bool {
	return r == ' ' || r == '\t'
}

func isEmojiLike(r rune) bool {
	if unicode.Is(unicode.Mn, r) {
		return true
	}

	switch {
	case r >= 0x1F300 && r <= 0x1FAFF:
		return true
	case r >= 0x2600 && r <= 0x27BF:
		return true
	case r >= 0x1F1E6 && r <= 0x1F1FF:
		return true
	case r == 0x200D:
		return true
	case r == 0xFE0E || r == 0xFE0F:
		return true
	default:
		return false
	}
}
