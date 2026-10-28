package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// servicepublic adapts Choisir le service public (choisirleservicepublic.gouv.fr), the French
// civil service's own job board: ~54k open postings from the State, local authorities and
// public hospitals, every trade, under the government's "Numérique" domain for its IT roles. It
// is boardless and an aggregator — the employer is read per posting. See the "Choisir le service
// public traps" section of AGENTS.md for the measurements behind each rule.
//
// The listing is the site's own paginated offer list, newest first, 25 cards a page. A card
// names the title, the employer, the place, the domain and the day it went online, so the
// coverage gate is asked before a page is bought; the body is the posting page. A whole walk is
// ~2,200 pages of ~190 KB (each page embeds every search filter), too much for one run, so a run
// reads the head of the list until it is all known, plus a window of the rest that moves on every
// run, and the unseen sweep waits long enough for the window to come round.
type servicepublic struct {
	http    HTMLGetter
	limiter waiter
	// headPages bounds the newest-first walk; windowPages is how many further pages a run reads
	// to refresh what it already stores.
	headPages   int
	windowPages int
	// maxNewPerRun bounds the posting pages one run reads.
	maxNewPerRun   int
	deadlineMargin time.Duration
	now            func() time.Time
}

const (
	servicepublicHost    = "choisirleservicepublic.gouv.fr"
	servicepublicBaseURL = "https://" + servicepublicHost
	servicepublicListURL = servicepublicBaseURL + "/nos-offres/"
	servicepublicPageURL = servicepublicBaseURL + "/nos-offres/page/%d/"

	// servicepublicRequestInterval paces every request at one a second — a courtesy ceiling
	// toward a public service, not a measured limit.
	servicepublicRequestInterval = time.Second
	servicepublicDetailWorkers   = 2
	// servicepublicHeadPages stops the newest-first walk on a quiet list: ~1,200 postings go
	// online a day (~50 pages), so a run three hours after the last meets known postings within
	// a few pages, and a cold start reads this many before turning to the window.
	servicepublicHeadPages = 150
	// servicepublicWindowPages is the refresh window: 300 pages a run, advanced by 100 pages an
	// hour, comes round a ~2,500-page cycle in about a day at a three-hour cadence.
	servicepublicWindowPages = 300
	servicepublicWindowStep  = 100
	// servicepublicWindowCycle is the granularity the window's cycle is rounded up to.
	servicepublicWindowCycle = 500
	// servicepublicSweepGrace leaves the window three turns to come round before a stored
	// posting it has not re-listed is closed.
	servicepublicSweepGrace = 72 * time.Hour
	// servicepublicMaxNewPerRun bounds the posting pages of one run: up to ~450 listing pages
	// and 900 posting pages at one a second, plus the writes, fit the 3,000 s run timeout this
	// provider is scheduled with. A cold catalogue is taken newest first over many runs.
	servicepublicMaxNewPerRun   = 900
	servicepublicDeadlineMargin = 5 * time.Minute
)

// NewServicePublic builds the adapter over the given client.
func NewServicePublic(c HTMLGetter) Source {
	return servicepublic{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(servicepublicRequestInterval), 1),
		headPages:      servicepublicHeadPages,
		windowPages:    servicepublicWindowPages,
		maxNewPerRun:   servicepublicMaxNewPerRun,
		deadlineMargin: servicepublicDeadlineMargin,
		now:            time.Now,
	}
}

func (servicepublic) Provider() string { return "servicepublic" }

// One national board, no per-tenant id.
func (servicepublic) boardless() {}

// Thousands of public employers post here, each named per posting.
func (servicepublic) aggregator() {}

// A run re-lists only part of the catalogue, so the sweep waits for the window to come round.
func (servicepublic) sweepGrace() time.Duration { return servicepublicSweepGrace }

// servicepublicCard is one listing card.
type servicepublicCard struct {
	id       string
	url      string
	title    string
	employer string
	place    string
	domain   string
}

// servicepublicPostingPath matches a posting URL, /offre-emploi/<title>-reference-<reference>/.
// The reference ("2026-2453798", "MENJ-30-2026-49681") is the identity; the title slug is
// cosmetic.
var servicepublicPostingPath = regexp.MustCompile(`^/offre-emploi/([a-z0-9-]+?)-reference-([a-z0-9-]+)/?$`)

// servicepublicParseURL reads a posting URL on the site's own host.
func servicepublicParseURL(raw string) (id, canonical string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host != servicepublicHost {
		return "", "", false
	}
	m := servicepublicPostingPath.FindStringSubmatch(strings.ToLower(u.Path))
	if m == nil {
		return "", "", false
	}
	return m[2], servicepublicBaseURL + u.Path, true
}

