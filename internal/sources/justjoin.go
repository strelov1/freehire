package sources

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/skilltag"
	"github.com/strelov1/freehire/internal/vocab"
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
	// justJoinDetailURL is the per-offer detail endpoint carrying the posting body the list
	// endpoint omits (a `/v1` route, distinct from the `/v2` list base).
	justJoinDetailURL = "https://api.justjoin.it/v1/offers/%s"
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

// Fetch is the list-only crawl (no description): kept for back-compat and as the fallback for
// callers that do not drive hydration. FetchNew is the hydrating path used by ingest.
func (s justjoin) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	offers, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, o := range offers {
		if job, ok := o.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew is the hydrating crawl: it pages the same list, but fetches a posting's detail (the
// body the list omits) only for an offer the catalogue does not already have — seen reports
// whether an offer's guid is already ingested. A seen offer yields the list-only job (no detail
// request); an unseen offer is hydrated with its detail; a single offer's detail failure is
// isolated (logged, falling back to list-only so the offer is still ingested). Detail fetches
// run under the shared bounded worker pool.
func (s justjoin) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	offers, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(offers, defaultDetailWorkers, func(o justJoinOffer) (Job, bool) {
		base, ok := o.toJob()
		if !ok {
			return Job{}, false // unusable offer — dropped, as in Fetch
		}
		if seen(o.GUID) {
			// Already ingested: refresh liveness only, no detail request. The pipeline
			// must not re-upsert content — that would wipe the description/facets
			// hydrated when this offer was new (an empty description re-derives to empty
			// facets). base carries just the identity fields toJob set.
			base.SeenRefresh = true
			return base, true
		}
		d, ok := s.detail(ctx, o.Slug)
		if !ok {
			log.Printf("justjoin: detail %q failed; ingesting list-only", o.Slug)
			return base, true
		}
		return d.apply(base), true
	}), nil
}

// crawl pages the cursor feed and returns every raw offer — the shared list walk behind Fetch
// and FetchNew.
func (s justjoin) crawl(ctx context.Context) ([]justJoinOffer, error) {
	var offers []justJoinOffer
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
		offers = append(offers, resp.Data...)
		// Stop at the end of the feed, or whenever the cursor fails to advance — a
		// non-increasing cursor means the page param was ignored (refetching page 1) or
		// the feed looped, so one extra request is the worst case, never an endless loop.
		if resp.Meta.Next == nil || resp.Meta.Next.Cursor <= cursor {
			break
		}
		cursor = resp.Meta.Next.Cursor
	}
	return offers, nil
}

// justJoinDetail is the per-offer detail payload (GET /v1/offers/{slug}). It carries the
// posting body the list omits, plus the structured facets justjoin states.
type justJoinDetail struct {
	Body            string          `json:"body"`
	RequiredSkills  []justJoinSkill `json:"requiredSkills"`
	ExperienceLevel struct {
		Value string `json:"value"`
	} `json:"experienceLevel"`
}

// justJoinSkill is one required-skill entry (only the canonical name is used).
type justJoinSkill struct {
	Name string `json:"name"`
}

// detail fetches an offer's detail, returning ok=false on a failed request so the caller
// falls back to the list-only job — an offer is never dropped over a missing detail.
func (s justjoin) detail(ctx context.Context, slug string) (justJoinDetail, bool) {
	var d justJoinDetail
	if err := s.http.GetJSON(ctx, fmt.Sprintf(justJoinDetailURL, slug), &d); err != nil {
		return justJoinDetail{}, false
	}
	return d, true
}

// apply enriches a list-derived job with the detail payload: the sanitized body becomes the
// description, and the structured facets justjoin states unambiguously are mapped into
// freehire's vocabularies (empty when unmapped, so the pipeline's dictionaries decide).
// Category is deliberately not set — justjoin's category is a language/stack tag that does not
// pin a single freehire role category, so the title dictionary decides it.
func (d justJoinDetail) apply(base Job) Job {
	base.Description = sanitizeHTML(d.Body)
	base.Skills = justJoinSkills(d.RequiredSkills)
	base.Seniority = justJoinSeniority(d.ExperienceLevel.Value)
	return base
}

// justJoinSkills canonicalizes the required skills through the skilltag dictionary, keeping
// only resolved technologies. Canonicalize, not Parse: these are already-discrete asserted
// names from a structured field, not prose to mine, so the corroboration rule Parse applies
// to free text must not drop an unambiguous single-word name for lack of a second strong term.
func justJoinSkills(skills []justJoinSkill) []string {
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Name)
	}
	return skilltag.Canonicalize(names)
}

// justJoinSeniority maps justjoin's experience level to freehire's seniority vocabulary.
// justjoin names the middle grade "mid"; the rest (intern/junior/senior/c_level) share
// spelling, so after that one rename vocabulary membership IS the map — an unrecognized level
// (e.g. "manager", which is not a freehire seniority) drops rather than being guessed.
func justJoinSeniority(level string) string {
	l := strings.ToLower(strings.TrimSpace(level))
	if l == "mid" {
		l = "middle"
	}
	if slices.Contains(vocab.SeniorityValues, l) {
		return l
	}
	return ""
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
