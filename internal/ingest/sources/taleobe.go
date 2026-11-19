package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// taleobe adapts Oracle Taleo Business Edition (TBE), the small-business product on
// <pod>.tbe.taleo.net. It is a different platform from Taleo Enterprise (taleo.go): different
// hosts, different URLs, no careersection, no session-bound search.
//
// The board is "<ORG>:<cws>" — the employer's org code and the number of one of its career
// sites (an org may run several, e.g. external and internal), the two query parameters every
// TBE URL carries (requisition.jsp?org=ADELPHI&cws=43&rid=3413). The pod an org lives on
// (phf02, tre01, …) is deliberately NOT part of the board: the platform's dispatcher resolves
// an org to its pod from any TBE host, and orgs move between pods, so a board keyed on the pod
// would 404 after a move.
//
// Every career site publishes its whole listing as one RSS document with the full HTML body
// inline per item (taleo:html-description), so a board costs one feed request — reached
// through the dispatcher, which 302s to the org's pod — with no pagination and no detail
// request. The feed carries exactly the "Open Positions" total the HTML career site states.
//
// How the platform says a board is gone is a redirect, never a status: an unknown org lands
// on the dispatcher's sign-in page, a deactivated org on a static 404 page (served 200), and a
// career-site number the org does not have on rssErrMsg.jsp ("Job Not Available").
type taleobe struct {
	http XMLResolvedGetter
}

// NewTaleoBE builds the Taleo Business Edition adapter over the given HTTP client.
func NewTaleoBE(c XMLResolvedGetter) Source { return taleobe{http: c} }

func (taleobe) Provider() string { return "taleobe" }

// fullBoardListing: the feed is a single unpaginated document listing every open posting of
// the career site. See the fullBoardListing interface for the bar.
func (taleobe) FullBoardListing() {}

// taleobeFeed is the RSS document a career site publishes.
type taleobeFeed struct {
	Items []taleobeItem `xml:"channel>item"`
}

// taleobeItem is one open posting. Location is the place as the career site displays it
// ("Remote, United States", "Nairobi, Kenya"); the City/State/Country parts are filled only
// when the employer attached a structured address, and State is inconsistent across orgs
// ("Louisiana", "US-DC"), so only the country code is read as a structured fact. The taleo:
// elements are bound by namespace URI, so they match whatever prefix the feed declares.
type taleobeItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
	Text        string `xml:"description"`
	HTML        string `xml:"urn:TBERss html-description"`
	ReqID       string `xml:"urn:TBERss reqId"`
	Location    string `xml:"urn:TBERss location"`
	City        string `xml:"urn:TBERss locationCity"`
	State       string `xml:"urn:TBERss locationState"`
	CountryCode string `xml:"urn:TBERss locationCountry"`
}

func (t taleobe) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	feedURL, err := TaleoBEFeedURL(e.Board)
	if err != nil {
		return nil, err
	}

	var feed taleobeFeed
	final, err := t.http.GetXMLResolved(ctx, feedURL, &feed)
	// The redirect is checked before the error: a gone board lands on an HTML page whose
	// decode failure is what the redirect CAUSES and says nothing about the board.
	if taleobeBoardGone(final) {
		return nil, fmt.Errorf("taleobe: board %q: %w (redirected to %s)", e.Board, ErrBoardGone, final)
	}
	if err != nil {
		return nil, fmt.Errorf("taleobe: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(feed.Items))
	for _, it := range feed.Items {
		id := strings.TrimSpace(it.ReqID)
		if id == "" {
			id = taleobeReqID(it.Link)
		}
		if id == "" {
			continue
		}
		location := firstNonEmpty(strings.TrimSpace(it.Location), joinNonEmpty(strings.TrimSpace(it.City), strings.TrimSpace(it.State)))
		body := it.HTML
		if strings.TrimSpace(body) == "" {
			body = it.Text
		}
		jobs = append(jobs, Job{
			ExternalID:  id,
			URL:         strings.Replace(strings.TrimSpace(it.Link), "http://", "https://", 1),
			Title:       strings.TrimSpace(it.Title),
			Company:     e.Company,
			Location:    location,
			Description: sanitizeHTML(body),
			Remote:      isRemote(location),
			PostedAt:    parsePubDate(strings.TrimSpace(it.PubDate)),
			Countries:   countryFromCode(strings.TrimSpace(it.CountryCode)),
		})
	}
	return jobs, nil
}

// taleobeOrg is the shape of an org code as the platform writes it (ADELPHI, SUN_PHARMA,
// NBF1199). The platform resolves it case-insensitively; the catalogue stores it upper-case.
var taleobeOrg = regexp.MustCompile(`^[A-Z0-9_]+$`)

// taleobeCWS is a career-site number.
var taleobeCWS = regexp.MustCompile(`^[0-9]+$`)

// TaleoBEFeedURL returns the RSS feed of a TBE board ("<ORG>:<cws>"), addressed through the
// platform's dispatcher so the request reaches the org's pod wherever it currently lives.
// Exported for cmd/harvest-boards' prober, which validates a board with this same request.
func TaleoBEFeedURL(board string) (string, error) {
	org, cws, ok := strings.Cut(board, ":")
	if !ok || !taleobeOrg.MatchString(org) || !taleobeCWS.MatchString(cws) {
		return "", fmt.Errorf("taleobe: board %q must be \"<ORG>:<cws>\"", board)
	}
	feed := "https://tbe.taleo.net/ats/servlet/Rss?" + url.Values{"org": {org}, "cws": {cws}}.Encode()
	return "https://tbe.taleo.net/dispatcher/servlet/DispatcherServlet?" + url.Values{
		"org":         {org},
		"act":         {"redirectOrg"},
		"redirectUrl": {feed},
	}.Encode(), nil
}

// taleobeBoardGone reports whether the feed request was redirected to one of the platform's
// "no such board" pages (see the type comment). An empty final URL is a request that got no
// response at all, which is a transport failure and says nothing about the board.
func taleobeBoardGone(final string) bool {
	u, err := url.Parse(final)
	if final == "" || err != nil {
		return false
	}
	return strings.HasSuffix(u.Path, "/dispatcher/login.jsp") ||
		strings.HasPrefix(u.Path, "/errorpages/") ||
		strings.HasSuffix(u.Path, "/rssErrMsg.jsp")
}

// taleobeReqID extracts the requisition id from a posting link
// (…/requisition.jsp?org=ADELPHI&cws=43&rid=3413 → 3413), or "" when it has none.
func taleobeReqID(link string) string {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return ""
	}
	return u.Query().Get("rid")
}
