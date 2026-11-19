package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"

	xhtml "golang.org/x/net/html"
)

// justremote adapts justremote.co, a remote-jobs board. Boardless (one public listing, no
// per-tenant board) and multi-company, so it stays in the source facet and takes each posting's
// company from the listing. Every posting is remote. See the "JustRemote traps" section of
// AGENTS.md for the measurements behind each rule.
//
// There is no JSON API a plain client can reach (/api/v1/* answers the SPA shell) and no
// sitemap or feed. The listing is the server-rendered page itself: /remote-jobs embeds the
// whole open inventory (~84 postings) as `window.__PRELOADED_STATE__`, which every other page
// repeats verbatim, so there is nothing to paginate. The listing carries no body; each posting
// page's schema.org JobPosting does, so the adapter hydrates only the postings the catalogue
// lacks, and — the board being ~96% reposts of Greenhouse/Lever/Ashby postings — only those of
// employers the aggregator-coverage gate will not discard.
type justremote struct {
	http justremoteHTTP
}

type justremoteHTTP interface {
	TextGetter
	HTMLGetter
}

const (
	justremoteBase       = "https://justremote.co"
	justremoteListingURL = justremoteBase + "/remote-jobs"
	// justremoteStateMarker precedes the JSON document the server renders into the page.
	justremoteStateMarker = "window.__PRELOADED_STATE__ ="
)

// NewJustRemote builds the JustRemote adapter over the given HTTP client.
func NewJustRemote(c justremoteHTTP) Source { return justremote{http: c} }

func (justremote) Provider() string { return "justremote" }

func (justremote) Boardless() {}

func (justremote) Aggregator() {}

// justremotePosting is one entry of the listing's jobsState.entity.all.
type justremotePosting struct {
	ID                  int64    `json:"id"`
	Title               string   `json:"title"`
	CompanyName         string   `json:"company_name"`
	JobType             string   `json:"job_type"`
	Href                string   `json:"href"`
	IsActive            bool     `json:"is_active"`
	LocationRestriction []string `json:"location_restrictions"`
}

// usable reports whether the posting can be stored: a native id and an employer (without which
// the company slug breaks), still open.
func (p justremotePosting) usable() bool {
	return p.ID != 0 && p.Href != "" && strings.TrimSpace(p.CompanyName) != "" && p.IsActive
}

func (p justremotePosting) url() string {
	return justremoteBase + "/" + strings.TrimPrefix(p.Href, "/")
}

// toJob maps a listing entry to a body-less Job. Every posting is "Fully Remote" (the only
// remote_type the listing has ever carried); where it may be worked from is the structured
// location_restrictions list, which is the location text so the location dictionary reads the
// countries, and "Worldwide" when the posting names none — a bare "Remote" no longer defaults
// to global.
func (p justremotePosting) toJob() Job {
	location := strings.Join(p.LocationRestriction, ", ")
	if location == "" {
		location = "Worldwide"
	}
	j := Job{
		ExternalID: strconv.FormatInt(p.ID, 10),
		URL:        p.url(),
		Title:      strings.TrimSpace(p.Title),
		Company:    strings.TrimSpace(p.CompanyName),
		Location:   location,
		Remote:     true,
		WorkMode:   "remote",
	}
	if p.JobType == "contract" {
		j.EmploymentType = "contract"
	}
	return j
}

// Fetch is the list-only fallback HydratingSource specifies: every posting, no body.
func (s justremote) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	postings, err := s.listing(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		jobs = append(jobs, p.toJob())
	}
	return jobs, nil
}

// FetchNew is the hydrating crawl: a posting already stored is a liveness refresh costing no
// request, and no content rewrite, which would wipe the body and the facets derived from it.
func (s justremote) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.listing(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, postings, seen, nil)
}

// FetchNewGated is FetchNew told which employers the coverage gate will discard. It matters
// here: 81 of the 84 postings measured on 2026-11-17 link to the employer's own Greenhouse,
// Lever or Ashby board, so without it the crawl would buy, and throw away, most bodies on every
// run. A covered posting is still yielded body-less so the gate sees and counts it.
func (s justremote) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	postings, err := s.listing(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(postings))
	for _, p := range postings {
		names = append(names, strings.TrimSpace(p.CompanyName))
	}
	return s.hydrate(ctx, postings, seen, gate.Covered(names))
}

func (s justremote) hydrate(ctx context.Context, postings []justremotePosting, seen func(string) bool, covered map[string]bool) ([]Job, error) {
	var attempted, read atomic.Int64
	jobs := fetchDetails(postings, 2, func(p justremotePosting) (Job, bool) {
		base := p.toJob()
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			return base, true
		}
		if covered[base.Company] {
			return base, true // the employer's own ATS covers it; a body would be pure loss
		}
		attempted.Add(1)
		ld, ok := s.detail(ctx, p)
		if !ok {
			// The listing is authoritative for the posting existing, so a failed body keeps it;
			// pipeline.HydrationRetryWindow re-offers the row for hydration later.
			log.Printf("justremote: body %d failed; ingesting list-only", p.ID)
			return base, true
		}
		read.Add(1)
		base.Description = sanitizeHTML(xhtml.UnescapeString(ld.Description))
		base.PostedAt = parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted))
		return base, true
	})
	// Postings were due a body and none could be read: the pages changed shape or are walled,
	// which a list-only success would report as a healthy crawl (the echojobs#2588 lesson).
	if attempted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("justremote: fetched %d posting pages and read none of them", attempted.Load())
	}
	return jobs, nil
}

// justremoteLD is the part of a posting page's schema.org JobPosting the adapter reads; the
// listing already carries everything else structurally.
type justremoteLD struct {
	Description string `json:"description"`
	DatePosted  string `json:"datePosted"`
}

func (s justremote) detail(ctx context.Context, p justremotePosting) (justremoteLD, bool) {
	root, err := s.http.GetHTML(ctx, p.url())
	if err != nil {
		return justremoteLD{}, false
	}
	var ld justremoteLD
	if !ldJobPosting(root, &ld) || strings.TrimSpace(ld.Description) == "" {
		return justremoteLD{}, false
	}
	return ld, true
}

// listing reads the open postings out of the page's embedded state. A page without the state
// document is a board-level failure — the markup changed or a wall answered 200 — never an empty
// inventory.
func (s justremote) listing(ctx context.Context) ([]justremotePosting, error) {
	page, err := s.http.GetText(ctx, justremoteListingURL)
	if err != nil {
		return nil, fmt.Errorf("justremote: listing: %w", err)
	}
	_, after, ok := strings.Cut(page, justremoteStateMarker)
	if !ok {
		return nil, fmt.Errorf("justremote: listing: no %s in the page", justremoteStateMarker)
	}
	var state struct {
		JobsState struct {
			Entity struct {
				All []justremotePosting `json:"all"`
			} `json:"entity"`
		} `json:"jobsState"`
	}
	// The decoder stops after the first JSON value, leaving the trailing ";" and markup alone.
	if err := json.NewDecoder(strings.NewReader(after)).Decode(&state); err != nil {
		return nil, fmt.Errorf("justremote: listing state: %w", err)
	}
	out := make([]justremotePosting, 0, len(state.JobsState.Entity.All))
	for _, p := range state.JobsState.Entity.All {
		if p.usable() {
			out = append(out, p)
		}
	}
	return out, nil
}
