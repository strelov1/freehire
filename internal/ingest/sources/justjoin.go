package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/dict/vocab"
)

// justjoinSite adapts one of the two Polish job boards that run on the Just Join IT platform:
// justjoin.it, the country's largest IT board (~11k open offers), and rocketjobs.pl, its sibling
// for every other trade (~16k). Both are boardless aggregators — thousands of employers and
// agencies post there, each named per offer. See the "Just Join IT and RocketJobs" section of
// AGENTS.md for the measurements behind each rule.
//
// The offer set is the site's active-jobs sitemap, which names only each offer's URL and when it
// was last touched; the platform's JSON API is disallowed by both hosts' robots.txt, so every
// fact is read from the offer page's server-rendered Next.js flight, which carries the whole
// offer object (salary per contract type, workplace type, skills, cities). A multi-city offer is
// one sitemap entry — its parent slug — so the slug is the identity. The adapter hydrates only
// offers the catalogue lacks, at most freshPerRun+backlogPerRun pages a run, and refreshes the
// rest by identity.
type justjoinSite struct {
	http     justjoinHTTP
	limiter  waiter
	provider string
	// host is the site's own host; sitemapURL its active-jobs sitemap index; offerPath the path
	// prefix of an offer page.
	host       string
	sitemapURL string
	offerPath  string
	// tech marks the IT board: every offer on justjoin.it is filed under one of its IT
	// categories by the employer, which is the structured "confirmed technical" signal
	// IsTechHint carries. rocketjobs.pl is every other trade and asserts nothing.
	tech bool

	freshPerRun    int
	backlogPerRun  int
	deadlineMargin time.Duration
	now            func() time.Time
}

type justjoinHTTP interface {
	XMLGetter
	TextGetter
}

const (
	// justjoinRequestInterval paces every request of a run, sitemap and offer page alike — a
	// courtesy ceiling, not a measured limit. Both sites served a plain client from the crawl
	// host without refusal.
	justjoinRequestInterval = time.Second
	justjoinDetailWorkers   = 2
	// justjoinFreshPerRun and justjoinBacklogPerRun split one run's 1,400 offer pages (~100 KB
	// compressed each, ~24 minutes at one a second, inside the scheduler's default 2,400 s crawl
	// budget) into hydrationLanes' two lanes. The sitemap names no employer, so nothing can be
	// judged before a page is bought; an offer the pipeline discards unsaved (an employer already
	// covered first-party) stays unseen and would be bought again on every run, which the backlog
	// lane keeps from starving the older offers.
	justjoinFreshPerRun    = 1100
	justjoinBacklogPerRun  = 300
	justjoinDeadlineMargin = 5 * time.Minute
	// justjoinMaxSitemaps bounds the sitemap index: each site filled one part.
	justjoinMaxSitemaps = 20
)

// NewJustJoin builds the justjoin.it adapter (IT offers).
func NewJustJoin(c justjoinHTTP) Source {
	return newJustJoinSite(c, "justjoin", "justjoin.it", "/job-offer/", true)
}

// NewRocketJobs builds the rocketjobs.pl adapter (every other trade, same platform).
func NewRocketJobs(c justjoinHTTP) Source {
	return newJustJoinSite(c, "rocketjobs", "rocketjobs.pl", "/oferta-pracy/", false)
}

func newJustJoinSite(c justjoinHTTP, provider, host, offerPath string, tech bool) justjoinSite {
	return justjoinSite{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(justjoinRequestInterval), 1),
		provider:       provider,
		host:           host,
		sitemapURL:     "https://" + host + "/sitemaps/active-jobs.xml",
		offerPath:      offerPath,
		tech:           tech,
		freshPerRun:    justjoinFreshPerRun,
		backlogPerRun:  justjoinBacklogPerRun,
		deadlineMargin: justjoinDeadlineMargin,
		now:            time.Now,
	}
}

func (s justjoinSite) Provider() string { return s.provider }

// One national board each, no per-tenant id.
func (justjoinSite) boardless() {}

// Thousands of employers and agencies post here, each named per offer.
func (justjoinSite) aggregator() {}

// justjoinPosting is one sitemap entry: the offer's slug, its URL and when the site last touched
// it.
type justjoinPosting struct {
	slug    string
	url     string
	lastmod time.Time
}

