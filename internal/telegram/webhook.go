package telegram

import "context"

// WebhookServer is a placeholder so the input adapter can be swapped later
// without changing the chat service contract.
type WebhookServer struct{}

func NewWebhookServer() *WebhookServer {
	return &WebhookServer{}
}

func (s *WebhookServer) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
