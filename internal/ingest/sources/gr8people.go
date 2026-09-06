package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// gr8people adapts career sites on the gr8people ATS — one vendor served under two marketing
// domains, gr8people.com and workgr8.com (confirmed live: identical frontend bundle, identical
// GraphQL schema, same session-token mechanism). A board is the tenant's whole careers host
// (e.g. "etrade.gr8people.com"), because the brand domain is not derivable from the tenant name
// alone — the same reasoning atsboard's modeHost applies to Factorial and Teamtailor.
//
// Fetch first mints a short-lived anonymous session token by reading the tenant's own /jobs
// page (a Next.js server-rendered shell whose __NEXT_DATA__ blob embeds it), then pages the
// tenant's own /graphql endpoint with that token as a Bearer credential. The default, unfiltered
// search already returns only what a public visitor sees — open, externally-visible postings —
// so unlike jobappnetwork's shared search proxy, no extra visibility filter is needed. See
// internal/ingest/sources/AGENTS.md's "gr8people traps" section for what was verified live.
type gr8peopleHTTP interface {
	TextGetter
	HeaderJSONPoster
}

type gr8people struct {
	http gr8peopleHTTP
}

// NewGr8People builds the gr8people adapter over the given HTTP client.
func NewGr8People(c gr8peopleHTTP) Source { return gr8people{http: c} }

func (gr8people) Provider() string { return "gr8people" }

// gr8peoplePageSize is the search page size.
const gr8peoplePageSize = 50

// gr8peopleTokenPattern captures the anonymous session JWT the tenant's /jobs page embeds in
// its Next.js __NEXT_DATA__ blob.
var gr8peopleTokenPattern = regexp.MustCompile(`"token"\s*:\s*"(eyJ[^"]+)"`)

// gr8peopleSearchQuery is a trimmed selection of the platform's own searchJobs query (verified
// live against the full jobPostingFields fragment) — only the fields this adapter maps.
const gr8peopleSearchQuery = `query searchJobs($query: String, $start: Int, $first: Int, $after: String, $filters: JobPostingSearchFiltersInput) {
  searchJobs: searchJobPostings(query: $query, start: $start, first: $first, after: $after, filters: $filters) {
    results {
      nodes {
        key
        title
        descriptionHTML
        workplaceType
        postedOn
        positionType { name }
        primaryPlace { name }
        places { nodes { name } }
      }
      pageInfo { hasNextPage endCursor }
      totalCount
    }
  }
}`

type gr8peoplePlace struct {
	Name string `json:"name"`
}

type gr8peoplePositionType struct {
	Name string `json:"name"`
}

// gr8peoplePosting is one node in the searchJobs response.
type gr8peoplePosting struct {
	Key             string                 `json:"key"`
	Title           string                 `json:"title"`
	DescriptionHTML string                 `json:"descriptionHTML"`
	WorkplaceType   string                 `json:"workplaceType"`
	PostedOn        string                 `json:"postedOn"`
	PositionType    *gr8peoplePositionType `json:"positionType"`
	PrimaryPlace    *gr8peoplePlace        `json:"primaryPlace"`
	Places          struct {
		Nodes []gr8peoplePlace `json:"nodes"`
	} `json:"places"`
}

// location renders the posting's places joined by "; ", falling back to primaryPlace when the
// places list is empty. Several places joined keeps a multi-location posting matchable on any
// of them, the same reasoning dayforce/landing.jobs apply.
func (p gr8peoplePosting) location() string {
	if loc := distinctJoin(p.Places.Nodes, "; ", gr8peoplePlace.name); loc != "" {
		return loc
	}
	if p.PrimaryPlace != nil {
		return p.PrimaryPlace.Name
	}
	return ""
}

func (p gr8peoplePlace) name() string { return p.Name }

type gr8peopleResponse struct {
	Data struct {
		SearchJobs struct {
			Results struct {
				Nodes    []gr8peoplePosting `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"results"`
		} `json:"searchJobs"`
	} `json:"data"`
}

func (s gr8people) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if e.Board == "" {
		return nil, fmt.Errorf("gr8people: board must be the tenant's careers host")
	}
	home, err := s.http.GetText(ctx, "https://"+e.Board+"/jobs")
	if err != nil {
		return nil, fmt.Errorf("gr8people: jobs page %s: %w", e.Board, err)
	}
	token := firstSubmatch(gr8peopleTokenPattern, home)
	if token == "" {
		return nil, fmt.Errorf("gr8people: no session token on %s /jobs", e.Board)
	}

	postings, err := s.list(ctx, e.Board, token)
	if err != nil {
		return nil, fmt.Errorf("gr8people: search %s: %w", e.Board, err)
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		jobs = append(jobs, s.toJob(e, p))
	}
	return jobs, nil
}

// list pages the tenant's /graphql search until a page comes back with no nodes or the platform
// reports no further page — Relay-style cursor pagination, not an exact total to chase. Only the
// FIRST page failing is a board-level error; a later page failing ends the walk with what was
// gathered.
func (s gr8people) list(ctx context.Context, board, token string) ([]gr8peoplePosting, error) {
	url := "https://" + board + "/graphql"
	headers := map[string]string{"Authorization": "Bearer " + token}

	var postings []gr8peoplePosting
	after := ""
	for page := 0; ; page++ {
		var resp gr8peopleResponse
		if err := s.http.PostJSONWithHeaders(ctx, url, headers, gr8peopleSearchBody(after), &resp); err != nil {
			if page == 0 {
				return nil, err
			}
			break
		}
		results := resp.Data.SearchJobs.Results
		if len(results.Nodes) == 0 {
			break
		}
		postings = append(postings, results.Nodes...)
		if !results.PageInfo.HasNextPage {
			break
		}
		after = results.PageInfo.EndCursor
	}
	return postings, nil
}

// gr8peopleSearchBody is one page of the tenant-scoped listing. after is omitted on the first
// page — the platform's cursor decoder rejects an empty string, so "no cursor yet" must be
// "no key" rather than an empty one.
func gr8peopleSearchBody(after string) map[string]any {
	variables := map[string]any{
		"query":   "",
		"start":   0,
		"first":   gr8peoplePageSize,
		"filters": map[string]any{},
	}
	if after != "" {
		variables["after"] = after
	}
	return map[string]any{
		"operationName": "searchJobs",
		"query":         gr8peopleSearchQuery,
		"variables":     variables,
	}
}

func (gr8people) toJob(e CompanyEntry, p gr8peoplePosting) Job {
	location := p.location()
	mode := workplaceTypeMode(strings.ReplaceAll(p.WorkplaceType, "_", "-"))
	return Job{
		ExternalID:     p.Key,
		URL:            "https://" + e.Board + "/jobs/" + p.Key,
		Title:          strings.TrimSpace(p.Title),
		Company:        e.Company,
		Location:       location,
		Description:    sanitizeHTML(p.DescriptionHTML),
		Remote:         mode == "remote" || isRemote(location),
		WorkMode:       mode,
		EmploymentType: gr8peopleEmploymentType(p.PositionType),
		PostedAt:       parseRFC3339(p.PostedOn),
	}
}

// gr8peopleEmploymentType maps the tenant-typed positionType name to our vocabulary. It is free
// text a tenant could rename, so only the two overwhelmingly common values seen live are
// mapped; anything else is left to the pipeline's own dictionaries.
func gr8peopleEmploymentType(pt *gr8peoplePositionType) string {
	if pt == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(pt.Name)) {
	case "full time", "full-time":
		return "full_time"
	case "part time", "part-time":
		return "part_time"
	default:
		return ""
	}
}