// justjoinSlug matches an offer slug: lowercase ASCII words joined by hyphens, as both sites
// write them.
var justjoinSlug = regexp.MustCompile(`^[a-z0-9]+(?:-+[a-z0-9]+)*$`)

// parsePosting reads a sitemap entry, or ok=false when it is not an offer URL on the site's own
// host.
func (s justjoinSite) parsePosting(loc sitemapLoc) (justjoinPosting, bool) {
	u, err := url.Parse(strings.TrimSpace(loc.Loc))
	if err != nil || u.Scheme != "https" || u.Host != s.host {
		return justjoinPosting{}, false
	}
	slug, ok := strings.CutPrefix(u.Path, s.offerPath)
	if !ok || !justjoinSlug.MatchString(slug) {
		return justjoinPosting{}, false
	}
	p := justjoinPosting{slug: slug, url: "https://" + s.host + s.offerPath + slug}
	if t := parseRFC3339(strings.TrimSpace(loc.LastMod)); t != nil {
		p.lastmod = *t
	}
	return p, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s justjoinSite) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every offer the sitemap names, refreshes the ones already stored, and reads the
// pages hydrationLanes picks out of the rest. An offer whose page cannot be read is not yielded;
// a later run tries again.
func (s justjoinSite) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []justjoinPosting
	for _, p := range postings {
		if seen(p.slug) {
			jobs = append(jobs, Job{ExternalID: p.slug, URL: p.url, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, s.freshPerRun, s.backlogPerRun, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, justjoinDetailWorkers, func(p justjoinPosting) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("%s: offer %s failed; deferring: %v", s.provider, p.slug, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts an offer the site
	// reported gone; offers left for want of time are not failures.
	if int64(len(picked)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("%s: requested %d offer pages and none answered", s.provider, int64(len(picked))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("%s: %d new offers left unread as the run neared its deadline, deferred to later runs", s.provider, n)
	}
	if deferred := len(unseen) - len(picked); deferred > 0 {
		log.Printf("%s: %d new offers past this run's bound of %d, deferred to later runs", s.provider, deferred, len(picked))
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNew returns, so one that
// runs into its deadline stores nothing at all.
func (s justjoinSite) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list reads the sitemap index and every part it names, and returns their offers, most recently
// touched first. The sitemap is the whole listing, so any failure fails the crawl: a partial
// listing would let the sweep close the offers on the part it did not read.
func (s justjoinSite) list(ctx context.Context) ([]justjoinPosting, error) {
	index, err := s.sitemap(ctx, s.sitemapURL)
	if err != nil {
		return nil, fmt.Errorf("%s: sitemap index: %w", s.provider, err)
	}
	if len(index.Sitemaps) == 0 || len(index.Sitemaps) > justjoinMaxSitemaps {
		return nil, fmt.Errorf("%s: sitemap index names %d parts", s.provider, len(index.Sitemaps))
	}
	seen := make(map[string]bool)
	var out []justjoinPosting
	for _, part := range index.Sitemaps {
		doc, err := s.sitemap(ctx, strings.TrimSpace(part.Loc))
		if err != nil {
			return nil, fmt.Errorf("%s: sitemap %s: %w", s.provider, part.Loc, err)
		}
		for _, loc := range doc.URLs {
			p, ok := s.parsePosting(loc)
			if !ok || seen[p.slug] {
				continue
			}
			seen[p.slug] = true
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: sitemap names no offer", s.provider)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].lastmod.Equal(out[j].lastmod) {
			return out[i].lastmod.After(out[j].lastmod)
		}
		return out[i].slug < out[j].slug
	})
	return out, nil
}

func (s justjoinSite) sitemap(ctx context.Context, u string) (sitemapDoc, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return sitemapDoc{}, err
	}
	return getSitemap(ctx, s.http, u)
}

// justjoinOffer is the part of an offer page's flight offer object the adapter reads.
type justjoinOffer struct {
	Slug            string          `json:"slug"`
	Title           string          `json:"title"`
	CompanyName     string          `json:"companyName"`
	Body            string          `json:"body"`
	City            string          `json:"city"`
	CountryCode     string          `json:"countryCode"`
	ExperienceLevel justjoinValue   `json:"experienceLevel"`
	WorkplaceType   justjoinValue   `json:"workplaceType"`
	WorkingTime     justjoinValue   `json:"workingTime"`
	RequiredSkills  []justjoinSkill `json:"requiredSkills"`
	EmploymentTypes []justjoinPay   `json:"employmentTypes"`
	Languages       []struct {
		Code  string `json:"code"`
		Level string `json:"level"`
	} `json:"languages"`
	PublishedAt   string `json:"publishedAt"`
	IsOfferActive *bool  `json:"isOfferActive"`
}

type justjoinValue struct {
	Value string `json:"value"`
}

type justjoinSkill struct {
	Name string `json:"name"`
}

// justjoinPay is one contract type's pay. The platform lists every contract type once in its
// original currency and again converted into four others; only the original is the employer's
// statement.
type justjoinPay struct {
	Type           string   `json:"type"`
	Currency       string   `json:"currency"`
	CurrencySource string   `json:"currencySource"`
	From           *float64 `json:"from"`
	To             *float64 `json:"to"`
	Gross          bool     `json:"gross"`
	Unit           string   `json:"unit"`
}

// errJustJoinNoOffer marks a page that answered without an offer object. An unknown slug answers
// 404, so this is the absence of an answer (a changed page, a challenge), not a gone offer.
var errJustJoinNoOffer = errors.New("page carries no offer")

// detail reads one offer page. ok=false with a nil error is the site's answer that the offer is
// gone (404/410, or the page's own isOfferActive=false — the sitemap still names an expired offer
// for about a day) or carries no usable posting; an error is the absence of an answer.
func (s justjoinSite) detail(ctx context.Context, p justjoinPosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	page, err := s.http.GetText(ctx, p.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	flight, err := decodeNextFlightText(page)
	if err != nil {
		return Job{}, false, err
	}
	raw, found := bracketSlice(flight, `"offer":{"slug"`, '{', '}')
	if !found {
		return Job{}, false, errJustJoinNoOffer
	}
	var o justjoinOffer
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		return Job{}, false, fmt.Errorf("decode offer: %w", err)
	}
	if o.IsOfferActive != nil && !*o.IsOfferActive {
		return Job{}, false, nil
	}
	if ref, isRef := strings.CutPrefix(o.Body, "$"); isRef {
		// A reference the flight does not carry is a changed page, not an empty offer: it
		// must stay an error, or every page could read as answered while yielding nothing.
		row, found := nextFlightTextRows(flight)[ref]
		if !found {
			return Job{}, false, fmt.Errorf("body row %q not found in flight", ref)
		}
		o.Body = row
	}
	job, ok := s.job(o, p)
	return job, ok, nil
}

// job maps an offer to a Job. ok is false for one with no title, employer or body.
func (s justjoinSite) job(o justjoinOffer, p justjoinPosting) (Job, bool) {
	title := strings.Join(strings.Fields(o.Title), " ")
	company := strings.Join(strings.Fields(o.CompanyName), " ")
	body := strings.TrimSpace(o.Body)
	if title == "" || company == "" || body == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:        p.slug,
		URL:               p.url,
		Title:             title,
		Company:           company,
		Location:          strings.TrimSpace(o.City),
		Countries:         countryFromCode(o.CountryCode),
		FallbackCountries: []string{"pl"},
		WorkMode:          workplaceTypeMode(o.WorkplaceType.Value),
		Seniority:         justjoinSeniority(o.ExperienceLevel.Value),
		EmploymentType:    justjoinEmploymentType(o.WorkingTime.Value),
		Skills:            justjoinSkills(o.RequiredSkills),
		EnglishLevel:      justjoinEnglish(o),
		IsTechHint:        s.tech,
	}
	j.Remote = j.WorkMode == "remote"
	if t := parseRFC3339(strings.TrimSpace(o.PublishedAt)); t != nil {
		j.PostedAt = NotFuture(t)
	}
	pays := justjoinOriginalPays(o.EmploymentTypes)
	if pick, ok := justjoinPreferredPay(pays); ok {
		lo, hi := int(*pick.From), int(*pick.To)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = strings.ToUpper(pick.Currency)
		j.SalaryPeriod = pick.Unit
	}
	j.Description = sanitizeHTML(justjoinPayParagraph(pays) + body)
	return j, true
}

// justjoinOriginalPays keeps the pay the employer stated: the original-currency entries with a
// full range, an ISO currency and a period our vocabulary knows.
func justjoinOriginalPays(all []justjoinPay) []justjoinPay {
	var out []justjoinPay
	for _, e := range all {
		if e.CurrencySource != "original" || e.From == nil || e.To == nil || *e.From <= 0 || *e.To < *e.From {
			continue
		}
		if len(strings.TrimSpace(e.Currency)) != 3 || !slices.Contains(vocab.SalaryPeriodValues, e.Unit) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// justjoinPreferredPay picks the one range the structured salary carries: an employment contract
// (umowa o pracę, stated gross) when the offer has one, else the first other contract type. The
// salary facet has room for one range and no contract or gross/net label, so the rest — and the
// label of the chosen one — are stated in the paragraph justjoinPayParagraph writes.
func justjoinPreferredPay(pays []justjoinPay) (justjoinPay, bool) {
	for _, e := range pays {
		if e.Type == "permanent" {
			return e, true
		}
	}
	if len(pays) > 0 {
		return pays[0], true
	}
	return justjoinPay{}, false
}

// justjoinContractLabels names the platform's contract types the way Polish offers do.
var justjoinContractLabels = map[string]string{
	"permanent":              "umowa o pracę",
	"b2b":                    "B2B",
	"mandate_contract":       "umowa zlecenie",
	"specific_task_contract": "umowa o dzieło",
	"internship":             "staż",
}

var justjoinUnitLabels = map[string]string{"month": "mies.", "hour": "godz.", "day": "dzień", "year": "rok"}

// justjoinPayParagraph states every contract type's pay with its gross/net basis, which in Poland
// differs by contract (an employment contract is quoted gross, B2B net of VAT) and which the
// structured salary cannot carry. Empty when the offer states no amount.
func justjoinPayParagraph(pays []justjoinPay) string {
	if len(pays) == 0 {
		return ""
	}
	var lines []string
	for _, e := range pays {
		basis := "netto"
		if e.Gross {
			basis = "brutto"
		}
		line := fmt.Sprintf("%s – %s %s / %s %s",
			strconv.Itoa(int(*e.From)), strconv.Itoa(int(*e.To)), strings.ToUpper(e.Currency), justjoinUnitLabels[e.Unit], basis)
		if label := justjoinContractLabels[e.Type]; label != "" {
			line = label + ": " + line
		}
		lines = append(lines, line)
	}
	return "<p><strong>Wynagrodzenie</strong><br>" + strings.Join(lines, "<br>") + "</p>\n"
}

// justjoinSkills canonicalizes the required skills through the skilltag dictionary, keeping only
// resolved technologies. Canonicalize, not Parse: these are discrete names from a structured
// field, not prose to mine.
func justjoinSkills(skills []justjoinSkill) []string {
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Name)
	}
	return skilltag.Canonicalize(names)
}

// justjoinSeniority maps the experience level to our seniority vocabulary. The platform names the
// middle grade "mid"; intern/junior/senior/c_level share spelling, so after that one rename
// vocabulary membership is the map, and an unknown level (e.g. "manager") drops.
func justjoinSeniority(level string) string {
	l := strings.ToLower(strings.TrimSpace(level))
	if l == "mid" {
		l = "middle"
	}
	if slices.Contains(vocab.SeniorityValues, l) {
		return l
	}
	return ""
}

// justjoinEmploymentType maps the working time. The contract types (B2B, employment, mandate)
// say how the work is paid, not whether it is full-time, so they are not read here.
func justjoinEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full_time":
		return "full_time"
	case "part_time":
		return "part_time"
	case "internship":
		return "internship"
	}
	return ""
}

// justjoinEnglish reads the CEFR level the offer requires in English, when it states one.
func justjoinEnglish(o justjoinOffer) string {
	for _, l := range o.Languages {
		if !strings.EqualFold(strings.TrimSpace(l.Code), "en") {
			continue
		}
		if level := strings.ToLower(strings.TrimSpace(l.Level)); slices.Contains(vocab.EnglishLevelValues, level) {
			return level
		}
	}
	return ""
}
