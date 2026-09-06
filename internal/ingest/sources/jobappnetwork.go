package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// jobappnetwork adapts jobappnetwork.com career sites (vendor: talentReef). A board is one
// numeric client id — one employer's whole talentReef account — exactly what the platform's own
// public apply links carry: apply.jobappnetwork.com/clients/<clientId>/posting/<id>/. The
// listing is a keyless POST to the platform's Elasticsearch-backed search proxy, filtered to the
// board's client id and to externally-visible postings, and carries each posting's whole body —
// no per-posting detail request is needed.
//
// See internal/ingest/sources/AGENTS.md's "jobappnetwork traps" section for what was verified
// live: the real API host (named "internal" but publicly reachable), the query shape, and why
// the internalOrExternal filter is mandatory rather than a post-hoc drop.
type jobappnetwork struct {
	http JSONPoster
}

// NewJobAppNetwork builds the jobappnetwork adapter over the given keyless JSON client.
func NewJobAppNetwork(c JSONPoster) Source { return jobappnetwork{http: c} }

func (jobappnetwork) Provider() string { return "jobappnetwork" }

const (
	jobappnetworkSearchURL = "https://prod-kong.internal.talentreef.com/apply/proxy-es/search-en-us/posting/_search"
	jobappnetworkApplyURL  = "https://apply.jobappnetwork.com/clients/%d/posting/%d/"
	// jobappnetworkPageSize is the page window. hits.total was exact and stable across page
	// sizes in every board sampled live, so it bounds the walk directly.
	jobappnetworkPageSize = 100
)

// jobappnetworkAddress is a posting's structured place.
type jobappnetworkAddress struct {
	City            string `json:"city"`
	StateOrProvince string `json:"stateOrProvince"`
	Country         string `json:"country"`
}

// label renders the address as "City, State, Country", skipping parts the platform left blank.
func (a jobappnetworkAddress) label() string {
	return joinNonEmpty(a.City, a.StateOrProvince, a.Country)
}

// jobappnetworkPosting is one _source in the search proxy's response. The full description is
// already here — verified identical to what the platform's own posting page renders — so
// nothing else needs fetching.
type jobappnetworkPosting struct {
	JobID       int                  `json:"jobId"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	Address     jobappnetworkAddress `json:"address"`
	CreatedDate string               `json:"createdDate"`
}

// jobappnetworkResponse is one page of the plain Elasticsearch hit list the proxy answers.
type jobappnetworkResponse struct {
	Hits struct {
		Total int `json:"total"`
		Hits  []struct {
			Source jobappnetworkPosting `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func (s jobappnetwork) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	clientID, err := parseJobAppNetworkBoard(e.Board)
	if err != nil {
		return nil, err
	}
	postings, err := s.list(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("jobappnetwork: board %q: %w", e.Board, err)
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		jobs = append(jobs, s.toJob(e, clientID, p))
	}
	return jobs, nil
}

// parseJobAppNetworkBoard validates the board is the platform's own numeric client id, exactly
// as it appears in a jobappnetwork apply URL — the only shape the search proxy's clientId filter
// accepts.
func parseJobAppNetworkBoard(board string) (int, error) {
	id, err := strconv.Atoi(board)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("jobappnetwork: board %q must be a positive numeric client id", board)
	}
	return id, nil
}

// list pages the search proxy until hits.total postings have been collected. An empty page also
// ends the walk, so a total that ever went wrong cannot spin the loop. Only the FIRST page
// failing is a board-level error; a later page failing ends the walk with what was gathered.
func (s jobappnetwork) list(ctx context.Context, clientID int) ([]jobappnetworkPosting, error) {
	var postings []jobappnetworkPosting
	for from := 0; ; from += jobappnetworkPageSize {
		var resp jobappnetworkResponse
		if err := s.http.PostJSON(ctx, jobappnetworkSearchURL, jobappnetworkSearchBody(clientID, from), &resp); err != nil {
			if from == 0 {
				return nil, err
			}
			break
		}
		if len(resp.Hits.Hits) == 0 {
			break
		}
		for _, h := range resp.Hits.Hits {
			postings = append(postings, h.Source)
		}
		if len(postings) >= resp.Hits.Total {
			break
		}
	}
	return postings, nil
}

// jobappnetworkSearchBody is one page of the client-scoped listing. internalOrExternal is
// filtered to externalOnly so a posting meant only for internal-transfer applicants never
// crosses the wire — pushed into the query itself rather than dropped after the fact, so nothing
// downstream has to remember to call a second check.
func jobappnetworkSearchBody(clientID, from int) map[string]any {
	return map[string]any{
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []map[string]any{
					{"term": map[string]any{"clientId": clientID}},
					{"term": map[string]any{"internalOrExternal": "externalOnly"}},
				},
			},
		},
		"from": from,
		"size": jobappnetworkPageSize,
	}
}

func (jobappnetwork) toJob(e CompanyEntry, clientID int, p jobappnetworkPosting) Job {
	return Job{
		ExternalID:  strconv.Itoa(p.JobID),
		URL:         fmt.Sprintf(jobappnetworkApplyURL, clientID, p.JobID),
		Title:       strings.TrimSpace(p.Title),
		Company:     e.Company,
		Location:    p.Address.label(),
		Description: sanitizeHTML(p.Description),
		Countries:   countriesFromCodes([]string{p.Address.Country}),
		PostedAt:    parseDate(p.CreatedDate),
	}
}
