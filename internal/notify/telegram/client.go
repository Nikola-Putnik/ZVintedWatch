// Package telegram sends ZVintedWatch notifications through the Telegram Bot
// API: a minimal API client (client.go) and a delivery service with per-chat
// queues, rate limiting and retries (service.go).
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the public Bot API endpoint.
const DefaultBaseURL = "https://api.telegram.org"

// Client is a minimal Telegram Bot API client.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// NewClient creates a client for a bot token. baseURL may be empty.
func NewClient(token, baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{token: token, baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 30 * time.Second}}
}

// APIError is an error reported by the Bot API.
type APIError struct {
	Code        int
	Description string
	// RetryAfter is set on HTTP 429 (flood control), in seconds.
	RetryAfter int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram: %s (code %d)", e.Description, e.Code)
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// User is a Telegram user or bot.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// Chat is a Telegram chat.
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"` // private, group, supergroup, channel
	Title     string `json:"title"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// Update is the subset of an update used for chat discovery.
type Update struct {
	UpdateID     int64       `json:"update_id"`
	Message      *chatHolder `json:"message"`
	ChannelPost  *chatHolder `json:"channel_post"`
	MyChatMember *chatHolder `json:"my_chat_member"`
}

// chatHolder matches every update payload that carries a chat.
type chatHolder struct {
	Chat Chat `json:"chat"`
}

// InlineButton is a URL button attached to a message.
type InlineButton struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]InlineButton `json:"inline_keyboard"`
}

func keyboard(buttons []InlineButton) *inlineKeyboard {
	if len(buttons) == 0 {
		return nil
	}
	return &inlineKeyboard{InlineKeyboard: [][]InlineButton{buttons}}
}

// call performs a JSON API call and decodes the result into out (may be nil).
func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(method), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) endpoint(method string) string {
	return c.baseURL + "/bot" + c.token + "/" + method
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		// Never leak the token, which is part of the URL, in error messages.
		return fmt.Errorf("telegram: request failed: %s", strings.ReplaceAll(err.Error(), c.token, "<token>"))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	var r apiResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("telegram: unexpected response (HTTP %d)", resp.StatusCode)
	}
	if !r.OK {
		return &APIError{Code: r.ErrorCode, Description: r.Description, RetryAfter: r.Parameters.RetryAfter}
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// GetMe returns the bot identity; it is used to validate a token.
func (c *Client) GetMe(ctx context.Context) (User, error) {
	var u User
	err := c.call(ctx, "getMe", struct{}{}, &u)
	return u, err
}

// GetUpdates returns pending updates without acknowledging them.
func (c *Client) GetUpdates(ctx context.Context) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", map[string]any{
		"limit":           100,
		"timeout":         0,
		"allowed_updates": []string{"message", "channel_post", "my_chat_member"},
	}, &updates)
	return updates, err
}

// SendMessage sends an HTML-formatted text message.
func (c *Client) SendMessage(ctx context.Context, chatID, text string, buttons []InlineButton) error {
	params := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
		"link_preview_options": map[string]any{
			"is_disabled": len(buttons) == 0,
		},
	}
	if kb := keyboard(buttons); kb != nil {
		params["reply_markup"] = kb
	}
	return c.call(ctx, "sendMessage", params, nil)
}

// SendPhotoURL sends a photo that Telegram downloads from photoURL.
func (c *Client) SendPhotoURL(ctx context.Context, chatID, photoURL, caption string, buttons []InlineButton) error {
	params := map[string]any{
		"chat_id":    chatID,
		"photo":      photoURL,
		"caption":    caption,
		"parse_mode": "HTML",
	}
	if kb := keyboard(buttons); kb != nil {
		params["reply_markup"] = kb
	}
	return c.call(ctx, "sendPhoto", params, nil)
}

// SendPhotoFile uploads a JPEG photo.
func (c *Client) SendPhotoFile(ctx context.Context, chatID string, jpeg []byte, caption string, buttons []InlineButton) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("chat_id", chatID)
	_ = w.WriteField("caption", caption)
	_ = w.WriteField("parse_mode", "HTML")
	if kb := keyboard(buttons); kb != nil {
		markup, _ := json.Marshal(kb)
		_ = w.WriteField("reply_markup", string(markup))
	}
	part, err := w.CreateFormFile("photo", "photo.jpg")
	if err != nil {
		return err
	}
	if _, err := part.Write(jpeg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("sendPhoto"), &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return c.do(req, nil)
}
