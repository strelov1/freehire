package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"golang.org/x/net/html"
)

// kontur adapts SKB Kontur's careers site (kontur.ru/career/vacancies), a single-company source
// with no per-tenant board id (boardless). The listing is one server-rendered page holding every
// vacancy as an <a class="vacancy"> card grouped by rubric; the vacancy page,
// /career/vacancies/<id>, carries a schema.org JobPosting ld+json block plus the full body in its
// main text column. The body costs a request per posting, so the adapter is a HydratingSource.
//
// Egress: kontur.ru throttles datacenter IPs (every HTML page stops after ~20 KB and never
// completes), so the provider is in proxiedProviders. A request without a browser Accept header
// gets a short stub with no vacancies; the shared client's GetHTML always sends "Accept:
// text/html", and kontur_test.go pins that the listing request carries it.
type kontur struct {
	http HTMLGetter
}

const (
	konturListURL    = "https://kontur.ru/career/vacancies"
	konturVacancyURL = "https://kontur.ru/career/vacancies/%s"
	// konturListContainer marks the rendered vacancy list. A page without it did not render the
	// list at all (a stub or a truncated render), which is a failure, not an empty catalogue.
	konturListContainer = "data-vacancy-list-container"
)

// NewKontur builds the Kontur adapter over the given HTTP client.
func NewKontur(c HTMLGetter) Source { return kontur{http: c} }

func (kontur) Provider() string { return "kontur" }

// kontur is single-company, so its config entries carry no board.
func (kontur) Boardless() {}

// konturCard is one vacancy as the listing shows it: the id and the title. The card's location
// and work-format lines are summaries ("Екатеринбург и ещё 8 городов") of what the vacancy page
// states in full, so they are read there.
type konturCard struct {
	ID    string
	Title string
}

func (k kontur) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return k.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew fetches the vacancy page only for a posting the catalogue does not already have. A new
// posting whose page cannot be read is dropped rather than stored body-less: it stays unseen, so
// the next crawl retries it. A crawl that listed new postings and read none of them is broken,
// not quiet, and fails.
func (k kontur) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	cards, err := k.list(ctx)
	if err != nil {
		return nil, err
	}
	var wanted, read atomic.Int64
	jobs := fetchDetails(cards, defaultDetailWorkers, func(c konturCard) (Job, bool) {
		url := fmt.Sprintf(konturVacancyURL, c.ID)
		if seen(c.ID) {
			return Job{ExternalID: c.ID, URL: url, Title: c.Title, Company: e.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		job, err := k.detail(ctx, e, c)
		if err != nil {
			log.Printf("kontur: vacancy %s: %v; deferring", c.ID, err)
			return Job{}, false
		}
		read.Add(1)
		return job, true
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("kontur: listed %d new postings and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// list reads every vacancy card off the listing page, deduplicated by id.
func (k kontur) list(ctx context.Context) ([]konturCard, error) {
	root, err := k.http.GetHTML(ctx, konturListURL)
	if err != nil {
		return nil, fmt.Errorf("kontur: listing: %w", err)
	}
	if !konturHasList(root) {
		return nil, errors.New("kontur: listing: the page rendered no vacancy list")
	}
	var cards []konturCard
	seen := make(map[string]bool)
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" || !HasClass(n, "vacancy") {
			return true
		}
		id := firstSubmatch(konturVacancyIDPattern, Attr(n, "href"))
		if id != "" && !seen[id] {
			seen[id] = true
			cards = append(cards, konturCard{ID: id, Title: konturCardTitle(n)})
		}
		return false
	})
	return cards, nil
}

// konturVacancyIDPattern captures the numeric id of a /career/vacancies/<id> link. The listing's
// city filter links (/career/vacancies/city-5919) carry no numeric id and are not matched.
var konturVacancyIDPattern = regexp.MustCompile(`^/career/vacancies/(\d+)$`)

func konturHasList(root *html.Node) bool {
	found := false
	walk(root, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type == html.ElementNode && Attr(n, konturListContainer) != "" {
			found = true
		}
		return !found
	})
	return found
}

func konturCardTitle(card *html.Node) string {
	if t := firstByClass(card, "vacancy__title"); t != nil {
		return strings.TrimSpace(textContent(t))
	}
	return ""
}

// konturPosting is the JobPosting ld+json on a vacancy page. hiringOrganization is always Kontur
// itself and is not read; the configured company is canonical.
type konturPosting struct {
	Title                         string           `json:"title"`
	Description                   string           `json:"description"`
	DatePosted                    string           `json:"datePosted"`
	EmploymentType                string           `json:"employmentType"`
	JobLocationType               string           `json:"jobLocationType"`
	JobLocation                   schemaPlaces     `json:"jobLocation"`
	ApplicantLocationRequirements schemaNamedAreas `json:"applicantLocationRequirements"`
}

// detail reads one vacancy page into a Job.
func (k kontur) detail(ctx context.Context, e CompanyEntry, c konturCard) (Job, error) {
	url := fmt.Sprintf(konturVacancyURL, c.ID)
	root, err := k.http.GetHTML(ctx, url)
	if err != nil {
		return Job{}, err
	}
	var p konturPosting
	if !ldJobPosting(root, &p) {
		return Job{}, errors.New("no JobPosting on the page")
	}

	labels := konturLabels(root)
	remote := strings.EqualFold(p.JobLocationType, "TELECOMMUTE")
	location := konturLocation(p.JobLocation)
	return Job{
		ExternalID:         c.ID,
		URL:                url,
		Title:              firstNonEmpty(strings.TrimSpace(p.Title), c.Title),
		Company:            e.Company,
		Location:           location,
		Description:        sanitizeHTML(konturDescription(root, p.Description)),
		Remote:             remote,
		WorkMode:           konturWorkMode(remote, labels),
		EmploymentType:     schemaEmploymentType(p.EmploymentType),
		ExperienceYearsMin: konturExperienceMin(labels),
		Countries:          konturCountries(p),
		PostedAt:           parseDate(p.DatePosted),
	}, nil
}

// konturDescription prefers the page's main text column, which holds the whole body (intro, team,
// tasks, requirements, offer); the ld+json description is a lead that, for some vacancies, is a
// single sentence. The ld+json text is the fallback when the column is missing.
func konturDescription(root *html.Node, ld string) string {
	if block := konturTextColumn(root); block != nil {
		if body := strings.TrimSpace(innerHTML(block)); body != "" {
			return body
		}
	}
	return plainTextToHTML(html.UnescapeString(ld))
}

// konturTextColumn is the first element whose class is exactly "text" — the main column. The page
// also carries "js-successMessage text none" form messages further down, which a class-token match
// would accept.
func konturTextColumn(root *html.Node) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if n.Type == html.ElementNode && Attr(n, "class") == "text" {
			found = n
			return false
		}
		return true
	})
	return found
}

