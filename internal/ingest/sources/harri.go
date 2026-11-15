package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/normalize"
)

// harri adapts Harri (harri.com), the hospitality ATS restaurant groups and franchisees hire
// frontline staff on. A posting lives at
//
//	https://harri.com/<brand slug>/job/<id>-<title slug>
//
// and the board is the brand slug a career page opens with (`panera-careers`, `jollibeegroupna`,
// `gk-beo1m4ztp4bs`). The site is a single-page app; the data is the keyless JSON API on
// gateway.harri.com its own pages call:
//
//	GET  /core/api/v1/profile/slug/<slug>        -> {"data":{"id":<brand id>,"type":"brand",...}}
//	POST /core/api/v1/harri_search/search_jobs   {"brand_level_ids":[id],"start":K,"size":N,...}
//	GET  /core-reader/api/v1/profile/job/<id>    -> the posting, with its HTML description
//
// Traps:
//
//   - A brand is a TREE. A group's brand id lists every posting of every brand under it, so
//     the posting's own brand name is a store ("JB Mira Mesa"), not an employer, and the
//     employer is the board's catalogue company. A posting's URL carries the STORE's slug,
//     which can differ from the career page that listed it.
//   - The slug lookup is case-insensitive, and an unknown slug answers HTTP 400 ("Slug not found"),
//     not 404 — that is read as the board being gone.
//   - `start` pages the search; `from`, `offset` and `page` are silently ignored and answer the
//     first page again. `hits` is the exact total, and `size` is honoured up to at least 500.
//   - The listing row carries title, structured locations and publish time but no body, hence
//     HydratingSource. The detail answers for expired postings too (`status: EXPIRED`), so a
//     detail that is not PUBLISHED is read as gone.
//   - `Timing` is where the platform states both the schedule and the work arrangement:
//     FULL_TIME / PART_TIME beside FULLY_REMOTE / HYBRID_REMOTE. Most postings carry both
//     schedules at once, which states nothing, so an employment type is taken only from a lone
//     one.
//   - `country_code` is not always ISO: the UK is "UK". It is normalised through the location
//     dictionary, falling back to the country name.
//   - The compensation block is not mapped: "Up to" postings carry their ceiling in
//     `compensation_from`, and the currency is only in the detail's brand settings, null on some
//     brands — reading it would be a guess.
//   - No metering was met at ~23 req/s of detail requests. The crawl is still paced
//     (harriRequestInterval) because a group's bootstrap is one detail request per posting
//     (hundreds to ~1,000) and nothing is known about the API's budget.
//   - This is an hourly frontline segment (cooks, servers, store managers); its technical yield
//     is expected to be near zero, like Workstream's.
type harri struct {
	http harriHTTP
}

// harriHTTP is the transport the adapter needs: the slug lookup and posting detail are GETs, the
// search is a POST.
type harriHTTP interface {
	JSONGetter
	JSONPoster
}

// NewHarri builds the Harri adapter over the given HTTP client (the paced one in production —
// see pacedHarri).
func NewHarri(c harriHTTP) Source { return harri{http: c} }

func (harri) Provider() string { return "harri" }

// fullBoardListing: the walk pages by `start` until it has read the search's own `hits`, and
// fails unless the distinct ids read equal it. A page failing, a page answering empty before the
// count is reached, or the page ceiling being hit fails the crawl.
func (harri) fullBoardListing() {}

const (
	harriAPIBase   = "https://gateway.harri.com"
	harriSiteBase  = "https://harri.com"
	harriPageSize  = 100
	harriMaxPages  = 100
	harriPublished = "PUBLISHED"
	// harriRequestInterval paces every request of a run (slug, search pages, details) through one
	// bucket: 5 req/s sits far under the rate that drew no refusal while bounding a group
	// bootstrap to a few minutes.
	harriRequestInterval = 200 * time.Millisecond
	harriRequestBurst    = 2
)

// rateLimitedHarri gates both of the adapter's request kinds through one limiter, so the search
// walk and the detail fan-out share a single budget.
type rateLimitedHarri struct {
	inner   harriHTTP
	limiter waiter
}

func (p rateLimitedHarri) GetJSON(ctx context.Context, url string, v any) error {
	if err := p.limiter.Wait(ctx); err != nil {
		return err
	}
	return p.inner.GetJSON(ctx, url, v)
}

func (p rateLimitedHarri) PostJSON(ctx context.Context, url string, body, v any) error {
	if err := p.limiter.Wait(ctx); err != nil {
		return err
	}
	return p.inner.PostJSON(ctx, url, body, v)
}

// pacedHarri wraps a client with a fresh limiter shared across one registry build.
func pacedHarri(c harriHTTP) harriHTTP {
	return rateLimitedHarri{inner: c, limiter: rate.NewLimiter(rate.Every(harriRequestInterval), harriRequestBurst)}
}

