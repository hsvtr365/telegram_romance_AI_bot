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
					Role:    "system",
					Content: "너는 대화 기록에서 유저와 캐릭터에 대한 시간의 흐름을 초월하여 보존해야 할 물리적 사실(직업, 사는 곳, 과거의 주요 사건, 취향 등)만 수집하는 요약 서브 에이전트다. 대화 내용에서 오직 지속적이고 중요한 핵심 정보만 선택하여 3줄 이내의 객관적 목록으로 구성하십시오. 서술어는 '~함', '~임' 식의 명사형 종결이나 건조한 사실 기술 목록으로만 이루어지도록 작성하십시오.",
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
