package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"

	channelx "github.com/hsvtr365/telegram_romance_AI_bot/internal/channel"
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
	return c.sendMessage(ctx, strconv.FormatInt(chatID, 10), text)
}

func (c *Client) SendText(ctx context.Context, target channelx.OutboundTarget, text string) error {
	return c.sendMessage(ctx, target.ExternalChatID, text)
}

func (c *Client) sendMessage(ctx context.Context, chatID string, text string) error {
	params := url.Values{}
	params.Set("chat_id", chatID)
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
	return c.sendChatAction(ctx, strconv.FormatInt(chatID, 10), action)
}

func (c *Client) SendTyping(ctx context.Context, target channelx.OutboundTarget) error {
	return c.sendChatAction(ctx, target.ExternalChatID, ChatActionTyping)
}

func (c *Client) SendAudio(ctx context.Context, target channelx.OutboundTarget, audio channelx.AudioAttachment) error {
	if len(audio.Data) == 0 {
		return nil
	}
	fileName := strings.TrimSpace(audio.FileName)
	if fileName == "" {
		fileName = "reward.wav"
	}
	return c.sendMultipart(ctx, "sendAudio", map[string]string{
		"chat_id": target.ExternalChatID,
		"caption": audio.Caption,
	}, "audio", fileName, audio.MIMEType, audio.Data)
}

func (c *Client) sendChatAction(ctx context.Context, chatID string, action string) error {
	params := url.Values{}
	params.Set("chat_id", chatID)
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

func (c *Client) sendMultipart(ctx context.Context, apiMethod string, fields map[string]string, fileField string, fileName string, contentType string, data []byte) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return err
		}
	}

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeQuotes(fileField), escapeQuotes(fileName)))
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	endpoint := strings.TrimRight(c.baseURL, "/") + "/" + apiMethod
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var response apiResponse[json.RawMessage]
	if err := json.Unmarshal(payload, &response); err != nil {
		return fmt.Errorf("decode telegram response: %w", err)
	}
	if !response.OK {
		return fmt.Errorf("telegram %s failed: %s", apiMethod, response.Description)
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
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
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

func escapeQuotes(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, `\"`).Replace(value)
}
