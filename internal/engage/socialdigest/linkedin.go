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

// ChannelLinkedIn is the ledger's name for this publisher, stored in social_digest_posts.channel
// and social_tokens.channel. Never rename it: the publish-once check reads it back.
const ChannelLinkedIn = "linkedin"

// linkedInAPIVersion is the API version header (YYYYMM). It expires: LinkedIn retires a version
// about a year after release and refuses requests naming it. It lives in code, not config, so
// bumping it is a reviewed commit after reading the Posts API migration notes
// (https://learn.microsoft.com/en-us/linkedin/marketing/integrations/migrations). Next review:
// before 2027-08.
const linkedInAPIVersion = "202608"

const linkedInPostsURL = "https://api.linkedin.com/rest/posts"

// linkedInCommentaryLimit is the cap on a post's text. Overflow is refused with
// FIELD_LENGTH_TOO_LONG, so the digest is trimmed by whole postings; see payload.
const linkedInCommentaryLimit = 3000

// TokenSource hands out the access token this publisher posts with. Declared by the consumer to
// keep socialdigest independent of linkedinauth; cmd/social-digest is where they meet.
type TokenSource interface {
	// AccessToken returns a token usable now. An expired or missing credential is an error,
	// never an empty string.
	AccessToken(ctx context.Context) (string, error)
}

// LinkedInPublisher posts a digest to a LinkedIn company page. Its credential expires every 60
// days, hence the TokenSource and cmd/linkedin-token-refresh.
type LinkedInPublisher struct {
	tokens TokenSource
	// orgURN is the company page posted to, as urn:li:organization:{id}. Configured, not
	// discovered: the token's holder may administer several pages.
	orgURN string
	origin string
	http   *http.Client

	// postsURL is a field so tests can point it at a stub and assert the wire format, headers
	// included.
	postsURL string
}

// NewLinkedInPublisher builds a publisher for one company page. origin is the public site
// origin the job links are rooted at.
func NewLinkedInPublisher(tokens TokenSource, orgURN, origin string) *LinkedInPublisher {
	return &LinkedInPublisher{
		tokens:   tokens,
		orgURN:   orgURN,
		origin:   origin,
		http:     &http.Client{Timeout: 20 * time.Second},
		postsURL: linkedInPostsURL,
	}
}

func (p *LinkedInPublisher) Name() string { return ChannelLinkedIn }

// linkedInPayload is the Posts API body for a plain text post by an organization. Every field
// is required (a missing one is a 400 MISSING_FIELD), even those holding the assumed default.
// distribution.targetEntities is empty because the digest is for all followers.
type linkedInPayload struct {
	Author                    string               `json:"author"`
	Commentary                string               `json:"commentary"`
	Visibility                string               `json:"visibility"`
	Distribution              linkedInDistribution `json:"distribution"`
	LifecycleState            string               `json:"lifecycleState"`
	IsReshareDisabledByAuthor bool                 `json:"isReshareDisabledByAuthor"`
}

type linkedInDistribution struct {
	FeedDistribution string `json:"feedDistribution"`
	// Present and empty, not omitted: the API rejects a distribution object without them.
	TargetEntities                 []string `json:"targetEntities"`
	ThirdPartyDistributionChannels []string `json:"thirdPartyDistributionChannels"`
}

// Render builds the exact JSON body Publish sends, indented so a dry run is readable.
func (p *LinkedInPublisher) Render(d Digest) (string, error) {
	body, err := json.MarshalIndent(p.payload(d), "", "  ")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// payload assembles the post. Unlike the Discord publisher's rune truncation, the list is
// trimmed by whole postings: each URL is bare text, and a mid-string cut would publish a
// broken link.
func (p *LinkedInPublisher) payload(d Digest) linkedInPayload {
	header := d.Title()

	var b strings.Builder
	b.WriteString(escapeLinkedInText(header))
	for i, item := range d.Items {
		entry := p.entry(i+1, item)
		// +2 for the blank line between entries. Runes, because the limit is a character count.
		if len([]rune(b.String()))+len([]rune(entry))+2 > linkedInCommentaryLimit {
			break
		}
		b.WriteString("\n\n")
		b.WriteString(entry)
	}

	return linkedInPayload{
		Author:     p.orgURN,
		Commentary: b.String(),
		Visibility: "PUBLIC",
		Distribution: linkedInDistribution{
			FeedDistribution:               "MAIN_FEED",
			TargetEntities:                 []string{},
			ThirdPartyDistributionChannels: []string{},
		},
		LifecycleState:            "PUBLISHED",
		IsReshareDisabledByAuthor: false,
	}
}

// entry is one posting as two lines: what it is, and where it is.
//
// The URL is not escaped (a backslash would be published verbatim and break it); our URLs
// contain no reserved characters, since slugs are lowercase alphanumerics and hyphens.
func (p *LinkedInPublisher) entry(n int, item Posting) string {
	line := fmt.Sprintf("%d. %s — %s", n, escapeLinkedInText(item.Title), escapeLinkedInText(item.Company))
	if where := placeOf(item); where != "" {
		line += " · " + escapeLinkedInText(where)
	}
	// Unescaped: text this file wrote, with no reserved characters.
	line += " · " + viewsLabel(item)
	return line + "\n" + jobURL(p.origin, item.Slug, ChannelLinkedIn)
}

// escapeLinkedInText neutralises the characters LinkedIn's "little" text format reserves. All
// must be escaped even in plain text: a bare "(" in "Senior C++ (Remote)" is a syntax error and
// the API answers the whole post with a 400.
//
// A Replacer, not a loop over a list, so the backslash is escaped in the same pass and the
// "\(" just written is not turned into "\\(".
func escapeLinkedInText(s string) string { return linkedInEscaper.Replace(s) }

var linkedInEscaper = strings.NewReplacer(
	`\`, `\\`,
	`|`, `\|`,
	`{`, `\{`,
	`}`, `\}`,
	`@`, `\@`,
	`[`, `\[`,
	`]`, `\]`,
	`(`, `\(`,
	`)`, `\)`,
	`<`, `\<`,
	`>`, `\>`,
	`#`, `\#`,
	`*`, `\*`,
	`_`, `\_`,
	`~`, `\~`,
)

// Publish sends the digest to the company page.
func (p *LinkedInPublisher) Publish(ctx context.Context, d Digest) error {
	token, err := p.tokens.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("linkedin credential: %w", err)
	}

	body, err := json.Marshal(p.payload(d))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.postsURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	// Both are required on every Posts API call; omitting either is a 400 that names neither.
	req.Header.Set("LinkedIn-Version", linkedInAPIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("linkedin posts api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The body names the offending field, the only way to tell failures sharing a status
		// apart (over-long commentary vs malformed URN, expired token vs revoked page role).
		// Bounded because it lands in a log line.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("linkedin posts api: status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}
