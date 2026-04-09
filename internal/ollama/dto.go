package ollama

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model     string      `json:"model"`
	Messages  []Message   `json:"messages"`
	Stream    bool        `json:"stream"`
	KeepAlive string      `json:"keep_alive,omitempty"`
	Options   ChatOptions `json:"options"`
}

type ChatOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	NumCtx      int     `json:"num_ctx,omitempty"`
}

type ChatResponse struct {
	Model   string  `json:"model"`
	Message Message `json:"message"`
	Done    bool    `json:"done"`
}
