package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/platform/flexjson"
)

// levtech adapts レバテックフリーランス (freelance.levtech.jp), a Japanese IT freelance project
// board run by an agency: it brokers 業務委託 (contract) and 派遣 (dispatch) projects and
// withholds every end client (hiringOrganization is always 社名非公開), so every posting is filed
// under the one employer the board itself is, like emagine.
//
// Boardless. The listing is the site's own search, /project/search/p<N>/, 40 projects a page,
// server-rendered into a Nuxt devalue payload (<script id="__NUXT_DATA__">). The same
// listing also holds the ~125k projects the site has closed, ordered AFTER every open one, so
// the walk ends at the first page that carries no open project. A card carries the title,
// monthly rate, nearest station and remote kind; the body, the city
// and the posting's own page live on the detail, whose schema.org JobPosting this adapter reads
// only for a posting the catalogue lacks, up to levtechDetailBudget a run. See the Levtech traps
// in AGENTS.md.
type levtech struct {
	http HTMLGetter
	// spent counts the detail requests claimed this run (the adapter is a value type, so the
	// counter is shared by pointer).
	spent *atomic.Int64
	// refusedBackoff overrides levtechRefusedBackoff (tests); zero keeps the default.
	refusedBackoff time.Duration
}

const (
	levtechBase = "https://freelance.levtech.jp"
	// levtechCompany is the employer for every posting: the agency keeps the end client private.
	levtechCompany = "Levtech Freelance"
	// levtechMaxPages is a safety ceiling, not the platform's. 63 pages held the whole open
	// catalogue on 2026-11-17. A walk that reaches it without finding the first all-closed page
	// fails the crawl rather than ending quietly short.
	levtechMaxPages = 150
	// The origin answered 200 to a plain client at one request a second; listing pages weigh
	// ~450 KB and details ~350 KB, so the pace is a courtesy rather than a measured limit.
	levtechRequestInterval = time.Second
	levtechRequestBurst    = 2
	levtechDetailWorkers   = 2
	// levtechDetailBudget bounds one run's detail requests: the first crawl faces ~2,500 unseen
	// postings, and at the pace above the backlog drains over a few runs while later runs buy
	// only the day's new postings.
	levtechDetailBudget = 600
	// levtechRefusedRetries bounds how often a 403 is asked again. The origin refuses the crawl
	// host's own IP, so requests go through the Scrapling tier's ROTATING proxy, which leaves
	// from a new exit IP each time; measured from the crawl host on 2026-11-17, roughly half of
	// those exits are refused (8 of 15 listing requests answered 200). Ten more tries leaves one
	// page in ~2,000 unread, so a ~63-page walk almost never fails on an unlucky exit.
	levtechRefusedRetries = 10
	levtechRefusedBackoff = 500 * time.Millisecond
)

// NewLevtech builds the Levtech Freelance adapter over the given HTML client.
func NewLevtech(c HTMLGetter) Source { return levtech{http: c, spent: new(atomic.Int64)} }

func (levtech) Provider() string { return "levtech" }

// get reads one page, asking again while the exit IP the request left from is refused (403).
func (s levtech) get(ctx context.Context, url string) (*html.Node, error) {
	var root *html.Node
	err := retryWhile(ctx, levtechRefusedRetries, s.backoff(), levtechRefused, func() (err error) {
		root, err = s.http.GetHTML(ctx, url)
		return err
	})
	return root, err
}

func (s levtech) backoff() time.Duration {
	if s.refusedBackoff > 0 {
		return s.refusedBackoff
	}
	return levtechRefusedBackoff
}

func levtechRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusForbidden
}
func (levtech) Boardless()  {}
func (levtech) Aggregator() {}

func (s levtech) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s levtech) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, cards, seen)
}

// levtechCard is one listed project, reduced to what the adapter reads.
type levtechCard struct {
	BaseData struct {
		ContractType          string         `json:"contractType"`
		ID                    flexjson.Int64 `json:"id"`
		Name                  string         `json:"name"`
		StationName           string         `json:"stationName"`
		StationPrefectureName string         `json:"stationPrefectureName"`
	} `json:"baseData"`
	IsClosed         bool           `json:"isClosed"`
	RemoteWorkTypeID flexjson.Int64 `json:"remoteWorkTypeId"`
}

