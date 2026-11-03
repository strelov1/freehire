package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"slices"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// sitemapBoard is the body shared by the boardless aggregators whose whole catalogue is one flat
// sitemap and whose posting pages each carry a schema.org JobPosting (getinit,
// berlinstartupjobs). Neither site has a listing API, so the sitemap names the postings and each
// page buys the facts; only postings the catalogue lacks are hydrated, the rest are refreshed by
// identity. Unlike the generic sitemapldjson provider it has a fixed sitemap, so the source is its
// own key and counts as the aggregator it is, not as KindOther.
type sitemapBoard struct {
	provider   string
	sitemapURL string
	// id extracts a posting's identity from its sitemap URL, "" for a page that is not a posting.
	id func(loc string) string
	// country is the fallback for a posting whose JobPosting states none.
	country string
	http    sitemapBoardHTTP
	limiter waiter
	maxNew  int
}

type sitemapBoardHTTP interface {
	XMLGetter
	HTMLGetter
}

const sitemapBoardDetailWorkers = 2

func newSitemapBoard(c sitemapBoardHTTP, provider, sitemapURL, country string, id func(string) string, every time.Duration, maxNew int) sitemapBoard {
	return sitemapBoard{
		provider:   provider,
		sitemapURL: sitemapURL,
		id:         id,
		country:    country,
		http:       c,
		limiter:    rate.NewLimiter(rate.Every(every), 1),
		maxNew:     maxNew,
	}
}

func (b sitemapBoard) Provider() string { return b.provider }

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (b sitemapBoard) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return b.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew refreshes the postings already stored and hydrates up to maxNew of the rest. A page
// that cannot be read is not yielded: nothing is stored for it yet, so a later run tries again.
func (b sitemapBoard) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	if err := b.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	locs, err := sitemapJobLocs(ctx, b.http, b.sitemapURL, b.id)
	if err != nil {
		return nil, fmt.Errorf("%s: sitemap: %w", b.provider, err)
	}
	if len(locs) == 0 {
		return nil, fmt.Errorf("%s: sitemap names no posting", b.provider)
	}
	var unseen []string
	var jobs []Job
	deferred := 0
	for _, loc := range locs {
		id := b.id(loc)
		if seen(id) {
			jobs = append(jobs, Job{ExternalID: id, URL: loc, SeenRefresh: true})
			continue
		}
		if len(unseen) >= b.maxNew {
			deferred++
			continue
		}
		unseen = append(unseen, loc)
	}
	if deferred > 0 {
		log.Printf("%s: %d new postings past this run's bound of %d, deferred to later runs", b.provider, deferred, b.maxNew)
	}
	read := fetchDetails(unseen, sitemapBoardDetailWorkers, func(loc string) (Job, bool) {
		return b.detail(ctx, loc)
	})
	return append(jobs, read...), nil
}

func (b sitemapBoard) detail(ctx context.Context, loc string) (Job, bool) {
	if err := b.limiter.Wait(ctx); err != nil {
		return Job{}, false
	}
	root, err := b.http.GetHTML(ctx, loc)
	if err != nil {
		log.Printf("%s: posting %s: %v", b.provider, loc, err)
		return Job{}, false
	}
	var p sitemapBoardPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	title := strings.TrimSpace(p.Title)
	company := strings.TrimSpace(p.HiringOrganization.Name)
	if title == "" || company == "" {
		return Job{}, false
	}
	remote := strings.EqualFold(strings.TrimSpace(p.JobLocationType), "TELECOMMUTE")
	job := Job{
		ExternalID:     b.id(loc),
		URL:            loc,
		Title:          title,
		Company:        company,
		Location:       strings.Join(p.localities(), "; "),
		Countries:      countriesFromCodes(p.countryCodes()),
		Description:    sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:         remote,
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: schemaEmploymentRaw(p.EmploymentType),
		PostedAt:       NotFuture(parseRFC3339OrDate(p.DatePosted)),
	}
	if job.Countries == nil && b.country != "" {
		job.FallbackCountries = []string{b.country}
	}
	return job, true
}

// sitemapBoardPosting is the part of a posting page's JobPosting the board adapters read.
type sitemapBoardPosting struct {
	Title              string             `json:"title"`
	Description        string             `json:"description"`
	DatePosted         string             `json:"datePosted"`
	EmploymentType     json.RawMessage    `json:"employmentType"`
	JobLocationType    string             `json:"jobLocationType"`
	JobLocation        sitemapBoardPlaces `json:"jobLocation"`
	HiringOrganization sitemapldjsonOrg   `json:"hiringOrganization"`
}

// sitemapBoardPlaces decodes jobLocation, a Place or an array of them, whose address is in turn
// one PostalAddress or an array: Get in IT states a single Place carrying one address per city.
// A shape it cannot read leaves the posting without a place rather than dropping the posting.
type sitemapBoardPlaces []schemaAddress

func (p *sitemapBoardPlaces) UnmarshalJSON(b []byte) error {
	var places schemaPlaces
	if json.Unmarshal(b, &places) == nil {
		for _, pl := range places {
			*p = append(*p, pl.Address)
		}
		return nil
	}
	var raw []struct {
		Address json.RawMessage `json:"address"`
	}
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("{")) {
		var one struct {
			Address json.RawMessage `json:"address"`
		}
		if json.Unmarshal(b, &one) != nil {
			return nil
		}
		raw = append(raw, one)
	} else if json.Unmarshal(b, &raw) != nil {
		return nil
	}
	for _, r := range raw {
		var addrs []schemaAddress
		if json.Unmarshal(r.Address, &addrs) == nil {
			*p = append(*p, addrs...)
		}
	}
	return nil
}

// localities lists the distinct cities, in the order the page states them.
func (p sitemapBoardPosting) localities() []string {
	var out []string
	for _, a := range p.JobLocation {
		if city := strings.TrimSpace(a.AddressLocality); city != "" && !slices.Contains(out, city) {
			out = append(out, city)
		}
	}
	return out
}

func (p sitemapBoardPosting) countryCodes() []string {
	var out []string
	for _, a := range p.JobLocation {
		out = append(out, a.AddressCountry)
	}
	return out
}