// konturLocation joins each jobLocation place's city, deduplicated, in page order. The country
// goes to Countries rather than repeating "RU" after every city.
func konturLocation(places schemaPlaces) string {
	var out []string
	seen := make(map[string]bool)
	for _, pl := range places {
		city := strings.TrimSpace(pl.Address.AddressLocality)
		if city == "" || seen[city] {
			continue
		}
		seen[city] = true
		out = append(out, city)
	}
	return strings.Join(out, "; ")
}

// konturCountries collects the structured countries: each place's addressCountry, plus the
// applicant location requirement a remote-capable vacancy states.
func konturCountries(p konturPosting) []string {
	var codes []string
	for _, pl := range p.JobLocation {
		codes = append(codes, pl.Address.AddressCountry)
	}
	codes = append(codes, p.ApplicantLocationRequirements.Names()...)
	return countriesFromCodes(codes)
}

// konturLabels returns the vacancy page's sidebar labels — the site's own picklists for work
// format ("В офисе", "В офисе или гибридно", "В офисе, удалённо или гибридно"), schedule and
// experience ("Опыт от 3 лет", "Можно без опыта") — whitespace-normalized.
func konturLabels(root *html.Node) []string {
	var labels []string
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && HasClass(n, "vacancy-label__title") {
			labels = append(labels, strings.Join(strings.Fields(textContent(n)), " "))
			return false
		}
		return true
	})
	return labels
}

// konturWorkMode reads the work arrangement. TELECOMMUTE in the ld+json is the structured remote
// signal and matches the "…удалённо…" label exactly; without it, the work-format label is the
// site's own fixed picklist, of which only two values name an arrangement unambiguously.
func konturWorkMode(remote bool, labels []string) string {
	if remote {
		return "remote"
	}
	for _, l := range labels {
		switch strings.ToLower(l) {
		case "в офисе или гибридно":
			return "hybrid"
		case "в офисе":
			return "onsite"
		}
	}
	return ""
}

// konturYearsPattern reads the number from the experience label "Опыт от 3 лет" / "Опыт от 1 года".
var konturYearsPattern = regexp.MustCompile(`(?i)^опыт от (\d+)`)

// konturExperienceMin reads the experience label: "Опыт от N …" is N, "Можно без опыта" is 0, and
// a vacancy with no experience label leaves the minimum unknown.
func konturExperienceMin(labels []string) *int {
	for _, l := range labels {
		if strings.EqualFold(l, "Можно без опыта") {
			zero := 0
			return &zero
		}
		if m := firstSubmatch(konturYearsPattern, l); m != "" {
			n, err := strconv.Atoi(m)
			if err == nil {
				return &n
			}
		}
	}
	return nil
}
