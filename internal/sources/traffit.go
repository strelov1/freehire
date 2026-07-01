package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// traffit adapts Traffit career sites. The board is the tenant subdomain (from a public
// URL, <tenant>.traffit.com); the keyless list endpoint returns fully-populated postings —
// title, inline HTML description, and a structured geolocation — so no per-posting detail
// fetch is needed. The endpoint caps a page at traffitPageSize items, so the board is paged.
type traffit struct {
	http JSONGetter
}

// NewTraffit builds the Traffit adapter over the given HTTP client.
func NewTraffit(c JSONGetter) Source { return traffit{http: c} }

func (traffit) Provider() string { return "traffit" }

const (
	// traffitPageSize is the endpoint's maximum page size; a request for more is clamped to it.
	traffitPageSize = 100
	// traffitMaxPages bounds the walk so a feed that never reaches count (or keeps returning
	// full pages) cannot loop forever. It sits far above any real tenant's posting count.
	traffitMaxPages = 500
)

// traffitList is one page of a tenant's public advert list.
type traffitList struct {
	Count int           `json:"count"`
	Items []traffitItem `json:"items"`
}

// traffitItem is one posting in the list. Both advertId and advertPublishId are stable
// numeric ids; advertId is the dedup key, with advertPublishId as a fallback.
type traffitItem struct {
	AdvertID        int64  `json:"advertId"`
	AdvertPublishID int64  `json:"advertPublishId"`
	Title           string `json:"title"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	Description     string `json:"description"`
	Geolocation     string `json:"geolocation"`
	ValidStart      int64  `json:"validStart"`
}

// traffitGeo is the posting's structured location, carried as a JSON string in geolocation.
type traffitGeo struct {
	Locality string `json:"locality"`
	Region1  string `json:"region1"`
	Country  string `json:"country"`
}

// String renders the location as "Locality, Region, Country", dropping empty parts. An empty
// or unparseable geolocation yields "" so the pipeline's dictionary derivation stays silent.
func (g traffitGeo) String() string {
	var parts []string
	for _, p := range []string{g.Locality, g.Region1, g.Country} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

func (t traffit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page, fetched := 0, 0; page < traffitMaxPages; page++ {
		url := fmt.Sprintf("https://%s.traffit.com/public/an/list/?limit=%d&offset=%d",
			e.Board, traffitPageSize, page*traffitPageSize)
		var list traffitList
		if err := t.http.GetJSON(ctx, url, &list); err != nil {
			return nil, fmt.Errorf("traffit: list %s offset %d: %w", e.Board, page*traffitPageSize, err)
		}
		if len(list.Items) == 0 {
			break
		}
		for _, it := range list.Items {
			if j, ok := t.toJob(it, e); ok {
				jobs = append(jobs, j)
			}
		}
		if fetched += len(list.Items); fetched >= list.Count {
			break
		}
	}
	return jobs, nil
}

// toJob maps a posting to a Job, returning ok=false when it carries no id (which would
// collide on the dedup key). Title falls back to name; company comes from config.
func (traffit) toJob(it traffitItem, e CompanyEntry) (Job, bool) {
	id := it.AdvertID
	if id == 0 {
		id = it.AdvertPublishID
	}
	if id == 0 {
		return Job{}, false
	}

	title := strings.TrimSpace(it.Title)
	if title == "" {
		title = strings.TrimSpace(it.Name)
	}

	var location string
	if it.Geolocation != "" {
		var geo traffitGeo
		if json.Unmarshal([]byte(it.Geolocation), &geo) == nil {
			location = geo.String()
		}
	}

	return Job{
		ExternalID:  strconv.FormatInt(id, 10),
		URL:         it.URL,
		Title:       title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(it.Description),
		PostedAt:    parseEpochSeconds(it.ValidStart),
	}, true
}
