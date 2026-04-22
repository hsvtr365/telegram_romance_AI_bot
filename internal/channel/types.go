package channel

import (
	"context"
	"strconv"
	"strings"
	"time"
)

const (
	Telegram = "telegram"
	Discord  = "discord"

	ChatTypePrivate = "private"
	ChatTypeDM      = "dm"
	ChatTypeMention = "mention"

	ActionTyping = "typing"
)

type InboundMessage struct {
	BotID             string
	Channel           string
	ExternalUserID    string
	ExternalChatID    string
	ExternalMessageID string
	ExternalUpdateID  string
	ChatType          string
	Text              string
	Username          string
	FirstName         string
	SentAt            time.Time
}

func (m InboundMessage) Target() OutboundTarget {
	return OutboundTarget{
		BotID:          m.BotID,
		Channel:        m.Channel,
		ExternalChatID: m.ExternalChatID,
	}
}

func (m InboundMessage) LockKey() string {
	return strings.Join([]string{
		strings.TrimSpace(m.BotID),
		strings.TrimSpace(m.Channel),
		strings.TrimSpace(m.ExternalChatID),
	}, ":")
}

func (m InboundMessage) MessageIDInt64() int64 {
	return parseInt64(m.ExternalMessageID)
}

func (m InboundMessage) UpdateIDInt64() int64 {
	return parseInt64(m.ExternalUpdateID)
}

func (m InboundMessage) UserIDInt64() int64 {
	return parseInt64(m.ExternalUserID)
}

func (m InboundMessage) ChatIDInt64() int64 {
	return parseInt64(m.ExternalChatID)
}

type OutboundTarget struct {
	BotID          string
	Channel        string
	ExternalChatID string
}

func (t OutboundTarget) Key() string {
	return strings.Join([]string{
		strings.TrimSpace(t.BotID),
		strings.TrimSpace(t.Channel),
		strings.TrimSpace(t.ExternalChatID),
	}, ":")
}

type Messenger interface {
	SendText(ctx context.Context, target OutboundTarget, text string) error
	SendTyping(ctx context.Context, target OutboundTarget) error
	SendAudio(ctx context.Context, target OutboundTarget, audio AudioAttachment) error
}

type AudioAttachment struct {
	Data     []byte
	MIMEType string
	FileName string
	Caption  string
}

type MessageHandler interface {
	HandleMessage(ctx context.Context, message InboundMessage) error
}

type MessageHandlerFunc func(ctx context.Context, message InboundMessage) error

func (f MessageHandlerFunc) HandleMessage(ctx context.Context, message InboundMessage) error {
	return f(ctx, message)
}

type Runner interface {
	Run(ctx context.Context) error
}

func parseInt64(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
