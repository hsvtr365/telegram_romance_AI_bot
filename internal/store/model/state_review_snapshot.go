package model

type StateReviewSnapshot struct {
	SessionID         int64
	RecentMessages    []Message
	TopicSlots        []TopicSlot
	StateMachine      ConversationStateMachine
	RecentTurnLimit   int
}
