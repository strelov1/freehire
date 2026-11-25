package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"math/rand/v2"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	xhtml "golang.org/x/net/html"
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
	// techHint marks every posting technical, for a board that is IT-only by its own definition.
	techHint bool
	// ignoreTelecommute drops the JobPosting's TELECOMMUTE flag, for a board that sets it
	// without the posting offering remote work (ictjobs).
	ignoreTelecommute bool
	// parseDeadline is set only for providers whose stated calendar deadline is mapped.
	parseDeadline func(json.RawMessage) *time.Time
	// applyURL reads the employer's own apply link off the posting page; nil or "" records none.
	applyURL func(*xhtml.Node) string
	// applicantCountries reads the countries from applicantLocationRequirements when jobLocation
	// names none, for a remote board that states where the work may be done from.
	applicantCountries bool
	// maxAge drops a posting whose datePosted is older, for a board that keeps every posting in
	// its sitemap long after the employer closed it. It also re-reads the stored postings each
	// run instead of refreshing them by identity, since a stored posting ages too and only its
	// page states its date; one dropped there is left out of the run, so the stale sweep closes it.
	maxAge  time.Duration
	now     func() time.Time
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
		now:        time.Now,
		limiter:    rate.NewLimiter(rate.Every(every), 1),
		maxNew:     maxNew,
	}
}

func (b sitemapBoard) Provider() string { return b.provider }

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (b sitemapBoard) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return b.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew refreshes the postings already stored and hydrates up to maxNew of the rest. Unreadable
// details retain their listing identity when at least one attempted detail answers.
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
	var unseen, stored []string
	var jobs []Job
	for _, loc := range locs {
		id := b.id(loc)
		switch {
		case seen(id) && b.maxAge > 0:
			stored = append(stored, loc)
		case seen(id):
			jobs = append(jobs, Job{ExternalID: id, URL: loc, SeenRefresh: true})
		default:
			unseen = append(unseen, loc)
		}
	}
	// A random slice, not the sitemap's first: a page that stays unreadable or is rejected stays
	// unseen, and a fixed prefix would hand it the same slot every run while later pages starve.
	rand.Shuffle(len(unseen), func(i, j int) { unseen[i], unseen[j] = unseen[j], unseen[i] })
	if deferred := len(unseen) - b.maxNew; deferred > 0 {
		log.Printf("%s: %d new postings past this run's bound of %d, deferred to later runs", b.provider, deferred, b.maxNew)
		unseen = unseen[:b.maxNew]
	}
	// Stored postings are outside the bound: one left unread would be closed as gone.
	unseen = append(unseen, stored...)
	var answered atomic.Int64
	read := fetchDetails(unseen, sitemapBoardDetailWorkers, func(loc string) (Job, bool) {
		job, ok, err := b.detail(ctx, loc)
		if err != nil {
			log.Printf("%s: posting %s failed; deferring: %v", b.provider, loc, err)
			// This board's sitemap identifies the posting but names no employer.
			return unreadableDetail(b.id(loc), loc, ""), true
		}
		answered.Add(1)
		return job, ok
	})
	if len(unseen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("%s: requested %d posting pages and none answered", b.provider, len(unseen))
	}
	return append(jobs, read...), nil
}