// levtechRemote* are the site's remoteWorkTypeId values; anything else (999999999 on a posting
// that says nothing) states no work arrangement.
const (
	levtechFullRemote    = 1
	levtechPartialRemote = 2
)

func levtechPageURL(page int) string {
	if page <= 1 {
		return levtechBase + "/project/search/"
	}
	return fmt.Sprintf("%s/project/search/p%d/", levtechBase, page)
}

func levtechDetailURL(id string) string { return levtechBase + "/project/detail/" + id + "/" }

// list walks the search to the first page with no open project. Any page failing fails the crawl:
// the walk is ~63 pages long and a silently short one would read as a shrunken catalogue.
func (s levtech) list(ctx context.Context) ([]levtechCard, error) {
	var out []levtechCard
	ids := map[int64]bool{}
	for page := 1; page <= levtechMaxPages; page++ {
		root, err := s.get(ctx, levtechPageURL(page))
		if err != nil {
			return nil, fmt.Errorf("levtech: listing page %d: %w", page, err)
		}
		cards, err := levtechPageCards(root)
		if err != nil {
			return nil, fmt.Errorf("levtech: listing page %d: %w", page, err)
		}
		open := 0
		for _, c := range cards {
			if c.IsClosed || c.BaseData.ID <= 0 {
				continue
			}
			open++
			if ids[int64(c.BaseData.ID)] {
				continue
			}
			ids[int64(c.BaseData.ID)] = true
			out = append(out, c)
		}
		if open == 0 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("levtech: no all-closed page within %d pages; refusing a possibly truncated catalogue", levtechMaxPages)
}

// levtechPageCards decodes the project cards out of a listing page's Nuxt payload. A page whose
// payload carries no project list is not a results page (selector drift, a challenge page), which
// is an error: an empty results page still carries the list, just empty.
func levtechPageCards(root *html.Node) ([]levtechCard, error) {
	raw := scriptTextByID(root, "__NUXT_DATA__")
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("page carries no __NUXT_DATA__ payload")
	}
	var table []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &table); err != nil {
		return nil, fmt.Errorf("undecodable payload: %w", err)
	}
	tree, err := unflattenPayload(table)
	if err != nil {
		return nil, fmt.Errorf("unresolvable payload: %w", err)
	}
	projects, ok := levtechFindProjects(tree)
	if !ok {
		return nil, errors.New("payload carries no project list")
	}
	b, err := json.Marshal(projects)
	if err != nil {
		return nil, err
	}
	var cards []levtechCard
	if err := json.Unmarshal(b, &cards); err != nil {
		return nil, fmt.Errorf("undecodable project list: %w", err)
	}
	return cards, nil
}

// levtechFindProjects walks the resolved payload for the search result: the map that carries the
// project list beside its pagination. Walking rather than a fixed key path keeps the adapter
// robust to Nuxt's surrounding route-state nesting.
func levtechFindProjects(node any) ([]any, bool) {
	switch n := node.(type) {
	case map[string]any:
		if p, ok := n["projects"].([]any); ok {
			if _, paged := n["paginationMeta"]; paged {
				return p, true
			}
		}
		for _, v := range n {
			if p, ok := levtechFindProjects(v); ok {
				return p, true
			}
		}
	case []any:
		for _, v := range n {
			if p, ok := levtechFindProjects(v); ok {
				return p, true
			}
		}
	}
	return nil, false
}

