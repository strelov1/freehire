package telegramnotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultAPIBase is the Telegram Bot API host. A fixed, trusted host — no SSRF
// surface — so a plain http.Client is used.
const defaultAPIBase = "https://api.telegram.org"

// ErrChatUnreachable wraps every 403 the Bot API answers a send with (bot blocked, account
// deactivated, bot removed, conversation never allowed): this chat will not accept messages
// again, and retrying does not help.
//
// Matched on the status code, not the description text, which Telegram may reword.
var ErrChatUnreachable = errors.New("telegramnotify: chat unreachable")

// Client is a thin Telegram Bot API client over net/http for the methods this feature needs
// (sendMessage, setWebhook), avoiding a bot-framework dependency.
type Client struct {
	token string
	base  string
	http  *http.Client
}

// NewClient builds a Client for the given bot token against the public Bot API.
func NewClient(token string) *Client {
	return NewClientWithBase(token, defaultAPIBase)
}

// NewClientWithBase builds a Client against a custom API base, e.g. a test stub server.
func NewClientWithBase(token, baseURL string) *Client {
	return &Client{token: token, base: baseURL, http: &http.Client{Timeout: 10 * time.Second}}
}

// SendMessage posts an HTML-formatted message to a chat, with web-page previews
// disabled so a multi-link digest does not expand into a wall of cards.
func (c *Client) SendMessage(ctx context.Context, chatID int64, html string) error {
	return c.call(ctx, "sendMessage", map[string]any{
		"chat_id":                  chatID,
		"text":                     html,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
}

// SetWebhook registers the inbound webhook URL and its secret token (sent by
// Telegram in the X-Telegram-Bot-Api-Secret-Token header on every update).
func (c *Client) SetWebhook(ctx context.Context, url, secret string) error {
	return c.call(ctx, "setWebhook", map[string]any{
		"url":          url,
		"secret_token": secret,
	})
}

// apiResponse is the envelope every Bot API method returns.
type apiResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func (c *Client) call(ctx context.Context, method string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := c.base + "/bot" + c.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, withoutURL(err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, withoutURL(err))
	}
	defer resp.Body.Close()

	var r apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("telegram %s: decode response: %w", method, err)
	}
	if !r.OK {
		if resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("telegram %s failed (%d): %s: %w", method, resp.StatusCode, r.Description, ErrChatUnreachable)
		}
		return fmt.Errorf("telegram %s failed (%d): %s", method, resp.StatusCode, r.Description)
	}
	return nil
}

// withoutURL drops the request URL from a net/http error: the bot token is in the path,
// *url.Error quotes it verbatim, and the error text is persisted and logged. The wrapped cause
// is kept for errors.Is/As.
func withoutURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

// Update is the minimal slice of a Telegram webhook update this feature reads: a
// message's chat id and text. Everything else is ignored.
type Update struct {
	Message *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

// StartToken extracts the payload of a "/start <token>" command and the chat it
// came from, reporting whether the update is such a command. The deep link
// t.me/<bot>?start=<token> delivers the token as the /start argument.
func StartToken(u Update) (token string, chatID int64, ok bool) {
	if u.Message == nil {
		return "", 0, false
	}
	const prefix = "/start "
	if !strings.HasPrefix(u.Message.Text, prefix) {
		return "", 0, false
	}
	token = strings.TrimSpace(strings.TrimPrefix(u.Message.Text, prefix))
	if token == "" {
		return "", 0, false
	}
	return token, u.Message.Chat.ID, true
}
