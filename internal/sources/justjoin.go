package sources

import (
	"context"
	"fmt"
)

// justjoin adapts justjoin.it, a Polish/CEE IT job marketplace. Boardless (one public API,
// no per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's company from the feed. The by-cursor endpoint pages by a numeric cursor returned
// in meta.next; the list response has no apply link, so the canonical URL is synthesized from
// the posting slug.
type justjoin struct {
	http JSONGetter
}

const (
	// justJoinMaxPages caps pagination as a backstop (the strictly-increasing cursor guard
	// below is the primary stop; this bounds a pathological feed).
	justJoinMaxPages = 600
	justJoinBaseURL  = "https://api.justjoin.it/v2/user-panel/offers/by-cursor"
	// justJoinOfferURL builds the canonical detail URL the feed omits, from the slug.
	justJoinOfferURL = "https://justjoin.it/job-offer/%s"
)

// NewJustJoin builds the JustJoin adapter over the given HTTP client.
func NewJustJoin(c JSONGetter) Source { return justjoin{http: c} }

func (justjoin) Provider() string { return "justjoin" }

func (justjoin) boardless() {}

func (justjoin) aggregator() {}

// justJoinResponse is one cursor page: the offers plus the cursor for the next page (nil at
// the end of the feed).
type justJoinResponse struct {
	Data []justJoinOffer `json:"data"`
	Meta struct {
		Next *struct {
			Cursor int `json:"cursor"`
		} `json:"next"`
	} `json:"meta"`
}

// justJoinOffer is one offer. publishedAt is RFC3339 with millisecond precision.
type justJoinOffer struct {
	GUID          string `json:"guid"`
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	CompanyName   string `json:"companyName"`
	City          string `json:"city"`
	WorkplaceType string `json:"workplaceType"`
	PublishedAt   string `json:"publishedAt"`
}

func (s justjoin) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	cursor := 0
	for page := 0; page < justJoinMaxPages; page++ {
		url := justJoinBaseURL
		if cursor > 0 {
			// The cursor value from meta.next.cursor is passed back as the `from` query
			// parameter (verified live — `?cursor=` is silently ignored and never advances).
			url = fmt.Sprintf("%s?from=%d", justJoinBaseURL, cursor)
		}
		var resp justJoinResponse
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("justjoin: list cursor %d: %w", cursor, err)
		}
		for _, o := range resp.Data {
			if job, ok := o.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		// Stop at the end of the feed, or whenever the cursor fails to advance — a
		// non-increasing cursor means the page param was ignored (refetching page 1) or
		// the feed looped, so one extra request is the worst case, never an endless loop.
		if resp.Meta.Next == nil || resp.Meta.Next.Cursor <= cursor {
			break
		}
		cursor = resp.Meta.Next.Cursor
	}
	return jobs, nil
}

// toJob maps an offer to a Job, returning ok=false for an unusable offer (no slug to build
// the URL, no guid to key on, or no company which would break the slug).
func (o justJoinOffer) toJob() (Job, bool) {
	if o.Slug == "" || o.GUID == "" || o.CompanyName == "" {
		return Job{}, false
	}
	return Job{
		ExternalID: o.GUID,
		URL:        fmt.Sprintf(justJoinOfferURL, o.Slug),
		Title:      o.Title,
		Company:    o.CompanyName,
		Location:   o.City,
		WorkMode:   workplaceTypeMode(o.WorkplaceType),
		PostedAt:   parseRFC3339(o.PublishedAt),
	}, true
}
