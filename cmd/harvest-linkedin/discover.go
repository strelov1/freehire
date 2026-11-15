package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"gopkg.in/yaml.v3"
)

const (
	// defaultJobAge bounds a query to recently-posted jobs: the aim is employers hiring NOW, and a wider
	// window mostly re-surfaces companies already proposed.
	defaultJobAge = 7
	// defaultPages is deliberately one: pages multiply the request count, so wider coverage is a
	// per-query decision.
	defaultPages = 1
	// pageSize is the listing's fixed page size, which drives the `start` offset.
	pageSize = 10
)

// searchBase is the public, unauthenticated job-search listing.
const searchBase = "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search"

// query is one entry of the worklist: what to search for, and where.
type query struct {
	Keywords string `yaml:"keywords"`
	Location string `yaml:"location"`
	JobAge   int    `yaml:"jobage"`
	Pages    int    `yaml:"pages"`
}

// loadQueries reads the worklist, applying the bounded defaults. A query naming no market is a usage
// error: an unbounded market turns a bounded harvest into an unbounded crawl.
func loadQueries(path string) ([]query, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read worklist %s: %w", path, err)
	}
	var doc struct {
		Queries []query `yaml:"queries"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse worklist %s: %w", path, err)
	}
	if len(doc.Queries) == 0 {
		return nil, fmt.Errorf("worklist %s names no queries", path)
	}
	for i := range doc.Queries {
		q := &doc.Queries[i]
		if q.Location == "" {
			return nil, fmt.Errorf("worklist %s: query %q names no location", path, q.Keywords)
		}
		if q.JobAge <= 0 {
			q.JobAge = defaultJobAge
		}
		if q.Pages <= 0 {
			q.Pages = defaultPages
		}
	}
	return doc.Queries, nil
}

// searchURL builds the listing URL for one page (1-indexed) of a query.
func searchURL(q query, page int) string {
	params := url.Values{}
	params.Set("keywords", q.Keywords)
	params.Set("location", q.Location)
	if q.JobAge > 0 {
		params.Set("f_TPR", "r"+strconv.Itoa(q.JobAge*86400))
	}
	params.Set("start", strconv.Itoa((page-1)*pageSize))
	return searchBase + "?" + params.Encode()
}

// candidate is one hiring company, in the shape `harvest-ats resolve` consumes; the json tags match
// its companySite.
type candidate struct {
	Name       string `json:"name"`
	Website    string `json:"website"`
	LinkedIn   string `json:"linkedin,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
}

// stats is what a run needs to judge itself. A query that succeeds with no postings looks like a
// blocked or re-templated one, so these counts, not the candidate list, decide whether the run failed.
type stats struct {
	totalQueries int
	emptyQueries int
}

// everyQueryEmpty reports the shape that must never be read as success: every query answered,
// none of them with a posting.
func (s stats) everyQueryEmpty() bool { return s.totalQueries > 0 && s.emptyQueries == s.totalQueries }

// fetcher retrieves a URL's body; injected so tests need no network.
type fetcher func(ctx context.Context, url string) (string, error)

// discover turns the query worklist into the candidate companies behind its postings.
//
// The listing names the employer and links its profile, so collapsing postings to one employer and
// the catalogue filter happen on the listing alone: a known or already-seen company costs no further
// request. Each survivor costs two: its posting (ATS-native id) and its profile (website). A candidate
// with no website is dropped, since resolve has nothing to follow; one whose posting fetch fails keeps
// its place without an id.
func discover(ctx context.Context, fetch fetcher, queries []query, existing map[string]bool) ([]candidate, stats, error) {
	var (
		out  []candidate
		st   stats
		seen = map[string]bool{}
	)
	for _, q := range queries {
		st.totalQueries++
		cards := searchCards(ctx, fetch, q)
		if len(cards) == 0 {
			log.Printf("harvest-linkedin: %q in %q returned no postings — markup change or block, not an empty market",
				q.Keywords, q.Location)
			st.emptyQueries++
			continue
		}
		for _, c := range cards {
			if c.profileURL == "" || seen[c.profileURL] {
				continue
			}
			seen[c.profileURL] = true
			if existing[normalize.CompanySlug(c.company)] {
				continue
			}
			if cand, ok := hydrate(ctx, fetch, c); ok {
				out = append(out, cand)
			}
		}
	}
	return out, st, nil
}

// searchCards pages one query, stopping at the first empty page. A page that fails to fetch is
// logged and skipped.
func searchCards(ctx context.Context, fetch fetcher, q query) []card {
	var all []card
	for page := 1; page <= q.Pages; page++ {
		body, err := fetch(ctx, searchURL(q, page))
		if err != nil {
			log.Printf("harvest-linkedin: search %q/%q page %d: %v", q.Keywords, q.Location, page, err)
			continue
		}
		cards := parseCards(body)
		if len(cards) == 0 {
			break
		}
		all = append(all, cards...)
	}
	return all
}

// hydrate fetches what the listing does not carry: the posting's ATS-native id and the
// employer's website. ok is false when the website cannot be determined.
func hydrate(ctx context.Context, fetch fetcher, c card) (candidate, bool) {
	externalID := ""
	if c.postingURL != "" {
		if body, err := fetch(ctx, c.postingURL); err == nil {
			externalID = postingExternalID(body)
		} else {
			log.Printf("harvest-linkedin: posting %s: %v", c.postingURL, err)
		}
	}
	body, err := fetch(ctx, c.profileURL)
	if err != nil {
		log.Printf("harvest-linkedin: profile %s: %v", c.profileURL, err)
		return candidate{}, false
	}
	website := profileWebsite(body)
	if website == "" {
		return candidate{}, false
	}
	return candidate{Name: c.company, Website: website, LinkedIn: c.profileURL, ExternalID: externalID}, true
}
