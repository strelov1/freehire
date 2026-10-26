package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/sync/errgroup"
)

// jobindex adapts Jobindex (jobindex.dk), the largest Danish job board. It has no public API:
// the search page server-renders its whole result state as a `var Stash = {...}` JSON object,
// which carries every posting's id, title, employer, area and date, plus the rendered card.
//
// Two measured limits shape the crawl (live-checked 2026-10-25):
//
//   - A query reaches at most max_page=50 pages of 20, so 1000 postings, while the unfiltered
//     catalogue holds ~17.6k. The crawl therefore walks the site's own ~78 subcategories
//     (`subid`), read from the page itself rather than hard-coded, so a new subcategory is
//     crawled without a release. All but three fit the window; those three (Pædagog, Pleje og
//     omsorg, Detailhandel — none technical, 1.1k-1.5k each) are walked newest-first, so what
//     the window cannot reach is consistently their oldest tail.
//   - Subcategories overlap: a posting is filed under several (the seven IT subcategories sum
//     to ~1.5k for ~975 distinct IT postings). One boardless crawl that de-duplicates by the
//     posting id stores each posting once; per-subcategory boards would store it once per
//     board, since the pipeline namespaces external_id by board.
//
// The card text is all Jobindex has: the posting page (/vis-job/<id>) renders the same ~340-
// character teaser, and the full body lives on the employer's own site behind a "Se jobbet"
// link to heterogeneous hosts (none of three sampled carried a schema.org JobPosting). So the
// adapter is list-only and spends no detail request.
//
// Like jobnet/jobdanmark it covers every sector; the dictionaries decide relevance.
type jobindex struct {
	http TextGetter
}

const (
	// jobindexSearchURL is the newest-first search; with a subid it selects one subcategory.
	jobindexSearchURL = "https://www.jobindex.dk/jobsoegning?sort=date"
	jobindexSliceURL  = "https://www.jobindex.dk/jobsoegning?subid=%d&sort=date&page=%d"
	jobindexJobURL    = "https://www.jobindex.dk/vis-job/"
	// jobindexMaxPages is the runaway backstop under the max_page the page itself declares
	// (50 when measured), so a malformed total cannot loop.
	jobindexMaxPages = 60
	// jobindexSliceWorkers bounds how many subcategories are walked at once. A page answers in
	// ~1s from the crawl hosts and a full crawl is ~1.2k pages, so 4 keeps a run near 5 minutes.
	jobindexSliceWorkers = 4
	jobindexStashMarker  = "var Stash = "
)

// NewJobindex builds the Jobindex adapter over the given raw-body client.
func NewJobindex(c TextGetter) Source { return jobindex{http: c} }

func (jobindex) Provider() string { return "jobindex" }

// jobindex is one national board with one catalogue, so its entry carries no board.
func (jobindex) boardless() {}

// jobindex aggregates many employers, so it stays in the source facet.
func (jobindex) aggregator() {}

// jobindexStash is the part of the page's Stash object the crawl reads.
type jobindexStash struct {
	List struct {
		StoreData struct {
			Categories     []jobindexCategoryGroup `json:"subjobcategory_list"`
			SearchResponse jobindexSearchResponse  `json:"searchResponse"`
		} `json:"storeData"`
	} `json:"jobsearch/result_app/list"`
}

// jobindexCategoryGroup is one `[groupName, [[name, subid], ...]]` entry of the category list.
type jobindexCategoryGroup struct {
	SubIDs []int
}

func (g *jobindexCategoryGroup) UnmarshalJSON(b []byte) error {
	var pair []json.RawMessage
	if err := json.Unmarshal(b, &pair); err != nil {
		return err
	}
	if len(pair) != 2 {
		return fmt.Errorf("category group has %d elements, want 2", len(pair))
	}
	var subs [][2]json.RawMessage
	if err := json.Unmarshal(pair[1], &subs); err != nil {
		return err
	}
	for _, s := range subs {
		var id int
		if err := json.Unmarshal(s[1], &id); err != nil {
			return fmt.Errorf("subcategory id: %w", err)
		}
		g.SubIDs = append(g.SubIDs, id)
	}
	return nil
}

type jobindexSearchResponse struct {
	HitCount   int              `json:"hitcount"`
	TotalPages int              `json:"total_pages"`
	MaxPage    int              `json:"max_page"`
	Results    []jobindexResult `json:"results"`
}

// jobindexResult is one posting. Company is null on postings Jobindex's robot found on another
// site; companytext then names the employer.
type jobindexResult struct {
	Tid     string `json:"tid"`
	Title   string `json:"headline"`
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	CompanyText string `json:"companytext"`
	Area        string `json:"area"`
	FirstDate   string `json:"firstdate"`
	ShareURL    string `json:"share_url"`
	HTML        string `json:"html"`
	IsArchived  bool   `json:"is_archived"`
}