// servicepublicPageCount reads the list's page count from its title, "(page 2 sur 2160)".
var servicepublicPageCount = regexp.MustCompile(`page\s+\d+\s+sur\s+(\d+)`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s servicepublic) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s servicepublic) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated reads the head of the list and this run's window, refreshes the postings already
// stored, yields body-less the ones the gate condemns on their card, and reads up to maxNewPerRun
// of the rest, newest first. A posting whose page cannot be read is not yielded; a later run tries
// again.
func (s servicepublic) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	cards, err := s.list(ctx, seen)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		employers := make([]string, 0, len(cards))
		for _, c := range cards {
			employers = append(employers, c.employer)
		}
		covered = gate.Covered(employers)
	}
	var jobs []Job
	var unseen []servicepublicCard
	deferred := 0
	for _, c := range cards {
		base := servicepublicBaseJob(c)
		if seen(c.id) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[c.employer] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, c)
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, servicepublicDetailWorkers, func(c servicepublicCard) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < s.deadlineMargin {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("servicepublic: posting %s failed; deferring: %v", c.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	if asked := int64(len(unseen)) - outOfTime.Load(); asked > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("servicepublic: requested %d posting pages and none answered", asked)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("servicepublic: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("servicepublic: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads the head of the list — newest first, until a page holds no posting the catalogue
// lacks, at most headPages — and then this run's window of the pages after it. The head failing
// fails the crawl; a window page failing ends the window, since the window only refreshes what
// is already stored and a later run comes round to the rest.
func (s servicepublic) list(ctx context.Context, seen func(string) bool) ([]servicepublicCard, error) {
	ids := make(map[string]bool)
	var out []servicepublicCard
	keep := func(cards []servicepublicCard) (fresh int) {
		for _, c := range cards {
			if ids[c.id] {
				continue
			}
			ids[c.id] = true
			out = append(out, c)
			if !seen(c.id) {
				fresh++
			}
		}
		return fresh
	}
	first, pages, err := s.page(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("servicepublic: listing page 1: %w", err)
	}
	if len(first) == 0 {
		return nil, errors.New("servicepublic: listing carries no posting")
	}
	head := 1
	if keep(first) > 0 {
		for head < min(s.headPages, pages) {
			head++
			cards, _, err := s.page(ctx, head)
			if err != nil {
				return nil, fmt.Errorf("servicepublic: listing page %d: %w", head, err)
			}
			if len(cards) == 0 || keep(cards) == 0 {
				break
			}
		}
	}
	for _, p := range servicepublicWindow(head, pages, s.windowPages, s.now()) {
		cards, _, err := s.page(ctx, p)
		if err != nil {
			log.Printf("servicepublic: window page %d failed, ending the window: %v", p, err)
			break
		}
		keep(cards)
	}
	return out, nil
}

// servicepublicWindow is the run's refresh window: up to windowPages absolute page numbers past
// the head, at an offset that advances by servicepublicWindowStep every hour around a cycle of
// the page count rounded up to the next 500. The cycle must not follow the head or the exact page
// count, both of which move between runs (the head ends wherever known postings begin, the list
// grows ~50 pages a day): an offset taken modulo a value that moves by one jumps to an arbitrary
// page, and consecutive windows stop sweeping the list in order. Rounded up, the cycle changes
// only every ten days or so, and the pages of the cycle past the list's end are skipped.
func servicepublicWindow(head, pages, windowPages int, now time.Time) []int {
	if pages <= head || windowPages <= 0 {
		return nil
	}
	cycle := (pages/servicepublicWindowCycle + 1) * servicepublicWindowCycle
	start := int((now.Unix() / 3600 * servicepublicWindowStep) % int64(cycle))
	var out []int
	for i := 0; i < windowPages; i++ {
		if p := 1 + (start+i)%cycle; p > head && p <= pages {
			out = append(out, p)
		}
	}
	return out
}

// page reads one listing page: its cards, and the list's page count.
func (s servicepublic) page(ctx context.Context, n int) ([]servicepublicCard, int, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, 0, err
	}
	u := servicepublicListURL
	if n > 1 {
		u = fmt.Sprintf(servicepublicPageURL, n)
	}
	root, err := s.http.GetHTML(ctx, u)
	if err != nil {
		return nil, 0, err
	}
	pages := 0
	if m := servicepublicPageCount.FindStringSubmatch(titleText(root)); m != nil {
		pages, _ = strconv.Atoi(m[1])
	}
	var cards []servicepublicCard
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && servicepublicHasClass(n, "fr-card--offer") {
			if c, ok := servicepublicReadCard(n); ok {
				cards = append(cards, c)
			}
			return false
		}
		return true
	})
	return cards, pages, nil
}

func servicepublicHasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(Attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

func servicepublicText(n *html.Node) string {
	if n == nil {
		return ""
	}
	return strings.Join(strings.Fields(textContent(n)), " ")
}

// servicepublicReadCard reads one offer card. The card's facts are list items, each opened by a
// screen-reader label ("Employeur : ").
func servicepublicReadCard(card *html.Node) (servicepublicCard, bool) {
	title := firstByClass(card, "fr-card__title")
	if title == nil {
		return servicepublicCard{}, false
	}
	a := firstByTag(title, "a")
	if a == nil {
		return servicepublicCard{}, false
	}
	id, canonical, ok := servicepublicParseURL(Attr(a, "href"))
	if !ok {
		return servicepublicCard{}, false
	}
	c := servicepublicCard{id: id, url: canonical, title: servicepublicText(a), domain: servicepublicText(firstByClass(card, "fr-tag"))}
	if desc := firstByClass(card, "fr-card__desc"); desc != nil {
		for li := desc.FirstChild; li != nil; li = li.NextSibling {
			if li.Type != html.ElementNode || li.Data != "li" {
				continue
			}
			label := servicepublicText(firstByClass(li, "sr-only"))
			value := strings.TrimSpace(strings.TrimPrefix(servicepublicText(li), label))
			switch {
			case strings.HasPrefix(label, "Employeur"):
				c.employer = value
			case strings.HasPrefix(label, "Localisation"):
				c.place = value
			}
		}
	}
	if c.title == "" || c.employer == "" {
		return servicepublicCard{}, false
	}
	return c, true
}

// servicepublicTechDomain is the government's own domain for its IT roles.
const servicepublicTechDomain = "Numérique"

// servicepublicBaseJob is the body-less Job a card alone supports.
func servicepublicBaseJob(c servicepublicCard) Job {
	return Job{
		ExternalID:        c.id,
		URL:               c.url,
		Title:             c.title,
		Company:           c.employer,
		Location:          c.place,
		IsTechHint:        strings.EqualFold(c.domain, servicepublicTechDomain),
		FallbackCountries: []string{"fr"},
	}
}

// servicepublicLD is the part of the posting page's schema.org JobPosting the adapter reads.
// Its description loses the page's structure and its salary is prose, so neither is read.
type servicepublicLD struct {
	Title              string `json:"title"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization string `json:"hiringOrganization"`
	Industry           string `json:"industry"`
	JobLocation        struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Country  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

// errServicepublicNoPosting marks a page that answered without a posting body: not the site's
// answer that the posting is gone, so no answer.
var errServicepublicNoPosting = errors.New("page carries no posting body")

// servicepublicUnpublished is the title of the page a withdrawn posting answers, under HTTP 200.
const servicepublicUnpublished = "Offre dépubliée"

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410, or the "Offre dépubliée" page); an error is the absence of an answer.
func (s servicepublic) detail(ctx context.Context, c servicepublicCard) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := s.http.GetHTML(ctx, c.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if strings.HasPrefix(titleText(root), servicepublicUnpublished) {
		return Job{}, false, nil
	}
	body := servicepublicBody(root)
	if body == "" {
		return Job{}, false, errServicepublicNoPosting
	}
	var ld servicepublicLD
	ldJobPosting(root, &ld)
	j := servicepublicBaseJob(c)
	j.Description = body
	if t := strings.Join(strings.Fields(ld.Title), " "); t != "" {
		j.Title = t
	}
	if org := strings.Join(strings.Fields(ld.HiringOrganization), " "); org != "" {
		j.Company = org
	}
	if ld.Industry != "" {
		j.IsTechHint = strings.EqualFold(strings.TrimSpace(ld.Industry), servicepublicTechDomain)
	}
	if code := location.NormalizeCountry(strings.TrimSpace(ld.JobLocation.Address.Country)); code != "" {
		j.Countries = []string{code}
	}
	if t, err := time.Parse("02/01/2006", strings.TrimSpace(ld.DatePosted)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	j.WorkMode = servicepublicWorkMode(servicepublicFact(root, "Télétravail possible"))
	return j, true, nil
}

// servicepublicBody is the page's own body column — "Vos missions en quelques mots", "Profil
// recherché" and the rest — with its "show more" buttons removed.
func servicepublicBody(root *html.Node) string {
	var col *html.Node
	walk(root, func(n *html.Node) bool {
		if col != nil {
			return false
		}
		if n.Type == html.ElementNode && servicepublicHasClass(n, "col-left") && servicepublicHasClass(n, "rte") {
			col = n
			return false
		}
		return true
	})
	if col == nil {
		return ""
	}
	var buttons []*html.Node
	walk(col, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "button" {
			buttons = append(buttons, n)
			return false
		}
		return true
	})
	for _, b := range buttons {
		b.Parent.RemoveChild(b)
	}
	body := sanitizeHTML(innerHTML(col))
	if strings.TrimSpace(servicepublicText(col)) == "" {
		return ""
	}
	return body
}

// servicepublicFact is the value of the posting's summary line with the given label: a <strong>
// label followed, in the same list item, by a span holding the value.
func servicepublicFact(root *html.Node, label string) string {
	var value string
	walk(root, func(n *html.Node) bool {
		if value != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "li" && servicepublicHasClass(n, "table-cell") {
			if strong := firstByTag(n, "strong"); strong != nil && servicepublicText(strong) == label {
				value = strings.TrimSpace(strings.TrimPrefix(servicepublicText(n), label))
				return false
			}
		}
		return true
	})
	return value
}

// servicepublicWorkMode reads the "Télétravail possible" line: telework that is possible is
// partial in the civil service, so "Oui" is hybrid and "Non" on-site; "Non renseigné" states
// nothing.
func servicepublicWorkMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "oui":
		return "hybrid"
	case "non":
		return "onsite"
	}
	return ""
}