// hydrate maps the listed cards to Jobs. A seen posting is a liveness refresh with no request;
// every other posting costs one detail request, up to the run's budget, claimed in listing order
// (newest first). A detail that could not be read, or that the budget did not reach, defers its
// posting to a later run rather than storing it body-less, which would never be hydrated again.
func (s levtech) hydrate(ctx context.Context, cards []levtechCard, seen func(string) bool) ([]Job, error) {
	var jobs, claimed []Job
	deferred := 0
	for _, c := range cards {
		base, ok := levtechBaseJob(c)
		if !ok {
			continue
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if s.spent.Add(1) > levtechDetailBudget {
			deferred++
			continue
		}
		claimed = append(claimed, base)
	}
	var answered atomic.Int64
	jobs = append(jobs, fetchDetails(claimed, levtechDetailWorkers, func(base Job) (Job, bool) {
		job, ok, err := s.withDetail(ctx, base)
		if err != nil {
			log.Printf("levtech: detail %s failed; deferring: %v", base.ExternalID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})...)
	// A crawl that asked for details and got no answer to any of them is broken, not quiet.
	if len(claimed) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("levtech: requested %d posting details and none answered", len(claimed))
	}
	if deferred > 0 {
		log.Printf("levtech: detail budget reached (%d a run); %d new postings deferred to later runs",
			levtechDetailBudget, deferred)
	}
	return jobs, nil
}

// levtechBaseJob maps a card to a body-less Job. Every project is IT work by the board's own
// scope (the designers and PMs it lists included), so the posting is confirmed technical.
func levtechBaseJob(c levtechCard) (Job, bool) {
	id := strconv.FormatInt(int64(c.BaseData.ID), 10)
	title := strings.Join(strings.Fields(c.BaseData.Name), " ")
	if id == "0" || title == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:        id,
		URL:               levtechDetailURL(id),
		Title:             title,
		Company:           levtechCompany,
		Location:          joinNonEmpty(strings.TrimSpace(c.BaseData.StationName), strings.TrimSpace(c.BaseData.StationPrefectureName)),
		IsTechHint:        true,
		FallbackCountries: []string{"jp"},
	}
	switch int64(c.RemoteWorkTypeID) {
	case levtechFullRemote:
		j.Remote, j.WorkMode = true, "remote"
	case levtechPartialRemote:
		j.WorkMode = "hybrid"
	}
	if c.BaseData.ContractType == "subcontractor" {
		j.EmploymentType = "contract"
	}
	return j, true
}

// levtechPosting is the detail page's schema.org JobPosting, reduced to what the adapter reads.
type levtechPosting struct {
	URL         string       `json:"url"`
	Description string       `json:"description"`
	JobLocation schemaPlaces `json:"jobLocation"`
	BaseSalary  struct {
		Currency string `json:"currency"`
		Value    struct {
			Value flexjson.Int64 `json:"value"`
			Unit  string         `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// withDetail adds the posting's body, city and rate. ok=false with a nil error is the platform's
// own answer that the posting is gone: a closed project's page answers 404.
//
// datePosted is deliberately not read: it carried the day of the request on every page sampled,
// including projects listed years ago, so it states when the page was rendered, not when the
// project was posted.
func (s levtech) withDetail(ctx context.Context, base Job) (Job, bool, error) {
	root, err := s.get(ctx, base.URL)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var p levtechPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false, errors.New("page carries no JobPosting")
	}
	if p.URL != "" && !strings.Contains(p.URL, "/project/detail/"+base.ExternalID+"/") {
		return Job{}, false, fmt.Errorf("page answered posting %s", p.URL)
	}
	body := sanitizeHTML(p.Description)
	if strings.TrimSpace(body) == "" {
		return Job{}, false, errors.New("posting carries no body")
	}
	base.Description = body
	if len(p.JobLocation) > 0 {
		a := p.JobLocation[0].Address
		if place := joinNonEmpty(strings.TrimSpace(a.AddressLocality), strings.TrimSpace(a.AddressRegion)); place != "" {
			base.Location = place
		}
		if strings.EqualFold(a.AddressCountry, "JP") {
			base.Countries = []string{"jp"}
		}
	}
	levtechSalary(&base, p)
	return base, true, nil
}

// levtechSalary copies the posting's rate. The site states one figure, the project's rate
// ceiling (単価), per month for a contract project and per hour for a dispatch one, so it is the
// upper bound only: the floor is whatever the negotiation settles on.
func levtechSalary(j *Job, p levtechPosting) {
	v := p.BaseSalary.Value
	if p.BaseSalary.Currency != "JPY" || v.Value <= 0 {
		return
	}
	var period string
	switch strings.ToUpper(v.Unit) {
	case "MONTH":
		period = "month"
	case "HOUR":
		period = "hour"
	default:
		return
	}
	hi := int(v.Value)
	j.SalaryMax = &hi
	j.SalaryCurrency = "JPY"
	j.SalaryPeriod = period
}