// Fetch reads the subcategory list, walks every subcategory, and returns each posting once.
// Any failed page fails the crawl: the post-run sweep closes what a run did not see, so a
// slice silently missing would close live postings.
func (s jobindex) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	first, err := s.page(ctx, jobindexSearchURL)
	if err != nil {
		return nil, fmt.Errorf("jobindex: category list: %w", err)
	}
	subIDs := jobindexSubIDs(first.List.StoreData.Categories)
	if len(subIDs) == 0 {
		return nil, fmt.Errorf("jobindex: the search page listed no subcategories")
	}

	slices := make([][]jobindexResult, len(subIDs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(jobindexSliceWorkers)
	for i, id := range subIDs {
		g.Go(func() error {
			res, err := s.slice(gctx, id)
			slices[i] = res
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var jobs []Job
	for _, results := range slices {
		for _, r := range results {
			if seen[r.Tid] {
				continue
			}
			if j, ok := r.toJob(); ok {
				seen[r.Tid] = true
				jobs = append(jobs, j)
			}
		}
	}
	return jobs, nil
}

// jobindexSubIDs flattens the category list into distinct subcategory ids, in page order. A
// subcategory sits under several groups (IT-ledelse is both IT and management), so it is
// walked once.
func jobindexSubIDs(groups []jobindexCategoryGroup) []int {
	var ids []int
	seen := make(map[int]bool)
	for _, g := range groups {
		for _, id := range g.SubIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// slice walks one subcategory newest-first to its last page or the page cap the site declares,
// whichever comes first.
func (s jobindex) slice(ctx context.Context, subID int) ([]jobindexResult, error) {
	var out []jobindexResult
	last := 1
	for page := 1; page <= last && page <= jobindexMaxPages; page++ {
		st, err := s.page(ctx, fmt.Sprintf(jobindexSliceURL, subID, page))
		if err != nil {
			return nil, fmt.Errorf("jobindex: subcategory %d page %d: %w", subID, page, err)
		}
		resp := st.List.StoreData.SearchResponse
		if len(resp.Results) == 0 {
			break
		}
		out = append(out, resp.Results...)
		if page == 1 {
			last = resp.TotalPages
			if resp.MaxPage > 0 && resp.MaxPage < last {
				last = resp.MaxPage
			}
		}
	}
	return out, nil
}

// page fetches one search page and decodes its Stash object.
func (s jobindex) page(ctx context.Context, url string) (jobindexStash, error) {
	var st jobindexStash
	body, err := s.http.GetText(ctx, url)
	if err != nil {
		return st, err
	}
	raw, ok := bracketSlice(body, jobindexStashMarker, '{', '}')
	if !ok {
		return st, fmt.Errorf("no Stash object on the page")
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return st, fmt.Errorf("decode Stash: %w", err)
	}
	return st, nil
}

// toJob maps a result to a Job, returning ok=false for an archived posting or one with no id,
// title or employer to key on.
func (r jobindexResult) toJob() (Job, bool) {
	title := strings.TrimSpace(r.Title)
	company := strings.TrimSpace(firstNonEmpty(r.Company.Name, r.CompanyText))
	if r.IsArchived || r.Tid == "" || title == "" || company == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  r.Tid,
		URL:         firstNonEmpty(r.ShareURL, jobindexJobURL+r.Tid),
		Title:       title,
		Company:     company,
		Description: sanitizeHTML(jobindexTeaser(r.HTML)),
		// The area is free text without a country ("Ballerup eller Næstved"); the board is
		// Danish, so the country is appended for the geo dictionary, as jobdanmark does.
		Location: joinNonEmpty(strings.TrimSpace(r.Area), "Danmark"),
		PostedAt: parseDate(r.FirstDate),
	}, true
}

// jobindexTeaser extracts the ad text from a result card: the paragraphs and lists inside the
// card's body (PaidJob-inner on an ad placed with Jobindex, jix_robotjob-inner on one its robot
// found elsewhere). The title, the logos and the toolbar around them are left out.
func jobindexTeaser(card string) string {
	root, err := html.Parse(strings.NewReader(card))
	if err != nil {
		return ""
	}
	inner := firstByClass(root, "PaidJob-inner")
	if inner == nil {
		inner = firstByClass(root, "jix_robotjob-inner")
	}
	if inner == nil {
		return ""
	}
	var b strings.Builder
	walk(inner, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch n.DataAtom {
		case atom.P, atom.Ul, atom.Ol:
			_ = html.Render(&b, n)
			return false
		}
		return true
	})
	return b.String()
}
