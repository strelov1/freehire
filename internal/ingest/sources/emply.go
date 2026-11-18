package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// emply adapts Emply (emply.com), a Danish ATS behind many Danish employers' career sites —
// municipalities, regions and private companies alike. A career site lives either at
// <tenant>.career.emply.com or on the customer's own domain (midtjob.dk, job.paychex.eu), and
// both serve the same keyless JSON endpoint the site's own job list calls:
//
//	POST https://<host>/api/integration/vacancy/get-page
//	{"count": N, "offset": K, "filters": []}
//
// which answers {"vacancies": [...], "count": <total>}. A vacancy carries its whole posting —
// title, location, publish date and the full HTML body in every language it is written in —
// so the listing is the crawl and no detail request is made. `filters` is required: without
// it the endpoint answers 500.
//
// The board is the tenant slug (`molslinjen`) or, for a site on a customer domain, that host
// (`midtjob.dk`); emplyHost tells them apart by the dot, and BoardDedupeKey folds a slug and
// its <slug>.career.emply.com host into one target.
type emply struct {
	http JSONPoster
}

// NewEmply builds the Emply adapter over the given HTTP client.
func NewEmply(c JSONPoster) Source { return emply{http: c} }

func (emply) Provider() string { return "emply" }

// fullBoardListing: the walk pages by offset to the listing's own `count` and then requires
// the distinct vacancy ids read to equal it. Any page failing, a page answering empty before
// the count is reached, a page repeating what was already read, or the page ceiling being hit
// fails the crawl — none of them is reported as a complete board.
func (emply) fullBoardListing() {}

const (
	emplyCareerApex = ".career.emply.com"
	emplyListPath   = "/api/integration/vacancy/get-page"
	// emplyPageSize is the count asked per page. The endpoint honours far larger values, so this
	// only keeps one response to a sane size; the walk does not depend on it being honoured.
	emplyPageSize = 100
	// emplyMaxPages is a ceiling, not an expected end: reaching it before the count is read
	// fails the crawl.
	emplyMaxPages = 100
)

// emplyHost resolves a board to its career-site host: a board with a dot is already a host
// (a customer domain, or a full <slug>.career.emply.com); a bare slug is an Emply subdomain.
func emplyHost(board string) string {
	board = strings.ToLower(strings.TrimSpace(board))
	if strings.Contains(board, ".") {
		return board
	}
	return board + emplyCareerApex
}

// emplyPageRequest is the listing request body, the fields the site's own script sends that
// the endpoint needs.
type emplyPageRequest struct {
	Count   int   `json:"count"`
	Offset  int   `json:"offset"`
	Filters []any `json:"filters"`
}

// emplyPage is one listing response. Vacancies is a pointer so a response without the array
// (an error document) is told apart from an empty board.
type emplyPage struct {
	Vacancies *[]emplyVacancy `json:"vacancies"`
	Count     int             `json:"count"`
}

type emplyVacancy struct {
	ID         string `json:"id"`
	ShortID    string `json:"shortId"`
	Title      string `json:"title"`
	TitleAsURL string `json:"titleAsUrl"`
	// TalentPool marks the site's standing "unsolicited application" entry, not a vacancy.
	TalentPool bool   `json:"talentPool"`
	Location   string `json:"location"`
	Published  string `json:"published"`
	// ExternalAdLink is set when the site links the vacancy to a page elsewhere instead of
	// its own ad page; the site's script prefers it, and so does the adapter.
	ExternalAdLink string             `json:"externalCseAdLink"`
	Translations   []emplyTranslation `json:"translations"`
}

type emplyTranslation struct {
	Title       string `json:"title"`
	Content     string `json:"content"`
	LanguageKey string `json:"languageKey"`
}

func (s emply) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	host := emplyHost(e.Board)
	vacancies, err := s.list(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("emply: board %s: %w", e.Board, err)
	}
	jobs := make([]Job, 0, len(vacancies))
	for _, v := range vacancies {
		if job, ok := emplyJob(host, e.Company, v); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// list walks the listing by offset until it has read the listing's own count, and proves the
// read complete by the distinct ids equalling that count.
func (s emply) list(ctx context.Context, host string) ([]emplyVacancy, error) {
	endpoint := "https://" + host + emplyListPath
	var all []emplyVacancy
	seen := map[string]bool{}
	offset, total := 0, 0
	for page := 0; ; page++ {
		if page == emplyMaxPages {
			return nil, fmt.Errorf("page ceiling %d reached at %d of %d vacancies", emplyMaxPages, offset, total)
		}
		var resp emplyPage
		req := emplyPageRequest{Count: emplyPageSize, Offset: offset, Filters: []any{}}
		if err := s.http.PostJSON(ctx, endpoint, req, &resp); err != nil {
			return nil, fmt.Errorf("listing at offset %d: %w", offset, err)
		}
		if resp.Vacancies == nil {
			return nil, fmt.Errorf("listing at offset %d: response carries no vacancies array", offset)
		}
		total = resp.Count
		batch := *resp.Vacancies
		for _, v := range batch {
			if !seen[v.ID] {
				seen[v.ID] = true
				all = append(all, v)
			}
		}
		offset += len(batch)
		if len(batch) == 0 || offset >= total {
			break
		}
	}
	if len(seen) != total {
		return nil, fmt.Errorf("read %d distinct vacancies, listing declares %d", len(seen), total)
	}
	return all, nil
}

// translation picks the text a vacancy is read in: English when it is written in English,
// otherwise its first language — the order the platform lists them in starts with the
// posting's primary one. A translation with no body is passed over while another has one.
func (v emplyVacancy) translation() emplyTranslation {
	var first, english *emplyTranslation
	for i := range v.Translations {
		t := &v.Translations[i]
		if strings.TrimSpace(t.Content) == "" {
			continue
		}
		if first == nil {
			first = t
		}
		if english == nil && strings.HasPrefix(strings.ToLower(t.LanguageKey), "en") {
			english = t
		}
	}
	switch {
	case english != nil:
		return *english
	case first != nil:
		return *first
	case len(v.Translations) > 0:
		return v.Translations[0]
	}
	return emplyTranslation{}
}

// emplyJob maps one vacancy. The talent-pool entry and a vacancy with no id or title are not
// postings.
func emplyJob(host, company string, v emplyVacancy) (Job, bool) {
	if v.TalentPool || v.ShortID == "" {
		return Job{}, false
	}
	tr := v.translation()
	title := strings.Join(strings.Fields(tr.Title), " ")
	if title == "" {
		title = strings.Join(strings.Fields(v.Title), " ")
	}
	if title == "" {
		return Job{}, false
	}
	jobURL := strings.TrimSpace(v.ExternalAdLink)
	if jobURL == "" {
		// The ad page is keyed by the short id alone (any slug resolves); the trailing
		// language segment opens the page in the language that was read.
		slug := v.TitleAsURL
		if slug == "" {
			slug = "ad"
		}
		jobURL = "https://" + host + "/ad/" + url.PathEscape(slug) + "/" + url.PathEscape(v.ShortID)
		if lang, _, _ := strings.Cut(tr.LanguageKey, "-"); lang != "" {
			jobURL += "/" + strings.ToLower(lang)
		}
	}
	location := strings.Join(strings.Fields(v.Location), " ")
	return Job{
		ExternalID:  v.ShortID,
		URL:         jobURL,
		Title:       title,
		Company:     company,
		Location:    location,
		Description: sanitizeHTML(tr.Content),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(v.Published),
	}, true
}
