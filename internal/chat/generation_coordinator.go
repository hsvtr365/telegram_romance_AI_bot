package chat

import (
	"context"
	"sync"
)

const (
	interruptReasonNone          = "none"
	interruptReasonNewInput      = "new_input"
	interruptReasonSafetyRestate = "safety_restate"
	interruptReasonReset         = "reset"
)

type GenerationHandle struct {
	ChatID                int64
	SessionID             int64
	SourceMessageID       int64
	GenerationID          int64
	StateRevisionAtStart  int64
	InterruptReason       string
	PreSendEligible       bool
	PartsSent             int
	RegenCount            int
	cancel                context.CancelFunc
}

type GenerationCoordinator struct {
	mu      sync.Mutex
	nextID  int64
	handles map[int64]*GenerationHandle
}

func NewGenerationCoordinator() *GenerationCoordinator {
	return &GenerationCoordinator{
		handles: make(map[int64]*GenerationHandle),
	}
}

func (c *GenerationCoordinator) CancelActive(chatID int64, reason string) {
	if c == nil || chatID == 0 {
		return
	}

	c.mu.Lock()
	handle := c.handles[chatID]
	if handle != nil && reason != "" {
		handle.InterruptReason = reason
	}
	cancel := cancelFunc(handle)
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (c *GenerationCoordinator) Begin(parent context.Context, chatID int64, sessionID int64, sourceMessageID int64, stateRevision int64, regenCount int) (*GenerationHandle, context.Context) {
	if c == nil {
		ctx, cancel := context.WithCancel(parent)
		return &GenerationHandle{
			ChatID:               chatID,
			SessionID:            sessionID,
			SourceMessageID:      sourceMessageID,
			StateRevisionAtStart: stateRevision,
			PreSendEligible:      true,
			InterruptReason:      interruptReasonNone,
			RegenCount:           regenCount,
			cancel:               cancel,
		}, ctx
	}

	ctx, cancel := context.WithCancel(parent)

	c.mu.Lock()
	c.nextID++
	handle := &GenerationHandle{
		ChatID:               chatID,
		SessionID:            sessionID,
		SourceMessageID:      sourceMessageID,
		GenerationID:         c.nextID,
		StateRevisionAtStart: stateRevision,
		InterruptReason:      interruptReasonNone,
		PreSendEligible:      true,
		RegenCount:           regenCount,
		cancel:               cancel,
	}
	c.handles[chatID] = handle
	c.mu.Unlock()

	return handle, ctx
}

type BeforeSendDecision struct {
	Allow      bool
	Regenerate bool
	Reason     string
}

func (c *GenerationCoordinator) BeforeSend(chatID int64, generationID int64) BeforeSendDecision {
	if c == nil {
		return BeforeSendDecision{Allow: true}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	handle := c.handles[chatID]
	if handle == nil || handle.GenerationID != generationID {
		return BeforeSendDecision{Allow: false, Reason: interruptReasonNewInput}
	}

	switch handle.InterruptReason {
	case interruptReasonSafetyRestate:
		if handle.PreSendEligible && handle.PartsSent == 0 && handle.RegenCount < 1 {
			return BeforeSendDecision{Allow: false, Regenerate: true, Reason: interruptReasonSafetyRestate}
		}
		return BeforeSendDecision{Allow: false, Reason: interruptReasonSafetyRestate}
	case interruptReasonNewInput:
		return BeforeSendDecision{Allow: false, Reason: interruptReasonNewInput}
	default:
		return BeforeSendDecision{Allow: true}
	}
}

func (c *GenerationCoordinator) MarkPartSent(chatID int64, generationID int64) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	handle := c.handles[chatID]
	if handle == nil || handle.GenerationID != generationID {
		return
	}
	handle.PartsSent++
	handle.PreSendEligible = false
}

func (c *GenerationCoordinator) Finish(chatID int64, generationID int64) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	handle := c.handles[chatID]
	if handle == nil || handle.GenerationID != generationID {
		return
	}
	delete(c.handles, chatID)
}

func (c *GenerationCoordinator) RequestSafetyRestate(chatID int64, sessionID int64, newRevision int64) bool {
	if c == nil || chatID == 0 {
		return false
	}

	c.mu.Lock()
	handle := c.handles[chatID]
	if handle == nil || handle.SessionID != sessionID || !handle.PreSendEligible || handle.PartsSent > 0 || handle.RegenCount >= 1 {
		c.mu.Unlock()
		return false
	}
	if newRevision <= handle.StateRevisionAtStart {
		c.mu.Unlock()
		return false
	}

	handle.InterruptReason = interruptReasonSafetyRestate
	cancel := handle.cancel
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return true
}

func (c *GenerationCoordinator) InterruptReason(chatID int64, generationID int64) string {
	if c == nil {
		return interruptReasonNone
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	handle := c.handles[chatID]
	if handle == nil || handle.GenerationID != generationID {
		return interruptReasonNewInput
	}
	return handle.InterruptReason
}

func cancelFunc(handle *GenerationHandle) context.CancelFunc {
	if handle == nil {
		return nil
	}
	return handle.cancel
}
