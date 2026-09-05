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
// social_digest_posts.channel, so it must not be renamed — the publish-once check
// reads it back and a rename would republish every past day.
const ChannelDiscord = "discord"

// discordEmbedColor is the left border of the embed, as Discord's decimal RGB.
// 0x2F81F7 — the blue the site uses for links.
const discordEmbedColor = 3113463

// DiscordPublisher posts a digest to a Discord channel through an incoming webhook.
//
// A webhook needs no bot, no gateway connection and no token refresh: the URL is the
// credential and it does not expire. That is most of why Discord is the first channel
// this feature ships with.
type DiscordPublisher struct {
	webhookURL string
	origin     string
	http       *http.Client
}

// NewDiscordPublisher builds a publisher for one webhook URL. origin is the public
// site origin the job links are rooted at.
//
// A plain http.Client rather than safehttp: the webhook URL is operator
// configuration pointing at a fixed vendor host, not user input, so there is no SSRF
// surface to guard — the same reasoning as internal/engage/telegramnotify.
func NewDiscordPublisher(webhookURL, origin string) *DiscordPublisher {
	return &DiscordPublisher{
		webhookURL: webhookURL,
		origin:     origin,
		http:       &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *DiscordPublisher) Name() string { return ChannelDiscord }

// discordPayload is the webhook body. Only the fields this feature sets — Discord
// ignores what it is not sent, and a struct listing everything it accepts would
// suggest we had decided about fields we have not.
type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
}

// Render builds the exact JSON body Publish sends, indented so that a dry run is
// something a person can read a judgement off. What a dry run is for is catching a
// list that reads badly, and that is a property of the text, not of the wire format.
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
		b.WriteString("\n\n")
	}
	return discordPayload{Embeds: []discordEmbed{{
		Title:       "Most viewed on freehire — " + d.Day.Format("2 January 2006"),
		Description: truncateRunes(strings.TrimRight(b.String(), "\n"), discordDescriptionLimit),
		Color:       discordEmbedColor,
	}}}
}

// discordDescriptionLimit is Discord's cap on an embed description. Ten items sit
// comfortably inside it, but titles in this catalogue are unbounded and
// escapeDiscordMarkdown can nearly double one made of punctuation. Overflow would be
// a 400 with nothing written to the ledger — recoverable, but it would cost the day
// silently until somebody read the log, and a slightly clipped post costs nothing.
const discordDescriptionLimit = 4096

// truncateRunes cuts s to at most limit runes, marking the cut so a clipped post does
// not read as a post that simply ended. Counted in runes, not bytes: Discord's limit
// is a character count, and cutting a byte slice mid-rune would produce a payload it
// rejects outright — trading a clipped post for no post at all.
func truncateRunes(s string, limit int) string {
	if len([]rune(s)) <= limit {
		return s
	}
	const ellipsis = "…"
	return string([]rune(s)[:limit-len([]rune(ellipsis))]) + ellipsis
}

// placeOf is where the job is, as ONE short phrase on one line. Remote is worth more
// to a reader than a city, so it wins when the posting claims both and there is room
// for one.
//
// jobs.location is whatever the source feed called the place, and some of them put a
// newline in it. Collapsing whitespace here rather than escaping it later: a line
// break would not be wrong-looking, it would silently restructure the list.
func placeOf(p Posting) string {
	if p.Remote {
		return "Remote"
	}
	return strings.Join(strings.Fields(p.Location), " ")
}

// escapeDiscordMarkdown neutralises the characters Discord reads as formatting. A
// job title like "C++ (Senior) *urgent*" is ordinary in this catalogue and would
// otherwise reformat the message around itself, or break the link that contains it.
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
		// Discord answers a rejected webhook with a JSON body naming the field it
		// disliked, which is the only thing that tells a 400 caused by a too-long
		// description apart from one caused by a retired webhook. Bounded, because
		// this ends up in a log line.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("discord webhook: status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}