// detail returns a nil error for a readable posting or a confirmed removal (404/410).
func (b sitemapBoard) detail(ctx context.Context, loc string) (Job, bool, error) {
	if err := b.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := b.http.GetHTML(ctx, loc)
	if err != nil {
		if detailUnreadable(err) {
			return Job{}, false, err
		}
		return Job{}, false, nil
	}
	var p sitemapBoardPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false, errNoPosting
	}
	title := strings.TrimSpace(p.Title)
	company := strings.TrimSpace(p.HiringOrganization.Name)
	if title == "" || company == "" {
		return Job{}, false, errors.New("posting lacks a title or employer")
	}
	remote := !b.ignoreTelecommute && IsTelecommute(p.JobLocationType)
	job := Job{
		ExternalID:     b.id(loc),
		URL:            loc,
		Title:          title,
		Company:        company,
		Location:       strings.Join(p.localities(), "; "),
		Countries:      countriesFromCodes(b.countryCodes(p)),
		Description:    sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:         remote,
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: schemaEmploymentRaw(p.EmploymentType),
		PostedAt:       NotFuture(parseRFC3339OrDate(p.DatePosted)),
		IsTechHint:     b.techHint,
	}
	if b.maxAge > 0 && job.PostedAt != nil && b.now().Sub(*job.PostedAt) > b.maxAge {
		return Job{}, false, nil
	}
	if b.parseDeadline != nil {
		job.ApplicationDeadline = b.parseDeadline(p.ValidThrough)
	}
	// The board states each posting's own currency (dsml.kz mixes KZT, USD and RUB).
	if salary := p.BaseSalary.amount; salary != nil {
		if currency := strings.ToUpper(strings.TrimSpace(salary.Currency)); currency != "" {
			salary.apply(&job, currency)
		}
	}
	if b.applyURL != nil {
		if target := b.applyURL(root); target != "" {
			job.ApplyMethod, job.ExternalApplyURL = ApplyExternal, target
		}
	}
	if job.Countries == nil && b.country != "" {
		job.FallbackCountries = []string{b.country}
	}
	return job, true, nil
}

// sitemapBoardPosting is the part of a posting page's JobPosting the board adapters read.
type sitemapBoardPosting struct {
	Title           string             `json:"title"`
	Description     string             `json:"description"`
	DatePosted      string             `json:"datePosted"`
	ValidThrough    json.RawMessage    `json:"validThrough"`
	EmploymentType  json.RawMessage    `json:"employmentType"`
	JobLocationType string             `json:"jobLocationType"`
	JobLocation     sitemapBoardPlaces `json:"jobLocation"`
	// ApplicantLocationRequirements is read only by a board that sets applicantCountries.
	// Kept raw so a shape the decoder rejects cannot fail a posting on a board that never reads it.
	ApplicantLocationRequirements json.RawMessage    `json:"applicantLocationRequirements"`
	HiringOrganization            sitemapldjsonOrg   `json:"hiringOrganization"`
	BaseSalary                    sitemapBoardSalary `json:"baseSalary"`
}

// sitemapBoardSalary decodes baseSalary when it is a MonetaryAmount. schema.org also allows a bare
// number or text; such a shape leaves the posting without a salary instead of failing its decode,
// which would drop the whole posting.
type sitemapBoardSalary struct{ amount *schemaMonetaryAmount }

func (s *sitemapBoardSalary) UnmarshalJSON(b []byte) error {
	var a schemaMonetaryAmount
	if json.Unmarshal(b, &a) == nil {
		s.amount = &a
	}
	return nil
}

// sitemapBoardPlaces decodes jobLocation, a Place or an array of them, whose address is in turn
// one PostalAddress or an array: Get in IT states a single Place carrying one address per city.
// A shape it cannot read leaves the posting without a place rather than dropping the posting.
type sitemapBoardPlaces []schemaAddress

func (p *sitemapBoardPlaces) UnmarshalJSON(b []byte) error {
	for _, place := range jsonObjects(b) {
		var one struct {
			Address json.RawMessage `json:"address"`
		}
		if json.Unmarshal(place, &one) != nil {
			continue
		}
		for _, addr := range jsonObjects(one.Address) {
			var a schemaAddress
			if json.Unmarshal(addr, &a) == nil {
				*p = append(*p, a)
			}
		}
	}
	return nil
}

// jsonObjects splits a value that is one JSON object or an array of them; anything else yields
// nothing.
func jsonObjects(b []byte) []json.RawMessage {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil
	}
	if b[0] == '{' {
		return []json.RawMessage{b}
	}
	var all []json.RawMessage
	if b[0] != '[' || json.Unmarshal(b, &all) != nil {
		return nil
	}
	return all
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

func (b sitemapBoard) countryCodes(p sitemapBoardPosting) []string {
	if codes := p.countryCodes(); b.applicantCountries && !slices.ContainsFunc(codes, func(c string) bool { return c != "" }) {
		var areas schemaNamedAreas
		if json.Unmarshal(p.ApplicantLocationRequirements, &areas) == nil {
			return areas.Names()
		}
	}
	return p.countryCodes()
}
