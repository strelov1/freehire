package sources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// lesta adapts Lesta Games' careers site (join.lesta.team), a single-company source with no
// per-tenant board id (boardless). The site's own front end pages through a JSON listing,
// /filters?offset=N (15 a page, with the catalogue's total), that carries every field but the
// body; the body is only on the server-rendered vacancy page, which has no ld+json, so it is read
// from the page's vacancy__item blocks. That request per posting makes the adapter a
// HydratingSource.
type lesta struct {
	http lestaHTTP
}

// lestaHTTP is the transport lesta needs: a JSON listing plus HTML vacancy pages.
type lestaHTTP interface {
	JSONGetter
	HTMLGetter
}

const (
	lestaListURL    = "https://join.lesta.team/filters?offset=%d"
	lestaVacancyURL = "https://join.lesta.team/vacancy/%s/%d"
	// lestaMaxPages is a safety ceiling for a listing that never reaches its own total; reaching it fails the crawl.
	lestaMaxPages = 100
)

// NewLesta builds the Lesta Games adapter over the given HTTP client.
func NewLesta(c lestaHTTP) Source { return lesta{http: c} }

func (lesta) Provider() string { return "lesta" }

// lesta is single-company, so its config entries carry no board.
func (lesta) boardless() {}

// lestaItem is one vacancy from the listing. category_name ("Dev", "Art", "General") and
// project_name (the game) are not read: the rubric is too coarse to be a category — an
// Application Security Engineer sits under "General" — so the title dictionary decides, and no
// Job field holds a project. labels carries "is_leadership", which says the role leads people,
// not how senior it is, so it is not a seniority either.
type lestaItem struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Slug         string   `json:"slug"`
	LocationCity string   `json:"location_city"`
	ExpRange     []string `json:"exp_range"`
	TagsSkills   []string `json:"tags_skills"`
}

func (l lesta) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return l.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew fetches the vacancy page only for a posting the catalogue does not already have. A new
// posting whose page cannot be read is dropped rather than stored body-less: it stays unseen, so
// the next crawl retries it.
func (l lesta) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	items, err := l.list(ctx)
	if err != nil {
		return nil, err
	}
	var wanted, read atomic.Int64
	jobs := fetchDetails(items, defaultDetailWorkers, func(it lestaItem) (Job, bool) {
		job := l.listJob(e, it)
		if seen(job.ExternalID) {
			job.SeenRefresh = true
			return job, true
		}
		wanted.Add(1)
		job, ok := l.detail(ctx, job)
		if ok {
			read.Add(1)
		}
		return job, ok
	})
	// A crawl that listed new postings and read none of them is broken, not quiet.
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("lesta: listed %d new postings and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// list walks the listing by offset until it has read the total the listing itself reports. A page
// that comes back empty before then, or a walk that hits the page ceiling, fails the crawl rather
// than ingesting a silently short catalogue.
func (l lesta) list(ctx context.Context) ([]lestaItem, error) {
	var all []lestaItem
	for page := 0; page < lestaMaxPages; page++ {
		var resp struct {
			Total     int         `json:"total"`
			Vacancies []lestaItem `json:"vacancies"`
		}
		offset := len(all)
		if err := l.http.GetJSON(ctx, fmt.Sprintf(lestaListURL, offset), &resp); err != nil {
			return nil, fmt.Errorf("lesta: list offset %d: %w", offset, err)
		}
		all = append(all, resp.Vacancies...)
		if len(all) >= resp.Total {
			return all, nil
		}
		if len(resp.Vacancies) == 0 {
			return nil, fmt.Errorf("lesta: list offset %d: empty page at %d of %d", offset, len(all), resp.Total)
		}
	}
	return nil, fmt.Errorf("lesta: list: %d pages without reaching the reported total", lestaMaxPages)
}

// listJob maps the listing fields — everything but the body.
func (lesta) listJob(e CompanyEntry, it lestaItem) Job {
	return Job{
		ExternalID:         strconv.FormatInt(it.ID, 10),
		URL:                fmt.Sprintf(lestaVacancyURL, it.Slug, it.ID),
		Title:              strings.TrimSpace(it.Title),
		Company:            e.Company,
		Location:           strings.TrimSpace(it.LocationCity),
		Remote:             isRemote(it.LocationCity),
		Skills:             skilltag.Canonicalize(it.TagsSkills),
		ExperienceYearsMin: lestaExperienceMin(it.ExpRange),
	}
}

// detail reads the vacancy page's body into job, returning ok=false when the page cannot be read
// or carries no body, so the caller skips just that posting.
func (l lesta) detail(ctx context.Context, job Job) (Job, bool) {
	root, err := l.http.GetHTML(ctx, job.URL)
	if err != nil {
		return Job{}, false
	}
	body := lestaDescription(root)
	if body == "" {
		return Job{}, false
	}
	job.Description = sanitizeHTML(body)
	return job, true
}

// lestaDescription joins the page's vacancy__item blocks — company intro, project intro, the
// role, the offer — in page order. The skills block is left out: it repeats tags_skills, which
// the listing already states as structured skills.
func lestaDescription(root *html.Node) string {
	var b strings.Builder
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || !HasClass(n, "vacancy__item") {
			return true
		}
		if firstByClass(n, "tags") == nil {
			b.WriteString(innerHTML(n))
		}
		return false
	})
	return strings.TrimSpace(b.String())
}

// lestaYearsPattern reads the lower bound of an experience band: "От 1 года до 3 лет" → 1,
// "От 3 до 6 лет" → 3, "Более 6 лет" → 6.
var lestaYearsPattern = regexp.MustCompile(`\d+`)

// lestaExperienceMin is the smallest lower bound across the bands a vacancy lists (a vacancy open
// to "1–3" and "3–6" years asks for at least 1). "Без опыта" (no experience) is 0. A band that
// states no number is skipped, and no readable band leaves the minimum unknown.
func lestaExperienceMin(bands []string) *int {
	var lowest *int
	for _, band := range bands {
		years := -1
		if strings.Contains(strings.ToLower(band), "без опыта") {
			years = 0
		} else if m := lestaYearsPattern.FindString(band); m != "" {
			years, _ = strconv.Atoi(m)
		}
		if years >= 0 && (lowest == nil || years < *lowest) {
			lowest = &years
		}
	}
	return lowest
}