type harriSlugResponse struct {
	Data json.RawMessage `json:"data"`
}

type harriProfile struct {
	ID         int64  `json:"id"`
	Type       string `json:"type"`
	IsArchived bool   `json:"is_archived"`
}

// harriSearchRequest is the search body the career page sends, with `start` added for paging.
type harriSearchRequest struct {
	Start         int      `json:"start"`
	Size          int      `json:"size"`
	Source        string   `json:"source"`
	BrandLevelIDs []int64  `json:"brand_level_ids"`
	Sort          []string `json:"sort"`
	SortType      string   `json:"sort_type"`
	Flow          string   `json:"flow"`
}

type harriSearchResponse struct {
	Data *struct {
		Hits    int              `json:"hits"`
		Results []harriListedJob `json:"results"`
	} `json:"data"`
}

type harriListedJob struct {
	ID    int64 `json:"id"`
	Brand struct {
		Slug string `json:"slug"`
	} `json:"brand"`
	Position struct {
		Name string `json:"name"`
	} `json:"position"`
	AliasPosition string          `json:"aliasPosition"`
	Locations     []harriLocation `json:"locations"`
	PublishTime   string          `json:"publishTime"`
}

type harriLocation struct {
	City        string `json:"city"`
	State       string `json:"state"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
}

type harriDetailResponse struct {
	Data *struct {
		Job struct {
			Description string `json:"description"`
			Status      string `json:"status"`
			Deleted     bool   `json:"deleted"`
			Timing      []struct {
				Code string `json:"code"`
			} `json:"Timing"`
		} `json:"job"`
	} `json:"data"`
}

func (s harri) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads a posting's detail only when the catalogue lacks it; a seen posting yields its
// listing row flagged SeenRefresh. A detail that could not be read is deferred to a later run
// rather than stored body-less, and a crawl that listed new postings and read none fails.
func (s harri) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	slug := strings.ToLower(strings.TrimSpace(e.Board))
	if slug == "" || strings.ContainsAny(slug, "/?#. ") {
		return nil, fmt.Errorf("harri: board %q is not a brand slug", e.Board)
	}
	brandID, err := s.brand(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("harri: board %s: %w", slug, err)
	}
	listed, err := s.list(ctx, brandID)
	if err != nil {
		return nil, fmt.Errorf("harri: board %s: %w", slug, err)
	}

	var wanted, read atomic.Int64
	jobs := fetchDetails(listed, defaultDetailWorkers, func(l harriListedJob) (Job, bool) {
		base, ok := harriBaseJob(slug, e.Company, l)
		if !ok {
			return Job{}, false
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			return base, true
		}
		wanted.Add(1)
		var d harriDetailResponse
		if err := s.http.GetJSON(ctx, harriAPIBase+"/core-reader/api/v1/profile/job/"+base.ExternalID, &d); err != nil {
			if detailUnreadable(err) {
				log.Printf("harri: posting %s: %v; deferring", base.ExternalID, err)
			}
			return Job{}, false
		}
		if d.Data == nil {
			log.Printf("harri: posting %s carries no job; deferring", base.ExternalID)
			return Job{}, false
		}
		read.Add(1)
		dj := d.Data.Job
		if dj.Deleted || dj.Status != harriPublished {
			return Job{}, false // the platform's own answer that the posting is closed
		}
		base.Description = sanitizeHTML(dj.Description)
		codes := make([]string, 0, len(dj.Timing))
		for _, t := range dj.Timing {
			codes = append(codes, t.Code)
		}
		base.WorkMode, base.EmploymentType = harriTiming(codes)
		base.Remote = base.WorkMode == "remote" || isRemote(base.Location)
		return base, true
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("harri: board %s: listed %d new postings and read none of them", slug, wanted.Load())
	}
	return jobs, nil
}

// brand resolves a career-page slug to the brand id the search is keyed by.
func (s harri) brand(ctx context.Context, slug string) (int64, error) {
	var resp harriSlugResponse
	if err := s.http.GetJSON(ctx, harriAPIBase+"/core/api/v1/profile/slug/"+url.PathEscape(slug), &resp); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusBadRequest || se.Code == http.StatusNotFound) {
			return 0, fmt.Errorf("slug lookup answered %d: %w", se.Code, ErrBoardGone)
		}
		return 0, fmt.Errorf("slug lookup: %w", err)
	}
	var p harriProfile
	if err := json.Unmarshal(resp.Data, &p); err != nil || p.ID == 0 {
		// The "Slug not found" answer carries a string where the profile would be.
		return 0, fmt.Errorf("slug lookup answered no brand: %w", ErrBoardGone)
	}
	if p.Type != "brand" {
		return 0, fmt.Errorf("slug names a %q profile, not a brand", p.Type)
	}
	if p.IsArchived {
		return 0, fmt.Errorf("brand %d is archived: %w", p.ID, ErrBoardGone)
	}
	return p.ID, nil
}

// list walks the brand's search by `start` until it has read the search's own `hits`, and proves
// the read complete by the distinct ids equalling that count.
func (s harri) list(ctx context.Context, brandID int64) ([]harriListedJob, error) {
	var all []harriListedJob
	seen := map[int64]bool{}
	start, total := 0, 0
	for page := 0; ; page++ {
		if page == harriMaxPages {
			return nil, fmt.Errorf("page ceiling %d reached at %d of %d postings", harriMaxPages, start, total)
		}
		req := harriSearchRequest{
			Start: start, Size: harriPageSize, Source: "web", BrandLevelIDs: []int64{brandID},
			Sort: []string{"publish_date"}, SortType: "desc", Flow: "CAREER_PORTAL",
		}
		var resp harriSearchResponse
		if err := s.http.PostJSON(ctx, harriAPIBase+"/core/api/v1/harri_search/search_jobs", req, &resp); err != nil {
			return nil, fmt.Errorf("search at start %d: %w", start, err)
		}
		if resp.Data == nil {
			return nil, fmt.Errorf("search at start %d: response carries no data", start)
		}
		total = resp.Data.Hits
		batch := resp.Data.Results
		for _, j := range batch {
			if !seen[j.ID] {
				seen[j.ID] = true
				all = append(all, j)
			}
		}
		start += len(batch)
		if len(batch) == 0 || start >= total {
			break
		}
	}
	if len(seen) != total {
		return nil, fmt.Errorf("read %d distinct postings, search declares %d", len(seen), total)
	}
	return all, nil
}

// harriBaseJob maps a listing row to the body-less job both paths start from. A row with no id or
// no title is not a posting.
func harriBaseJob(board, company string, l harriListedJob) (Job, bool) {
	title := strings.Join(strings.Fields(l.AliasPosition), " ")
	if title == "" {
		title = strings.Join(strings.Fields(l.Position.Name), " ")
	}
	if l.ID == 0 || title == "" {
		return Job{}, false
	}
	id := strconv.FormatInt(l.ID, 10)
	brand := strings.TrimSpace(l.Brand.Slug)
	if brand == "" {
		brand = board
	}
	jobURL := harriSiteBase + "/" + url.PathEscape(brand) + "/job/" + id
	if s := normalize.Slug(title); s != "" {
		jobURL += "-" + s
	}
	place, countries := harriPlaces(l.Locations)
	return Job{
		ExternalID: id,
		URL:        jobURL,
		Title:      title,
		Company:    company,
		Location:   place,
		Remote:     isRemote(place),
		PostedAt:   parseRFC3339(l.PublishTime),
		Countries:  countries,
	}, true
}

// harriPlaces renders a posting's structured locations as "City, State, Country" places joined
// by "; ", and returns the distinct countries they state. A state that repeats the country (UK
// postings carry "United Kingdom" in both) is written once.
func harriPlaces(locs []harriLocation) (string, []string) {
	var places, countries []string
	seenPlace, seenCountry := map[string]bool{}, map[string]bool{}
	for _, l := range locs {
		var parts []string
		for _, p := range []string{l.City, l.State, l.Country} {
			p = strings.Join(strings.Fields(p), " ")
			if p != "" && (len(parts) == 0 || !strings.EqualFold(parts[len(parts)-1], p)) {
				parts = append(parts, p)
			}
		}
		if place := strings.Join(parts, ", "); place != "" && !seenPlace[place] {
			seenPlace[place] = true
			places = append(places, place)
		}
		c := location.NormalizeCountry(l.CountryCode)
		if c == "" {
			c = location.NormalizeCountry(l.Country)
		}
		if c != "" && !seenCountry[c] {
			seenCountry[c] = true
			countries = append(countries, c)
		}
	}
	return strings.Join(places, "; "), countries
}

// harriTiming maps the detail's Timing codes to a work mode and an employment type. The work
// arrangement is stated by its own codes; a schedule is taken only when exactly one is stated,
// since "PART_TIME + FULL_TIME" — most postings — says nothing.
func harriTiming(codes []string) (workMode, employmentType string) {
	var schedules []string
	for _, c := range codes {
		switch strings.ToUpper(strings.TrimSpace(c)) {
		case "FULLY_REMOTE":
			workMode = "remote"
		case "HYBRID_REMOTE":
			if workMode == "" {
				workMode = "hybrid"
			}
		case "FULL_TIME":
			schedules = append(schedules, "full_time")
		case "PART_TIME":
			schedules = append(schedules, "part_time")
		}
	}
	if len(schedules) == 1 {
		employmentType = schedules[0]
	}
	return workMode, employmentType
}
