package model

type MemorySlotSnapshot struct {
	SessionID         int64
	RecentMessages    []Message
	TopicSlots        []TopicSlot
	ConversationState ConversationStateSlot
}

type MemorySlotAnalysisResult struct {
	Topics            []AnalyzedTopicSlot       `json:"topics"`
	ConversationState AnalyzedConversationState `json:"conversation_state"`
	ResolvedTopics    []string                  `json:"resolved_topics"`
}
