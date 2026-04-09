package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const ChatActionTyping = "typing"

type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewClient(token string, logger *slog.Logger) *Client {
	return &Client{
		baseURL:    "https://api.telegram.org/bot" + token,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSec int, limit int, allowedUpdates []string) ([]Update, error) {
	params := url.Values{}
	params.Set("timeout", strconv.Itoa(timeoutSec))
	params.Set("limit", strconv.Itoa(limit))

	if offset > 0 {
		params.Set("offset", strconv.FormatInt(offset, 10))
	}

	if len(allowedUpdates) > 0 {
		encoded, err := json.Marshal(allowedUpdates)
		if err != nil {
			return nil, err
		}
		params.Set("allowed_updates", string(encoded))
	}

	var response apiResponse[[]Update]
	if err := c.call(ctx, http.MethodGet, "getUpdates", params, &response); err != nil {
		return nil, err
	}

	if !response.OK {
		return nil, fmt.Errorf("telegram getUpdates failed: %s", response.Description)
	}

	return response.Result, nil
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	params := url.Values{}
	params.Set("chat_id", strconv.FormatInt(chatID, 10))
	params.Set("text", text)

	var response apiResponse[json.RawMessage]
	if err := c.call(ctx, http.MethodPost, "sendMessage", params, &response); err != nil {
		return err
	}

	if !response.OK {
		return fmt.Errorf("telegram sendMessage failed: %s", response.Description)
	}

	return nil
}

func (c *Client) SendChatAction(ctx context.Context, chatID int64, action string) error {
	params := url.Values{}
	params.Set("chat_id", strconv.FormatInt(chatID, 10))
	params.Set("action", action)

	var response apiResponse[json.RawMessage]
	if err := c.call(ctx, http.MethodPost, "sendChatAction", params, &response); err != nil {
		return err
	}

	if !response.OK {
		return fmt.Errorf("telegram sendChatAction failed: %s", response.Description)
	}

	return nil
}

func (c *Client) call(ctx context.Context, method string, apiMethod string, params url.Values, out any) error {
	endpoint := strings.TrimRight(c.baseURL, "/") + "/" + apiMethod

	var req *http.Request
	var err error

	switch method {
	case http.MethodGet:
		req, err = http.NewRequestWithContext(ctx, method, endpoint+"?"+params.Encode(), nil)
	case http.MethodPost:
		req, err = http.NewRequestWithContext(ctx, method, endpoint, bytes.NewBufferString(params.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	default:
		return fmt.Errorf("unsupported http method: %s", method)
	}

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
		return fmt.Errorf("telegram api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode telegram response: %w", err)
	}

	return nil
}
