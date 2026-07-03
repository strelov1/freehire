package sources

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
)

// gupyBaseURL is the Gupy public portal jobs API. Gupy is the dominant Brazilian
// ATS (Creditas, Afya, Cogna, Omie, …); a company's per-tenant career page lives at
// <subdomain>.gupy.io, but the listing API is this one central host keyed by the
// numeric companyId.
const gupyBaseURL = "https://employability-portal.gupy.io/api/v1/jobs"

// gupyPageLimit is the listing page size. The API caps limit at 100 (limit=200 is a
// 400), so this is also the maximum.
const gupyPageLimit = 100

// gupyMaxPages bounds the offset walk. The stop signal is a short/empty page, but a
// misbehaving API that always returned a full page would otherwise loop forever, so
// this caps the walk at 5000 postings — well above any single company's openings.
const gupyMaxPages = 50

// gupyDetailURL is Gupy's richer public job endpoint, keyed by the same numeric job id as
// the listing. The portal listing (gupyBaseURL) flattens a posting's sections into one
// tag-less description blob — headings glued to sentences, list items separated only by
// ";" — so its description renders as an unstructured wall of text. This endpoint returns
// the original per-section HTML instead; it is what the public career page itself consumes.
const gupyDetailURL = "https://private-api.gupy.io/job-publication/public/jobs"

// gupy adapts the Gupy portal API. Its list endpoint carries the description inline,
// so — like Greenhouse — it needs no per-posting detail request. The board id is the
// company's numeric Gupy companyId.
type gupy struct {
	http JSONGetter
}

// NewGupy builds the Gupy adapter over the given HTTP client.
func NewGupy(c JSONGetter) Source { return gupy{http: c} }

func (gupy) Provider() string { return "gupy" }

// gupyJob is one posting from the Gupy listing. The description is inline; jobUrl is an
// absolute public URL; workplaceType is a structured "remote"/"hybrid"/"on-site" enum.
type gupyJob struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	JobURL        string `json:"jobUrl"`
	Description   string `json:"description"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
	IsRemoteWork  bool   `json:"isRemoteWork"`
	WorkplaceType string `json:"workplaceType"`
	PublishedDate string `json:"publishedDate"`
}

func (g gupy) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := g.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if p.JobURL == "" { // url is the dedup key — a posting without one is unusable
			continue
		}
		jobs = append(jobs, Job{
			ExternalID:  strconv.FormatInt(p.ID, 10),
			URL:         p.JobURL,
			Title:       strings.TrimSpace(p.Name),
			Company:     e.Company,
			Location:    joinNonEmpty(p.City, p.State, p.Country),
			Description: g.description(ctx, p.ID, p.Description),
			Remote:      p.IsRemoteWork,
			WorkMode:    workplaceTypeMode(p.WorkplaceType),
			PostedAt:    parseRFC3339(p.PublishedDate),
		})
	}
	return jobs, nil
}

// list pages through the company's postings. It stops on an empty or short page rather
// than on pagination.total: when limit == page size, Gupy reports total = min(real, limit),
// so a full first page would falsely look complete. A short page is the reliable last-page
// signal (the same reasoning SmartRecruiters' listPostings relies on totalFound for, but
// Gupy's total can't be trusted).
func (g gupy) list(ctx context.Context, board string) ([]gupyJob, error) {
	var postings []gupyJob
	for offset, page := 0, 0; page < gupyMaxPages; page++ {
		url := fmt.Sprintf("%s?companyId=%s&limit=%d&offset=%d", gupyBaseURL, board, gupyPageLimit, offset)
		var resp struct {
			Data []gupyJob `json:"data"`
		}
		if err := g.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("gupy: list company %s: %w", board, err)
		}
		if len(resp.Data) == 0 {
			break
		}
		postings = append(postings, resp.Data...)
		if len(resp.Data) < gupyPageLimit {
			break // short page = last page
		}
		offset += len(resp.Data)
	}
	return postings, nil
}

// gupyDetail is a posting's structured detail. Gupy splits a job into fixed sections;
// concatenating them in board order reproduces the full description with its original
// markup (paragraphs, lists, emphasis) that the flat listing description discards.
type gupyDetail struct {
	Description         string `json:"description"`         // intro / "about the role"
	Responsibilities    string `json:"responsibilities"`    // Responsabilidades e atribuições
	Prerequisites       string `json:"prerequisites"`       // Requisitos e qualificações
	RelevantExperiences string `json:"relevantExperiences"` // Informações adicionais / benefits
}

// description fetches the posting's structured detail and assembles its sections into one
// sanitized HTML document. The flat listing description is the fallback when the detail
// endpoint is unreachable or yields nothing usable, so a detail hiccup degrades to the old
// behaviour rather than dropping the body.
func (g gupy) description(ctx context.Context, id int64, flat string) string {
	var d gupyDetail
	if err := g.http.GetJSON(ctx, fmt.Sprintf("%s/%d", gupyDetailURL, id), &d); err == nil {
		if assembled := gupySections(d); assembled != "" {
			return sanitizeHTML(assembled)
		}
	}
	return sanitizeHTML(flat)
}

// gupySections concatenates the detail's section fragments in display order, skipping any
// that carry no real text (Gupy commonly emits a "<p>.</p>" placeholder intro). It returns
// "" when every section is empty, signalling the caller to fall back to the flat listing.
func gupySections(d gupyDetail) string {
	var b strings.Builder
	for _, section := range []string{d.Description, d.Responsibilities, d.Prerequisites, d.RelevantExperiences} {
		if gupyHasText(section) {
			b.WriteString(section)
		}
	}
	return b.String()
}

// gupyTextPolicy strips all markup, leaving only text — used to tell a real section from a
// placeholder like "<p>.</p>" or "<p>&nbsp;</p>". Compiled once, safe for concurrent use.
var gupyTextPolicy = bluemonday.StrictPolicy()

// gupyHasText reports whether an HTML fragment contains any letter or digit once tags and
// entities are stripped, so whitespace/punctuation-only sections are treated as empty.
func gupyHasText(fragment string) bool {
	for _, r := range html.UnescapeString(gupyTextPolicy.Sanitize(fragment)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
