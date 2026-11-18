package socialdigest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChannelDiscord is the ledger's name for this publisher. Stored in
// social_digest_posts.channel; renaming it would republish every past day.
const ChannelDiscord = "discord"

// discordEmbedColor is the embed's left border as decimal RGB (0x2F81F7, the site's link blue).
const discordEmbedColor = 3113463

// DiscordPublisher posts a digest to a Discord channel through an incoming webhook.
//
// A webhook needs no bot, gateway connection or token refresh: the URL is the
// credential and does not expire.
type DiscordPublisher struct {
	webhookURL string
	origin     string
	http       *http.Client
}

// NewDiscordPublisher builds a publisher for one webhook URL. origin is the public
// site origin the job links are rooted at.
//
// A plain http.Client, not safehttp: the webhook URL is operator configuration for a
// fixed vendor host, not user input, so there is no SSRF surface (as in telegramnotify).
func NewDiscordPublisher(webhookURL, origin string) *DiscordPublisher {
	return &DiscordPublisher{
		webhookURL: webhookURL,
		origin:     origin,
		http:       &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *DiscordPublisher) Name() string { return ChannelDiscord }

// discordPayload is the webhook body, listing only the fields this feature sets.
type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
}

// Render builds the exact JSON body Publish sends, indented so a dry run is readable.
func (p *DiscordPublisher) Render(d Digest) (string, error) {
	body, err := json.MarshalIndent(p.payload(d), "", "  ")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (p *DiscordPublisher) payload(d Digest) discordPayload {
	var b strings.Builder
	for i, item := range d.Items {
		fmt.Fprintf(&b, "**%d.** [%s](%s)\n%s",
			i+1,
			escapeDiscordMarkdown(item.Title),
			jobURL(p.origin, item.Slug, ChannelDiscord),
			escapeDiscordMarkdown(item.Company),
		)
		if where := placeOf(item); where != "" {
			b.WriteString(" · " + escapeDiscordMarkdown(where))
		}
		// The view count is a number and a word this file wrote, so it needs no escaping.
		b.WriteString(" · " + viewsLabel(item))
		b.WriteString("\n\n")
	}
	return discordPayload{Embeds: []discordEmbed{{
		Title:       d.Title(),
		Description: truncateRunes(strings.TrimRight(b.String(), "\n"), discordDescriptionLimit),
		Color:       discordEmbedColor,
	}}}
}

// discordDescriptionLimit is Discord's cap on an embed description. Ten items fit,
// but titles are unbounded and escapeDiscordMarkdown can nearly double one made of
// punctuation. Overflow would be a 400 that silently costs the day; a clipped post costs nothing.
const discordDescriptionLimit = 4096

// truncateRunes cuts s to at most limit runes, marking the cut. Counted in runes, not
// bytes: Discord's limit is a character count and a mid-rune cut would be rejected.
func truncateRunes(s string, limit int) string {
	if len([]rune(s)) <= limit {
		return s
	}
	const ellipsis = "…"
	return string([]rune(s)[:limit-len([]rune(ellipsis))]) + ellipsis
}

// placeOf is where the job is, as ONE short phrase on one line. Remote wins over a
// city when the posting claims both.
//
// Some feeds put a newline in jobs.location, which would silently restructure the
// list, so whitespace is collapsed here.
func placeOf(p Posting) string {
	if p.Remote {
		return "Remote"
	}
	return strings.Join(strings.Fields(p.Location), " ")
}

// escapeDiscordMarkdown neutralises characters Discord reads as formatting, e.g. a
// title like "C++ (Senior) *urgent*" that would reformat the message or break its link.
func escapeDiscordMarkdown(s string) string {
	return discordEscaper.Replace(s)
}

var discordEscaper = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	`*`, `\*`,
	`_`, `\_`,
	`~`, `\~`,
	`|`, `\|`,
	`[`, `\[`,
	`]`, `\]`,
	`(`, `\(`,
	`)`, `\)`,
	`#`, `\#`,
	`>`, `\>`,
)

// Publish sends the digest to the webhook.
func (p *DiscordPublisher) Publish(ctx context.Context, d Digest) error {
	body, err := json.Marshal(p.payload(d))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The JSON body names the rejected field, which tells a too-long description
		// from a retired webhook. Bounded because it goes into a log line.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("discord webhook: status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}
