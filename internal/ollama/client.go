package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	BaseURL     string
	Model       string
	TimeoutSec  int
	KeepAlive   string
	NumCtx      int
	Temperature float64
	TopP        float64
}

type Client struct {
	baseURL     string
	model       string
	keepAlive   string
	numCtx      int
	temperature float64
	topP        float64
	httpClient  *http.Client
	logger      *slog.Logger
}

func NewClient(cfg Config, logger *slog.Logger) *Client {
	return &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		model:       cfg.Model,
		keepAlive:   cfg.KeepAlive,
		numCtx:      cfg.NumCtx,
		temperature: cfg.Temperature,
		topP:        cfg.TopP,
		httpClient: &http.Client{
			Timeout: normalizeTimeout(cfg.TimeoutSec),
		},
		logger: logger,
	}
}

func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	reqBody := ChatRequest{
		Model:     c.model,
		Messages:  messages,
		Stream:    false,
		KeepAlive: c.keepAlive,
		Options: ChatOptions{
			Temperature: c.temperature,
			TopP:        c.topP,
			NumCtx:      c.numCtx,
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ollama returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var response ChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}

	return strings.TrimSpace(response.Message.Content), nil
}

func (c *Client) CheckHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("ollama health returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func normalizeTimeout(timeoutSec int) time.Duration {
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		return 35 * time.Second
	}
	return timeout
}
