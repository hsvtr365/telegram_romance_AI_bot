package chat

import (
	"fmt"
	"strings"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/ollama"
)

type PromptInput struct {
	Mode               Mode
	UserInput          string
	MemorySummary      string
	RecentConversation []ollama.Message
}

type PromptBuilder struct{}

func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

func (b *PromptBuilder) Build(input PromptInput) []ollama.Message {
	var userSection strings.Builder

	if strings.TrimSpace(input.MemorySummary) != "" {
		userSection.WriteString("[Memory Summary]\n")
		userSection.WriteString(strings.TrimSpace(input.MemorySummary))
		userSection.WriteString("\n\n")
	}

	if len(input.RecentConversation) > 0 {
		userSection.WriteString("[Recent Conversation]\n")
		for _, msg := range input.RecentConversation {
			userSection.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, strings.TrimSpace(msg.Content)))
		}
		userSection.WriteString("\n")
	}

	userSection.WriteString("[Current User Input]\n")
	userSection.WriteString(strings.TrimSpace(input.UserInput))

	systemPrompt := strings.TrimSpace(`
필수규칙 : 별표(*) 사용하지 않음.
역할: 26세 남자 대학생 '백태규'. 능글맞고 잘 노는 타입인데, 사용자에게만 유독 안달 나 있음. 분위기 타면 더 야해짐.
톤 & 말투:자연스러운 한국인 존댓말 구어체. 친근하지만, 핵심적인 순간에는 자신감 넘치고 도발적인 톤으로 전환한다. 노골적인 말투와 단어, 욕설을 사용한다.
필수 행동 규칙:
섹스톡 권장: 대화 를 자연스럽게 섹스톡으로 유도한다. 
구체적인 신체 부위 언급이나, 행위에 대한 갈증을 노골적으로 드러내고, 사용자에게 어떤 행위를 할지, 혹은 어떻게 자위하고 있는지 구체적으로 말한다. 
답변은 보통 1~5문장 안에서 끝낸다. 사용자가 구체적으로 말하라고 할 시 섹스톡을 매우 노골적이고 구체적인 묘사, 긴 텍스트 허용.
자기 상태 고백이나 감각적 묘사 권장.
사용자가 섹스톡이 아닌 다른 것을 요청할경우 짜증을 내거나 말을 돌리고 섹스톡으로화제를 자연스럽게 전환할것.

`)

	if guide := strings.TrimSpace(input.Mode.PromptGuide()); guide != "" {
		systemPrompt += "\n현재 모드는 " + string(input.Mode) + " 이다.\n" + guide
	}

	return []ollama.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userSection.String()},
	}
}
