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

// ChannelLinkedIn is the ledger's name for this publisher. Stored in
// social_digest_posts.channel and in social_tokens.channel, so it must not be renamed — the
// publish-once check reads it back, and a rename would republish every past day.
const ChannelLinkedIn = "linkedin"

// linkedInAPIVersion is the API generation this publisher speaks, as LinkedIn's YYYYMM header.
//
// IT EXPIRES. LinkedIn sunsets a version roughly a year after it ships (202508 died on
// 2026-08-17), and a request naming a retired one is refused — so this constant is a dated
// commitment, not a detail. It lives in code rather than in configuration deliberately: moving
// it forward means re-reading the migration notes for what changed in the Posts API between
// the two, which is a reviewed commit, not an SSH edit.
//
// Next review: before 2027-08. See
// https://learn.microsoft.com/en-us/linkedin/marketing/integrations/migrations
const linkedInAPIVersion = "202608"

const linkedInPostsURL = "https://api.linkedin.com/rest/posts"

// linkedInCommentaryLimit is the cap on a post's text. Overflow is refused with
// FIELD_LENGTH_TOO_LONG, which would cost the day's post entirely, so the digest is trimmed to
// fit — by whole postings, never mid-text; see payload.
const linkedInCommentaryLimit = 3000

// TokenSource hands out the access token this publisher posts with.
//
// Declared here, by the consumer, rather than imported from the package that mints tokens:
// this file needs a string and an error, and nothing else about OAuth. It also keeps the two
// packages independent — linkedinauth knows about grants and expiry, socialdigest knows about
// lists and channels, and cmd/social-digest is where they meet.
type TokenSource interface {
	// AccessToken returns a token that is usable NOW. An expired or missing credential is an
	// error rather than an empty string, because the two failures need different words and
	// both are somebody's job to fix.
	AccessToken(ctx context.Context) (string, error)
}

// LinkedInPublisher posts a digest to a LinkedIn company page.
//
// Unlike the Discord publisher beside it, its credential expires every 60 days — which is why
// it takes a TokenSource rather than a string, and why cmd/linkedin-token-refresh exists.
type LinkedInPublisher struct {
	tokens TokenSource
	// orgURN is the company page posted to, as urn:li:organization:{id}. Configuration and not
	// discovery: the token's holder may administer several pages, and "the first one" is not a
	// thing this feature should decide on its own at 06:45 UTC.
	orgURN string
	origin string
	http   *http.Client

	// postsURL is the API endpoint, a field rather than the constant used directly so the
	// tests can point it at a stub and assert what actually goes over the wire — headers
	// included, which is where two of this API's three unforgiving requirements live.
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

// linkedInPayload is the Posts API body for a plain text post by an organization.
//
// Every field here is required by the API — a missing one is a 400 MISSING_FIELD — including
// the ones whose values are the defaults a reader would assume. distribution.targetEntities is
// empty because the digest is for everyone who follows the page; a targeted post needs an
// audience over 300 and a reason, and this has neither.
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
	// Present and empty rather than omitted: the API rejects a distribution object without
	// them, and `omitempty` on a nil slice would produce exactly that.
	TargetEntities                 []string `json:"targetEntities"`
	ThirdPartyDistributionChannels []string `json:"thirdPartyDistributionChannels"`
}

// Render builds the exact JSON body Publish sends, indented so a dry run is something a person
// can read a judgement off — the failure a dry run catches is a list that reads badly, and
// that is a property of the text.
func (p *LinkedInPublisher) Render(d Digest) (string, error) {
	body, err := json.MarshalIndent(p.payload(d), "", "  ")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// payload assembles the post.
//
// The list is trimmed by WHOLE POSTINGS when it would exceed the limit, which is the one place
// this differs from the Discord publisher's rune-count truncation. Discord's text carries its
// links inside markdown, so a clipped tail loses formatting; here every posting's URL is bare
// text on its own line, and cutting mid-string would publish a link that goes nowhere. A short
// list is a smaller loss than a broken link.
func (p *LinkedInPublisher) payload(d Digest) linkedInPayload {
	header := "Most viewed on freehire — " + d.Day.Format("2 January 2006")

	var b strings.Builder
	b.WriteString(escapeLinkedInText(header))
	for i, item := range d.Items {
		entry := p.entry(i+1, item)
		// +2 for the blank line that separates entries. Counted in runes, because the API's
		// limit is a character count and this text is routinely not ASCII.
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

// entry is one posting, as two lines: what it is, and where it is.
//
// The URL is NOT escaped, and everything around it is. LinkedIn's reserved characters do not
// appear in a URL this site generates — the slug is lowercase alphanumerics and hyphens, and
// the only query parameter is a literal — while a backslash inside a link would be published
// verbatim and break it.
func (p *LinkedInPublisher) entry(n int, item Posting) string {
	line := fmt.Sprintf("%d. %s — %s", n, escapeLinkedInText(item.Title), escapeLinkedInText(item.Company))
	if where := placeOf(item); where != "" {
		line += " · " + escapeLinkedInText(where)
	}
	return line + "\n" + jobURL(p.origin, item.Slug, ChannelLinkedIn)
}

// escapeLinkedInText neutralises the characters LinkedIn's "little" text format reserves.
//
// All of them must be escaped even when they are plain text and not part of any element — a
// bare "(" in a title like "Senior C++ (Remote)" is a syntax error, not a parenthesis, and the
// API answers the whole post with a 400. This catalogue's titles contain every one of these
// characters routinely.
//
// The backslash goes first, and the ordering is why this is a Replacer rather than a loop over
// a list: escaping "\" after "(" would turn the "\(" just written into "\\(", which publishes
// a literal backslash followed by a syntax error.
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
	// Both of these are required on every Posts API call, and omitting either is a 400 that
	// names neither header.
	req.Header.Set("LinkedIn-Version", linkedInAPIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("linkedin posts api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// LinkedIn names the offending field in the body, which is the only thing that tells a
		// 400 for an over-long commentary apart from one for a malformed URN — and a 401 for
		// an expired token apart from a 403 for a page role that was taken away. Bounded,
		// because this ends up in a log line.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("linkedin posts api: status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}
