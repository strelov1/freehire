package sources

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// ismartRecruit adapts iSmartRecruit-hosted career boards. Each company embeds a public job
// widget at app.ismartrecruit.com/openJobWebsite?tenantId=<token>, where the token wraps the
// company's tenant string as "E7p" + rawbase64("<domain>_<code>") + "Q9e"; the YAML board id
// carries the readable "<domain>_<code>" and the adapter wraps it. The listing server-renders
// EVERY open posting in a single grid page (title, location, and a jobDescription?x=<token>
// detail link) with no pagination, so one GET yields the whole board. A detail page carries no
// schema.org JobPosting — its role is in og:title and its full body in og:description. Keyless.

const ismartRecruitBaseURL = "https://app.ismartrecruit.com"

// ismartRecruitTokenPrefix/Suffix wrap every iSmartRecruit tenant and posting token. They are a
// fixed decorative envelope around a base64 payload, not encryption (identical across tenants).
const (
	ismartRecruitTokenPrefix = "E7p"
	ismartRecruitTokenSuffix = "Q9e"
)

type ismartRecruit struct {
	http HTMLGetter
}

// NewISmartRecruit builds the iSmartRecruit career-board adapter over the given HTML client.
func NewISmartRecruit(c HTMLGetter) Source { return ismartRecruit{http: c} }

func (ismartRecruit) Provider() string { return "ismartrecruit" }

// ismartRecruitCard is one posting as the listing renders it — everything but the body.
type ismartRecruitCard struct {
	ExternalID string
	URL        string
	Title      string
	Location   string
}

func (s ismartRecruit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	root, err := s.http.GetHTML(ctx, ismartRecruitListingURL(e.Board))
	if err != nil {
		return nil, fmt.Errorf("ismartrecruit: listing %s: %w", e.Board, err)
	}
	cards := ismartRecruitCards(root)
	return fetchDetails(cards, defaultDetailWorkers, func(c ismartRecruitCard) (Job, bool) {
		return s.detail(ctx, e, c), true
	}), nil
}

// ismartRecruitListingURL wraps a readable board id into its tenant token and builds the grid
// listing URL that server-renders every posting. The tenant token uses no URL-reserved bytes for
// the boards in the wild, so it is interpolated verbatim to byte-match the widget's own URL.
func ismartRecruitListingURL(board string) string {
	tenant := ismartRecruitTokenPrefix + base64.RawStdEncoding.EncodeToString([]byte(board)) + ismartRecruitTokenSuffix
	return fmt.Sprintf("%s/openJobWebsite?tenantId=%s&view=grid&col=2&lang=en", ismartRecruitBaseURL, tenant)
}

// detail fetches one posting's detail page and folds its og:description body onto the card. A
// failed or body-less detail fetch is not fatal: the listing already carries the posting's id,
// url, title, and location, so the job still ingests (only its description is left empty).
func (s ismartRecruit) detail(ctx context.Context, e CompanyEntry, c ismartRecruitCard) Job {
	job := Job{
		ExternalID: c.ExternalID,
		URL:        c.URL,
		Title:      c.Title,
		Company:    e.Company,
		Location:   c.Location,
		Remote:     isRemote(c.Location),
	}
	root, err := s.http.GetHTML(ctx, c.URL)
	if err != nil {
		return job
	}
	return ismartRecruitEnrich(job, root)
}

// ismartRecruitEnrich folds a detail page's og metadata onto a card-derived job: the body from
// og:description (double-entity-encoded in the attribute, which sanitizeHTML's parse decodes),
// and og:title as a title fallback for the rare card that renders none.
func ismartRecruitEnrich(job Job, root *html.Node) Job {
	if body := metaProperty(root, "og:description"); body != "" {
		job.Description = sanitizeHTML(body)
	}
	if job.Title == "" {
		job.Title = strings.TrimSpace(metaProperty(root, "og:title"))
	}
	return job
}

// ismartRecruitCards extracts every posting card from a listing page in document order,
// deduplicated by external id (the grid renders each posting once).
func ismartRecruitCards(root *html.Node) []ismartRecruitCard {
	var cards []ismartRecruitCard
	seen := map[string]struct{}{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" || !hasClass(n, "jobListing_Data") {
			return true
		}
		href := attr(n, "href")
		id := ismartRecruitExternalID(href)
		if id == "" {
			return false
		}
		if _, dup := seen[id]; dup {
			return false
		}
		seen[id] = struct{}{}
		cards = append(cards, ismartRecruitCard{
			ExternalID: id,
			URL:        href,
			Title:      textContent(firstByClass(n, "ui-panel-title")),
			Location:   textContent(firstByClass(n, "ui-panelgrid-cell")),
		})
		return false // the card's title/location were read from n's subtree already
	})
	return cards
}

// ismartRecruitXPattern captures the x=<token> posting token from a jobDescription detail href,
// stopping at the next query separator so a trailing &view=grid is excluded.
var ismartRecruitXPattern = regexp.MustCompile(`[?&]x=([^&]+)`)

// ismartRecruitIDPattern captures the numeric posting id from a decoded token
// "<domain>_<id>_W_<lang>". The domain carries no underscore, so the first "_<digits>_W_" run is
// the posting id.
var ismartRecruitIDPattern = regexp.MustCompile(`_(\d+)_W_`)

// ismartRecruitExternalID returns a posting's stable dedup id: the numeric id decoded from the
// detail href's x token, or "" when the href is not a decodable posting link.
func ismartRecruitExternalID(href string) string {
	m := ismartRecruitXPattern.FindStringSubmatch(href)
	if m == nil {
		return ""
	}
	payload := ismartRecruitDecodeToken(m[1])
	if payload == "" {
		return ""
	}
	return firstSubmatch(ismartRecruitIDPattern, payload)
}

// ismartRecruitDecodeToken unwraps the E7p…Q9e envelope and base64-decodes the payload, tolerating
// padded or raw/URL alphabets, returning "" for a token that is not a printable decode.
func ismartRecruitDecodeToken(tok string) string {
	if !strings.HasPrefix(tok, ismartRecruitTokenPrefix) || !strings.HasSuffix(tok, ismartRecruitTokenSuffix) {
		return ""
	}
	mid := tok[len(ismartRecruitTokenPrefix) : len(tok)-len(ismartRecruitTokenSuffix)]
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if dec, err := enc.DecodeString(mid); err == nil && isPrintable(string(dec)) {
			return string(dec)
		}
	}
	return ""
}
