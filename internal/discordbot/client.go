package discordbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// defaultAPIBase is the Discord REST API host. A fixed, trusted host — no SSRF
// surface — so a plain http.Client is used.
const defaultAPIBase = "https://discord.com/api/v10"

// Client is a thin Discord REST client over net/http for the handful of calls
// this feature needs: editing the deferred interaction response and
// registering guild slash commands.
type Client struct {
	token string
	base  string
	http  *http.Client
}

// NewClient builds a Client for the given bot token against the public API.
func NewClient(token string) *Client {
	return NewClientWithBase(token, defaultAPIBase)
}

// NewClientWithBase builds a Client against a custom API base. Used by tests
// (pointing at a stub server).
func NewClientWithBase(token, baseURL string) *Client {
	return &Client{token: token, base: baseURL, http: &http.Client{Timeout: 10 * time.Second}}
}

// EditOriginalResponse edits the message a deferred interaction reply left
// pending, so a slash-command handler can defer immediately (Discord requires
// a response within 3 seconds) and fill in the real content once its work is
// done. No Authorization header is sent: the interaction token itself
// authorizes this call, and a bot token here would be wrong, not just
// redundant.
func (c *Client) EditOriginalResponse(ctx context.Context, applicationID, interactionToken, content string) error {
	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return err
	}
	url := c.base + "/webhooks/" + applicationID + "/" + interactionToken + "/messages/@original"
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("discord edit original response: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord edit original response failed (%d): %s", resp.StatusCode, b)
	}
	return nil
}

// RegisterCommands replaces the guild's slash-command set with commands. This
// call is authorized by the bot token, unlike EditOriginalResponse.
func (c *Client) RegisterCommands(ctx context.Context, applicationID, guildID string, commands []Command) error {
	body, err := json.Marshal(commands)
	if err != nil {
		return err
	}
	url := c.base + "/applications/" + applicationID + "/guilds/" + guildID + "/commands"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bot "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("discord register commands: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord register commands failed (%d): %s", resp.StatusCode, b)
	}
	return nil
}
