package telegram

import (
	"testing"
	"time"

	channelx "github.com/hsvtr365/telegram_romance_AI_bot/internal/channel"
)

func TestInboundFromUpdateMapsTelegramMessage(t *testing.T) {
	msg, ok := inboundFromUpdate("lee-jinhyuk", Update{
		UpdateID: 42,
		Message: &Message{
			MessageID: 77,
			Date:      1713763200,
			From: &User{
				ID:        1001,
				FirstName: "수지",
				Username:  "suji",
			},
			Chat: Chat{
				ID:   2002,
				Type: "private",
			},
			Text: "  해냈다  ",
		},
	})
	if !ok {
		t.Fatal("expected update to map")
	}

	if msg.BotID != "lee-jinhyuk" || msg.Channel != channelx.Telegram {
		t.Fatalf("unexpected bot/channel: %+v", msg)
	}
	if msg.ExternalUserID != "1001" || msg.ExternalChatID != "2002" || msg.ExternalMessageID != "77" || msg.ExternalUpdateID != "42" {
		t.Fatalf("unexpected external ids: %+v", msg)
	}
	if msg.Text != "해냈다" || msg.Username != "suji" || msg.FirstName != "수지" {
		t.Fatalf("unexpected message fields: %+v", msg)
	}
	if want := time.Unix(1713763200, 0).UTC(); !msg.SentAt.Equal(want) {
		t.Fatalf("unexpected sent_at: got %s want %s", msg.SentAt, want)
	}
}

func TestInboundFromUpdateRejectsNonPrivateOrEmptyMessages(t *testing.T) {
	if _, ok := inboundFromUpdate("bot", Update{Message: &Message{From: &User{ID: 1}, Chat: Chat{ID: 2, Type: "group"}, Text: "hi"}}); ok {
		t.Fatal("expected group message to be rejected")
	}
	if _, ok := inboundFromUpdate("bot", Update{Message: &Message{From: &User{ID: 1}, Chat: Chat{ID: 2, Type: "private"}}}); ok {
		t.Fatal("expected empty message to be rejected")
	}
}
