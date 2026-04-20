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
	if c.usesOpenAICompat() {
		return c.chatOpenAICompat(ctx, messages)
	}

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
	path := "/api/tags"
	providerLabel := "ollama"
	if c.usesOpenAICompat() {
		path = "/models"
		providerLabel = "openai-compatible"
	}

	url := c.endpointURL(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		return fmt.Errorf("%s health returned %d: %s", providerLabel, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func (c *Client) chatOpenAICompat(ctx context.Context, messages []Message) (string, error) {
	reqBody := OpenAIChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: c.temperature,
		TopP:        c.topP,
		Stream:      false,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpointURL("/chat/completions"), bytes.NewReader(payload))
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
		return "", fmt.Errorf("openai-compatible returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var response OpenAIChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode openai-compatible response: %w", err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("openai-compatible response missing choices")
	}

	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

func (c *Client) usesOpenAICompat() bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(c.baseURL)), "/v1")
}

func (c *Client) endpointURL(path string) string {
	base := strings.TrimRight(strings.TrimSpace(c.baseURL), "/")
	path = "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
	return base + path
}

func normalizeTimeout(timeoutSec int) time.Duration {
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		return 35 * time.Second
	}
	return timeout
}
