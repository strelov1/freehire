package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
)

// homerun adapts Homerun (homerun.co), a Dutch ATS for small and mid-sized employers whose
// career site lives at https://<board>.homerun.co — or on the employer's own domain
// (vacatures.douglas.nl, werkenbijautokan.nl), which the tenant host redirects to. Whatever the
// domain, every tenant publishes one open Atom feed at https://feed.homerun.co/<board>: every
// open vacancy with its full body, department, location name, job type and publication time,
// unpaginated (douglas: 106 entries). That feed is the listing.
//
// What the feed lacks is structured geography: its location is the name the employer gave an
// office ("Douglas Hoofdkantoor - Nijmegen", "Preston Palace", "Remote", "Other"). The vacancy's
// own page carries a schema.org JobPosting with that office's postal address and ISO country —
// on 1,332 of the 2,043 pages that answered (2026-10-25); the rest carry no JobPosting or an
// empty block, which is what Homerun emits for an office with no address. So a NEW vacancy costs
// one page request (HydratingSource), and the page is only enrichment: when it cannot be read
// the feed's own entry is stored as it stands. The one answer that changes the outcome is a 404:
// 62 of those 2,499 feed entries (2.5%) named a page Homerun answers "not found" for — a
// vacancy the feed still carries after it was taken down — and those are dropped.
//
// An unknown or switched-off tenant's feed redirects to 404.homerun.co, which answers 404:
// ErrBoardGone. The tenant HOST is no such signal: 115 tenants whose site redirects to
// 404.homerun.co still serve live vacancies on their feed.
type homerun struct {
	http homerunGetter
}

// homerunGetter is the transport the adapter needs: the feed as XML, each vacancy page as HTML.
type homerunGetter interface {
	XMLGetter
	HTMLGetter
}

// NewHomerun builds the Homerun adapter over the given HTTP client.
func NewHomerun(c homerunGetter) Source { return homerun{http: c} }

func (homerun) Provider() string { return "homerun" }

// fullBoardListing: the feed is one unpaginated document carrying every open vacancy.
func (homerun) fullBoardListing() {}

// homerunBoard is the shape of a tenant label.
var homerunBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// homerunFeed is the Atom document. XMLName is checked so an answer that is not the feed is told
// apart from a tenant with no openings, which Homerun serves as a feed with no entry.
type homerunFeed struct {
	XMLName xml.Name       `xml:"feed"`
	Entries []homerunEntry `xml:"entry"`
}

type homerunEntry struct {
	ID    string `xml:"id"`
	Title string `xml:"title"`
	Links []struct {
		Rel  string `xml:"rel,attr"`
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Description string `xml:"description"`
	Content     string `xml:"content"`
	Location    string `xml:"location>name"`
	Type        string `xml:"type>name"`
	Updated     string `xml:"updated"`
}

// pageURL is the entry's vacancy page, the link the feed marks alternate.
func (e homerunEntry) pageURL() string {
	for _, l := range e.Links {
		if strings.TrimSpace(l.Rel) == "alternate" {
			return strings.TrimSpace(l.Href)
		}
	}
	return ""
}

// Fetch reads the feed and every vacancy's page.
func (s homerun) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	entries, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(entries, defaultDetailWorkers, func(en homerunEntry) (Job, bool) {
		return s.detail(ctx, e, en)
	}), nil
}

// FetchNew is Fetch that spends a page request only on vacancies the catalogue does not hold; a
// seen vacancy is re-listed as a SeenRefresh, the feed itself proving it is still open.
func (s homerun) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	entries, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(entries, defaultDetailWorkers, func(en homerunEntry) (Job, bool) {
		if seen(en.ID) {
			return Job{ExternalID: en.ID, URL: en.pageURL(), Title: strings.TrimSpace(en.Title),
				Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, en)
	}), nil
}

// list reads the tenant's feed, dropping any entry without the id a close needs or the page a
// visitor would open.
func (s homerun) list(ctx context.Context, rawBoard string) ([]homerunEntry, error) {
	board := strings.ToLower(strings.TrimSpace(rawBoard))
	if !homerunBoard.MatchString(board) {
		return nil, fmt.Errorf("homerun: board %q is not a tenant label", rawBoard)
	}
	var feed homerunFeed
	if err := s.http.GetXML(ctx, "https://feed.homerun.co/"+board, &feed); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return nil, fmt.Errorf("homerun: board %q: %w", board, ErrBoardGone)
		}
		return nil, fmt.Errorf("homerun: board %s feed: %w", board, err)
	}
	var out []homerunEntry
	seen := map[string]bool{}
	for _, en := range feed.Entries {
		en.ID = strings.TrimSpace(en.ID)
		if en.ID == "" || en.pageURL() == "" || seen[en.ID] {
			continue
		}
		seen[en.ID] = true
		out = append(out, en)
	}
	return out, nil
}

