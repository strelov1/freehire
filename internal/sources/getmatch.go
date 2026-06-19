package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// getmatch adapts getmatch.ru, a curated Russian IT job marketplace. Its public, keyless feed
// (/api/offers) returns a paginated list where every offer carries its own employer, so one
// paged crawl assembles every Job — but the list description is a short summary, so each offer
// is enriched from the per-offer detail endpoint (/api/offers/{id}) for the full HTML body.
// Unlike a single-company adapter, the company comes from the offer (the marketplace lists many
// employers), so its boardless config entry's company is only a validation placeholder.
type getmatch struct {
	http JSONGetter
}

const (
	getmatchBaseURL   = "https://getmatch.ru"
	getmatchListURL   = "https://getmatch.ru/api/offers?offset=%d&limit=%d"
	getmatchDetailURL = "https://getmatch.ru/api/offers/%d"
	getmatchPageLimit = 100
	// getmatchMaxPages bounds pagination so a wrong or missing meta.total cannot loop.
	getmatchMaxPages = 200
	// getmatchDateLayout matches the zone-less published_at getmatch emits
	// ("2026-06-19T12:55:17.948391"). The .9 fractional form parses any sub-second width.
	getmatchDateLayout = "2006-01-02T15:04:05.999999999"
)

// NewGetmatch builds the getmatch adapter over the given HTTP client.
func NewGetmatch(c JSONGetter) Source { return getmatch{http: c} }

func (getmatch) Provider() string { return "getmatch" }

// getmatch is a marketplace with one global feed, so its config entries carry no board.
func (getmatch) boardless() {}

// getmatch aggregates postings from many companies, so it stays in the source facet.
func (getmatch) aggregator() {}

// getmatchListResponse is the /api/offers page: Meta.Total is the catalogue size used to stop
// pagination; Offers is the page.
type getmatchListResponse struct {
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
	Offers []getmatchOffer `json:"offers"`
}

// getmatchOffer is one posting. Company nests the employer's own name (the marketplace lists
// many employers); OfferDescription is the list summary and Description the full detail body.
type getmatchOffer struct {
	ID               int    `json:"id"`
	Position         string `json:"position"`
	URL              string `json:"url"`
	PublishedAt      string `json:"published_at"`
	OfferDescription string `json:"offer_description"`
	Description      string `json:"description"`
	Company          struct {
		Name string `json:"name"`
	} `json:"company"`
	LocationItems []getmatchLocation `json:"location_items"`
}

// getmatchLocation is one of an offer's locations: a display Label and a Format that is either
// a work mode (remote/hybrid/office) or a relocation flag (relocation_company/_candidate).
type getmatchLocation struct {
	Label  string `json:"label"`
	Format string `json:"format"`
}

func (g getmatch) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 0; page < getmatchMaxPages; page++ {
		offset := page * getmatchPageLimit
		var resp getmatchListResponse
		if err := g.http.GetJSON(ctx, fmt.Sprintf(getmatchListURL, offset, getmatchPageLimit), &resp); err != nil {
			if offset == 0 {
				return nil, fmt.Errorf("getmatch: list offset %d: %w", offset, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}
		if len(resp.Offers) == 0 {
			break
		}
		for _, o := range resp.Offers {
			jobs = append(jobs, g.toJob(ctx, o))
		}
		if resp.Meta.Total > 0 && offset+getmatchPageLimit >= resp.Meta.Total {
			break
		}
	}
	return jobs, nil
}

// toJob maps an offer to a Job. The company is the offer's own employer, not the configured
// entry; the work mode is structured only when the offer's locations agree on one (see
// getmatchWorkMode).
func (g getmatch) toJob(ctx context.Context, o getmatchOffer) Job {
	mode := getmatchWorkMode(o.LocationItems)
	return Job{
		ExternalID:  strconv.Itoa(o.ID),
		URL:         getmatchBaseURL + o.URL,
		Title:       o.Position,
		Company:     o.Company.Name,
		Description: sanitizeHTML(g.description(ctx, o)),
		Location:    getmatchLocationString(o.LocationItems),
		Remote:      mode == "remote",
		WorkMode:    mode,
		PostedAt:    parseLayout(getmatchDateLayout, o.PublishedAt),
	}
}

// description returns the offer's full HTML body from the detail endpoint, falling back to the
// list summary when the detail body is empty (e.g. event cards) or its request fails, so an
// offer is never dropped over a missing description.
func (g getmatch) description(ctx context.Context, o getmatchOffer) string {
	var detail getmatchOffer
	if err := g.http.GetJSON(ctx, fmt.Sprintf(getmatchDetailURL, o.ID), &detail); err == nil {
		if strings.TrimSpace(detail.Description) != "" {
			return detail.Description
		}
	}
	return o.OfferDescription
}

// getmatchWorkMode derives the structured work mode from the offer's location formats, mapping
// remote/hybrid/office via the shared workplaceTypeMode (which yields "" for the relocation
// flags, so they are ignored). It returns a mode only when the offer's locations resolve to a
// single distinct one; a mix (or none) yields "" so the pipeline's location parser decides.
func getmatchWorkMode(items []getmatchLocation) string {
	var mode string
	for _, it := range items {
		m := workplaceTypeMode(it.Format)
		if m == "" {
			continue
		}
		if mode == "" {
			mode = m
		} else if mode != m {
			return ""
		}
	}
	return mode
}

// getmatchLocationString joins an offer's distinct location labels in order.
func getmatchLocationString(items []getmatchLocation) string {
	var labels []string
	seen := map[string]struct{}{}
	for _, it := range items {
		l := strings.TrimSpace(it.Label)
		if l == "" {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		labels = append(labels, l)
	}
	return strings.Join(labels, ", ")
}
