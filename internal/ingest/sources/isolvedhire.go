package sources

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// isolvedFamily adapts the iSolved Hire and ApplicantPro career sites — one Vue-based platform
// served under two host domains. The board is the tenant subdomain, forming host
// "<board>.<host>". A company's postings are enumerated from its sitemap.xml (the /jobs/<id>
// URLs); each posting's fields come from that detail page's schema.org JobPosting ld+json —
// the sitemap-plus-ld+json-detail shape shared with successfactors/clinch.
type isolvedFamily struct {
	http     isolvedHTTP
	provider string
	host     string
}

// isolvedHTTP is the client capability the family needs: the sitemap as a stream (some tenants'
// sitemaps run past the buffered-body cap — see Fetch) and each detail page as parsed HTML (for
// its ld+json).
type isolvedHTTP interface {
	StreamGetter
	HTMLGetter
}

// NewIsolvedHire builds the *.isolvedhire.com adapter.
func NewIsolvedHire(c isolvedHTTP) Source {
	return isolvedFamily{http: c, provider: "isolvedhire", host: "isolvedhire.com"}
}

// NewApplicantPro builds the *.applicantpro.com adapter (same platform, different host).
func NewApplicantPro(c isolvedHTTP) Source {
	return isolvedFamily{http: c, provider: "applicantpro", host: "applicantpro.com"}
}

func (s isolvedFamily) Provider() string { return s.provider }

// isolvedJobID captures the numeric posting id from a /jobs/<id> URL. The sitemap lists both
// /jobs/<id> and /jobs/<id>.html plus marketing/classification pages, so the id (deduped) is
// the stable enumeration key.
var isolvedJobID = regexp.MustCompile(`/jobs/(\d+)`)

// isolvedNoticePeek is how much of the sitemap stream is inspected for a platform notice.
// Both notices are the document's first line and well under 200 bytes; the margin covers a
// leading BOM or doctype without reaching into a real sitemap's first <url> entry.
const isolvedNoticePeek = 512

// isolvedNotices are the platform's own answers for a board it does not serve, each held as
// the stable fragment of its sentence rather than the whole line: the notices are wrapped in
// marketing markup that changes, and a predicate matching the full sentence would silently
// stop matching after a reword — which is the failure this whole check exists to end.
//
// Both count as gone, though they do not mean the same thing. The first is a subdomain that
// resolves to no tenant at all. The second is a tenant whose site the vendor switched off,
// which an unpaid invoice and a departed company produce alike, so it may return. It is
// counted anyway because the alternative is worse in both directions: the 39 prod boards in
// that state have been failing since July under a message that explains nothing, and a board
// retired by mistake is one command to restore, whereas one that keeps failing quietly is
// only ever found by an audit like the one that produced this list.
var isolvedNotices = []string{
	"typed the url for this website incorrectly",
	"career site has been disabled",
}

// isolvedBoardGone reports whether the head of a sitemap response is one of those notices
// rather than a sitemap. Matched case-insensitively: the fragments are prose, and prose that
// gets recapitalised is the same answer.
func isolvedBoardGone(head string) bool {
	lower := strings.ToLower(head)
	for _, notice := range isolvedNotices {
		if strings.Contains(lower, notice) {
			return true
		}
	}
	return false
}

func (s isolvedFamily) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	smURL := fmt.Sprintf("https://%s.%s/sitemap.xml", e.Board, s.host)

	// The sitemap is streamed and scanned <loc>-by-<loc> rather than buffered whole: a large
	// tenant's sitemap runs past the buffered-body size cap (35 MiB+ seen in prod), which would
	// truncate it mid-element and fail the XML decode. Streaming reads it in full at any size.
	seen := map[string]struct{}{}
	var ids []string
	err := s.http.GetStream(ctx, smURL, "application/xml", func(r io.Reader) error {
		// A tenant the platform no longer serves answers 200 with an HTML notice at this
		// URL rather than a 404, so the head of the stream is inspected before it reaches
		// the XML decoder — otherwise the notice arrives as a syntax error on line 3 and
		// says nothing about the board. Peeking leaves the bytes in place for the decoder.
		br := bufio.NewReader(r)
		head, err := br.Peek(isolvedNoticePeek)
		if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
			return err
		}
		if isolvedBoardGone(string(head)) {
			return ErrBoardGone
		}

		dec := xml.NewDecoder(br)
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			start, ok := tok.(xml.StartElement)
			if !ok || start.Name.Local != "loc" {
				continue
			}
			var loc string
			if err := dec.DecodeElement(&loc, &start); err != nil {
				return err
			}
			if m := isolvedJobID.FindStringSubmatch(loc); m != nil {
				if _, ok := seen[m[1]]; !ok {
					seen[m[1]] = struct{}{}
					ids = append(ids, m[1])
				}
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("%s: sitemap %q: %w", s.provider, e.Board, err)
	}

	return fetchDetails(ids, defaultDetailWorkers, func(id string) (Job, bool) {
		return s.detail(ctx, e, id)
	}), nil
}

// detail fetches one posting's detail page and maps its JobPosting ld+json to a Job, returning
// ok=false when the fetch fails or the page carries no JobPosting.
func (s isolvedFamily) detail(ctx context.Context, e CompanyEntry, id string) (Job, bool) {
	loc := fmt.Sprintf("https://%s.%s/jobs/%s", e.Board, s.host, id)
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p isolvedPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()

	// datePosted is a space-separated "2006-01-02 15:04:05" with no zone; the date part is
	// the reliable signal, so posted_at is date-granularity.
	posted := p.DatePosted
	if len(posted) > 10 {
		posted = posted[:10]
	}

	return Job{
		ExternalID:  id,
		URL:         loc,
		Title:       p.Title,
		Company:     firstNonEmpty(e.Company, p.HiringOrganization.Name),
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location),
		PostedAt:    parseDate(posted),
	}, true
}

// isolvedPosting is the schema.org JobPosting decoded from a detail page's ld+json.
type isolvedPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlace `json:"jobLocation"`
}