// homerunJobPosting is what the vacancy page's JobPosting adds to the feed.
type homerunJobPosting struct {
	JobLocationType string       `json:"jobLocationType"`
	JobLocation     schemaPlaces `json:"jobLocation"`
}

// detail maps a feed entry to a Job, reading the vacancy's page for its address. A 404 or 410 is
// Homerun's "not found" for a vacancy the feed still lists, and drops it, and so does a page the
// run's stopped egress did not request (homerunEgress); any other failure, or a page without a
// JobPosting, leaves the feed's entry to stand on its own, body and all.
func (s homerun) detail(ctx context.Context, e CompanyEntry, en homerunEntry) (Job, bool) {
	j := homerunJob(e, en)
	root, err := s.http.GetHTML(ctx, j.URL)
	if err != nil {
		// A page the stopped run never asked for is deferred to the next run rather than
		// stored bare: a stored vacancy's page is never read again.
		if errors.Is(err, errHomerunStopped) || !detailUnreadable(err) {
			return Job{}, false
		}
		return j, true
	}
	homerunAddPage(&j, root)
	return j, true
}

// homerunJob is the Job the feed entry alone states.
func homerunJob(e CompanyEntry, en homerunEntry) Job {
	return Job{
		ExternalID:     en.ID,
		URL:            en.pageURL(),
		Title:          strings.TrimSpace(en.Title),
		Company:        e.Company,
		Location:       strings.TrimSpace(en.Location),
		Description:    sanitizeHTML(firstNonEmpty(strings.TrimSpace(en.Description), strings.TrimSpace(en.Content))),
		EmploymentType: homerunEmploymentType(en.Type),
		PostedAt:       parseLayout("2006-01-02 15:04:05", strings.TrimSpace(en.Updated)),
	}
}

// homerunAddPage folds the page's JobPosting into the Job: the office's countries, its town
// beside the employer's own name for the office, and TELECOMMUTE as remote — the only work
// arrangement the page states (47 of 1,332 JobPostings, 2026-10-25). Its absence says nothing,
// so it never yields "onsite"; the location name ("Hybride", "Remote") stays in the text for the
// parser to read.
func homerunAddPage(j *Job, root *html.Node) {
	var jp homerunJobPosting
	if !ldJobPosting(root, &jp) {
		return
	}
	for _, pl := range jp.JobLocation {
		a := pl.Address
		if c := location.NormalizeCountry(a.AddressCountry); c != "" && !slices.Contains(j.Countries, c) {
			j.Countries = append(j.Countries, c)
		}
		town := joinNonEmpty(strings.TrimSpace(a.AddressLocality), strings.TrimSpace(a.AddressRegion))
		locality := strings.ToLower(strings.TrimSpace(a.AddressLocality))
		switch {
		case locality == "" || strings.Contains(strings.ToLower(j.Location), locality):
			// No town, or the office name already says it ("Douglas Hoofdkantoor - Nijmegen").
		case j.Location == "":
			j.Location = town
		default:
			j.Location += "; " + town
		}
	}
	if strings.EqualFold(strings.TrimSpace(jp.JobLocationType), "TELECOMMUTE") {
		j.Remote, j.WorkMode = true, workModeFromRemote(true)
	}
}

// homerunEmploymentType maps the feed's job type, which Homerun writes in English whatever the
// site's language. Measured across 2,499 entries: Full-time, Full-time / Part-time, Part-time,
// Internship, Traineeship, Freelance, Volunteer, Seasonal, Contracted, Temporary. A vacancy
// offering both full and part time states neither; a traineeship is a junior employee, not an
// internship, and a volunteer is unpaid — neither has a value in the vocabulary.
func homerunEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full-time":
		return "full_time"
	case "part-time":
		return "part_time"
	case "internship":
		return "internship"
	case "freelance", "contracted", "temporary", "seasonal":
		return "contract"
	}
	return ""
}
