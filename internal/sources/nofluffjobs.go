package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/skilltag"
)

// nofluffjobs adapts nofluffjobs.com, a Polish/CEE IT job board (the complement to justjoin).
// Boardless (one public API, no per-tenant board) and multi-company, so it stays in the source
// facet and takes each posting's company from the feed. The listing carries structured facets but
// no description, so — like justjoin — the description is hydrated per posting from a detail
// endpoint, and only for postings not already ingested (HydratingSource.FetchNew).
type nofluffjobs struct {
	http nofluffjobsHTTP
}

// nofluffjobsHTTP is the two-stage transport: a streamed listing (the ~60 MB document exceeds the
// size-capped GetJSON) and a per-posting JSON detail.
type nofluffjobsHTTP interface {
	GetStream(ctx context.Context, url, accept string, fn func(io.Reader) error) error
	GetJSON(ctx context.Context, url string, v any) error
}

const (
	nofluffjobsListURL   = "https://nofluffjobs.com/api/posting"
	nofluffjobsOfferURL  = "https://nofluffjobs.com/job/%s"
	nofluffjobsDetailURL = "https://nofluffjobs.com/api/posting/%s"
)

// NewNoFluffJobs builds the NoFluffJobs adapter over the given two-stage client.
func NewNoFluffJobs(c nofluffjobsHTTP) Source { return nofluffjobs{http: c} }

func (nofluffjobs) Provider() string { return "nofluffjobs" }

func (nofluffjobs) boardless() {}

func (nofluffjobs) aggregator() {}

// nofluffjobsListing is the single listing document.
type nofluffjobsListing struct {
	Postings []nofluffjobsPosting `json:"postings"`
}

// nofluffjobsPosting is one listing entry. posted is epoch milliseconds; technology is a single
// skill string; seniority is an array whose first entry is the grade.
type nofluffjobsPosting struct {
	ID          string   `json:"id"`
	URL         string   `json:"url"`
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Technology  string   `json:"technology"`
	Seniority   []string `json:"seniority"`
	FullyRemote bool     `json:"fullyRemote"`
	Posted      int64    `json:"posted"`
	Location    struct {
		Places []struct {
			City    string `json:"city"`
			Country struct {
				Name string `json:"name"`
			} `json:"country"`
		} `json:"places"`
	} `json:"location"`
}

// nofluffjobsDetail is the per-posting detail payload; the description is split across the offer
// (details.description) and the requirements (requirements.description) HTML sections.
type nofluffjobsDetail struct {
	Details struct {
		Description string `json:"description"`
	} `json:"details"`
	Requirements struct {
		Description string `json:"description"`
	} `json:"requirements"`
}

// crawl streams and decodes the listing document — the shared list walk behind Fetch and FetchNew.
func (s nofluffjobs) crawl(ctx context.Context) ([]nofluffjobsPosting, error) {
	var doc nofluffjobsListing
	err := s.http.GetStream(ctx, nofluffjobsListURL, "application/json", func(r io.Reader) error {
		return json.NewDecoder(r).Decode(&doc)
	})
	if err != nil {
		return nil, fmt.Errorf("nofluffjobs: listing: %w", err)
	}
	return doc.Postings, nil
}

// Fetch is the list-only crawl (no description) — the fallback for a non-hydrating pipeline.
func (s nofluffjobs) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	postings, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew is the hydrating crawl: it fetches a posting's detail description only for a posting the
// catalogue does not already have (seen). A seen posting yields the list-only job (no detail
// request, marked SeenRefresh so the pipeline refreshes liveness without wiping the hydrated body);
// an unseen posting is hydrated; a single detail failure is isolated (logged, list-only fallback).
func (s nofluffjobs) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p nofluffjobsPosting) (Job, bool) {
		base, ok := p.toJob()
		if !ok {
			return Job{}, false // unusable posting — dropped, as in Fetch
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			return base, true
		}
		d, ok := s.detail(ctx, p.URL)
		if !ok {
			log.Printf("nofluffjobs: detail %q failed; ingesting list-only", p.URL)
			return base, true
		}
		base.Description = d.description()
		return base, true
	}), nil
}

// detail fetches a posting's detail, returning ok=false on a failed request so the caller falls
// back to the list-only job — a posting is never dropped over a missing detail.
func (s nofluffjobs) detail(ctx context.Context, slug string) (nofluffjobsDetail, bool) {
	var d nofluffjobsDetail
	if err := s.http.GetJSON(ctx, fmt.Sprintf(nofluffjobsDetailURL, slug), &d); err != nil {
		return nofluffjobsDetail{}, false
	}
	return d, true
}

// description assembles the sanitized body from the offer and requirements HTML sections.
func (d nofluffjobsDetail) description() string {
	parts := make([]string, 0, 2)
	for _, s := range []string{d.Details.Description, d.Requirements.Description} {
		if strings.TrimSpace(s) != "" {
			parts = append(parts, s)
		}
	}
	return sanitizeHTML(strings.Join(parts, "\n"))
}

// toJob maps a listing posting to a Job, returning ok=false for an unusable posting (no id to key
// on, no url slug to build the canonical URL, or no company which would break the slug). Skills and
// seniority come straight from the listing — no detail request needed for the structured facets.
func (p nofluffjobsPosting) toJob() (Job, bool) {
	if p.ID == "" || p.URL == "" || p.Name == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID: p.ID,
		URL:        fmt.Sprintf(nofluffjobsOfferURL, p.URL),
		Title:      strings.TrimSpace(p.Title),
		Company:    strings.TrimSpace(p.Name),
		Location:   nofluffjobsLocation(p),
		Remote:     p.FullyRemote,
		Skills:     skilltag.Parse(p.Technology),
		Seniority:  nofluffjobsSeniority(p.Seniority),
	}
	if p.FullyRemote {
		job.WorkMode = "remote"
	}
	if p.Posted > 0 {
		t := time.UnixMilli(p.Posted).UTC()
		job.PostedAt = &t
	}
	return job, true
}

// nofluffjobsLocation is the first place's "City, Country", or empty when the posting lists no place.
func nofluffjobsLocation(p nofluffjobsPosting) string {
	if len(p.Location.Places) == 0 {
		return ""
	}
	place := p.Location.Places[0]
	parts := make([]string, 0, 2)
	for _, s := range []string{place.City, place.Country.Name} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

// nofluffjobsSeniority maps NoFluffJobs' first seniority grade into freehire's vocabulary. Its
// ladder is Trainee < Junior < Mid < Senior < Expert; three names differ from freehire's and are
// renamed (mid→middle, trainee→intern, expert→principal, the top individual-contributor grade).
// After the renames, vocabulary membership is the map — an unrecognized grade drops rather than
// being guessed (the title dictionary then decides).
func nofluffjobsSeniority(levels []string) string {
	if len(levels) == 0 {
		return ""
	}
	l := strings.ToLower(strings.TrimSpace(levels[0]))
	switch l {
	case "mid":
		l = "middle"
	case "trainee":
		l = "intern"
	case "expert":
		l = "principal"
	}
	if slices.Contains(enrich.SeniorityValues, l) {
		return l
	}
	return ""
}
