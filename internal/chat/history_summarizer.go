package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store"
)

type HistorySummarizer struct {
	llm   LLM
	store *store.Manager
}

func NewHistorySummarizer(llm LLM, conversationStore *store.Manager) *HistorySummarizer {
	return &HistorySummarizer{
		llm:   llm,
		store: conversationStore,
	}
}

func (s *Service) updateHistorySummaryAsync(sessionID int64, recentLimit int) {
	if s.store == nil || s.llm == nil || sessionID == 0 {
		return
	}

	// We use the same async runner or a dedicated one? 
	// Let's use the structuredRunner or memoryAnalyzer's runner for now if appropriate, 
	// but those are keyed. Let's just use a goroutine or a dedicated task in structuredRunner.
	
	s.analyticRunner.Enqueue(AsyncTask{
		Key:     fmt.Sprintf("history_summary:%d", sessionID),
		Version: time.Now().Unix(), // Always run if triggered
		Build: func(ctx context.Context) (func(context.Context) error, error) {
			oldMessages, err := s.store.ListOldMessages(ctx, sessionID, recentLimit, 200)
			if err != nil {
				return nil, err
			}

			if len(oldMessages) < 5 { // Not enough to summarize
				return nil, nil 
			}

			var sb strings.Builder
			for _, m := range oldMessages {
				sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
			}

			prompt := []ollama.Message{
				{
					Role: "system",
					Content: "너는 대화 내용을 압축 요약하는 서브 에이전트다. 제공된 대화 기록을 3줄 이내의 짧고 명확한 문장으로 요약하라. 핵심 사실과 관계의 전개 상황 위주로 서술하며, 서술어는 '~함', '~임' 식의 명사형 종결이나 짧고 건조한 말투를 사용하라.",
				},
				{
					Role:    "user",
					Content: "다음 대화 내용을 3줄 이내로 요약해:\n\n" + sb.String(),
				},
			}

			summary, err := s.llm.Chat(ctx, prompt)
			if err != nil {
				return nil, err
			}

			summary = strings.TrimSpace(summary)
			if summary == "" {
				return nil, nil
			}

			return func(commitCtx context.Context) error {
				return s.store.UpdateSessionHistorySummary(commitCtx, sessionID, summary)
			}, nil
		},
	})
}
