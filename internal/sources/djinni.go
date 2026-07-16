package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// djinni adapts djinni.co, a Ukrainian/CEE IT job board. Each anonymous listing page
// (/jobs/?page=N) embeds one application/ld+json block that is an ARRAY of full schema.org
// JobPosting objects — description included — so one fetch per page yields every posting
// without a per-posting detail request. Boardless (djinni.co is one site with no per-tenant
// board) and an aggregator (many companies; each posting's company comes from the feed), so it
// stays in the source facet and inherits the reindex aggregator/ATS suppression.
type djinni struct {
	http HTMLResolvedGetter
}

const (
	// djinniListBase is the guest listing; the caller appends the "page=N" (1-based) marker,
	// which doubles as the end-of-feed sentinel checked against the resolved final URL.
	djinniListBase = "https://djinni.co/jobs/?"
	// djinniMaxPages caps pagination as a backstop, comfortably above the observed corpus
	// (~488 pages). The empty-page stop below is the primary terminator; this only bounds a
	// pathological feed that never empties.
	djinniMaxPages = 600
)

// djinniPageDelay paces the sequential page crawl. Djinni rate-limits a fast burst from a
// datacenter IP with a 403 (a no-delay crawl 403'd around page ~200 from prod, while the same
// pages spaced ~1s apart return 200), so we throttle to stay under the limit and be a polite
// crawler; partial-on-error bounds the damage if a 403 still lands. A var so tests zero it.
var djinniPageDelay = 600 * time.Millisecond

// NewDjinni builds the djinni listing adapter over the given HTML client.
func NewDjinni(c HTMLResolvedGetter) Source { return djinni{http: c} }

func (djinni) Provider() string { return "djinni" }

func (djinni) boardless() {}

func (djinni) aggregator() {}

// Fetch pages the listing from page 1 upward, mapping each page's JSON-LD JobPosting array to
// jobs. It stops at the end of the feed, detected by the redirect: a past-the-end page 302s to
// the bare listing (/jobs/), so the FINAL URL no longer carries the requested page marker.
// (Following the redirect would otherwise re-serve page 1 indefinitely, since page 1 is not
// empty.) A genuinely empty non-redirected page is a secondary stop.
//
// A page fetch that fails partway (Djinni 403s a datacenter IP that crawls too fast) does NOT
// discard the crawl: the pages already collected are the freshest postings (Djinni orders by
// recency), so Fetch keeps them and stops. It fails the whole board only when page 1 itself
// fails — an empty successful crawl would otherwise let the unseen-sweep close the catalogue.
func (s djinni) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= djinniMaxPages; page++ {
		if page > 1 {
			select {
			case <-ctx.Done():
				return jobs, ctx.Err()
			case <-time.After(djinniPageDelay):
			}
		}
		pageMarker := fmt.Sprintf("page=%d", page)
		root, final, err := s.http.GetHTMLResolved(ctx, djinniListBase+pageMarker)
		if err != nil {
			if len(jobs) == 0 {
				return nil, fmt.Errorf("djinni: fetch page %d: %w", page, err)
			}
			log.Printf("djinni: page %d failed (%v); keeping %d jobs from pages 1..%d", page, err, len(jobs), page-1)
			break // partial crawl — keep the freshest pages rather than losing everything
		}
		if !strings.Contains(final, pageMarker) {
			break // redirected off the end of the feed (past-the-end page 302s to /jobs/)
		}
		nodes := LDJobPostings(root)
		if len(nodes) == 0 {
			break // a non-redirected page with no postings — nothing more to read
		}
		for _, raw := range nodes {
			var p djinniPosting
			if json.Unmarshal(raw, &p) != nil {
				continue // a posting that fails to decode is skipped, never aborting the page
			}
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
	}
	return jobs, nil
}

// djinniPosting is one schema.org JobPosting from the listing. identifier is a bare integer;
// jobLocationType (TELECOMMUTE/ON_SITE) is the work-arrangement signal; the applicant location
// requirement carries only a country.
type djinniPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	URL                string `json:"url"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	JobLocationType    string `json:"jobLocationType"`
	Identifier         int64  `json:"identifier"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	ApplicantLocationRequirements struct {
		Address struct {
			AddressCountry string `json:"addressCountry"`
		} `json:"address"`
	} `json:"applicantLocationRequirements"`
}

// toJob maps a posting to a Job, returning ok=false for a posting with no identifier (no dedup
// key), no url (no canonical address), or no company (which would break the company slug).
func (p djinniPosting) toJob() (Job, bool) {
	if p.Identifier == 0 || p.URL == "" || p.HiringOrganization.Name == "" {
		return Job{}, false
	}
	remote := strings.EqualFold(p.JobLocationType, "TELECOMMUTE")
	return Job{
		ExternalID:     strconv.FormatInt(p.Identifier, 10),
		URL:            p.URL,
		Title:          p.Title,
		Company:        p.HiringOrganization.Name,
		Location:       p.ApplicantLocationRequirements.Address.AddressCountry,
		Description:    sanitizeHTML(djinniDescriptionHTML(p.Description)),
		Remote:         remote,
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       djinniDate(p.DatePosted),
	}, true
}

// djinniDate parses Djinni's datePosted, an ISO-8601 local datetime WITHOUT a zone (e.g.
// "2026-07-16T04:43:20.486231"). parseRFC3339 rejects the missing zone, so this reads it as
// UTC, trying parseRFC3339 first (should Djinni ever emit a zoned value) and a bare date last.
func djinniDate(s string) *time.Time {
	if t := parseRFC3339(s); t != nil {
		return t
	}
	if t := parseLayout("2006-01-02T15:04:05.999999999", s); t != nil {
		return t
	}
	return parseDate(s)
}

// djinniBulletMarkers are the line-leading glyphs Djinni company editors use to mark list items
// in the otherwise plain-text JSON-LD description (bullet, middle dot, triangular/hollow/filled
// bullets, hyphen, asterisk, en/em dash). A marker counts only when it leads a line and is
// followed by whitespace, so a mid-sentence dash, a hyphenated word, or the "*Note" footnote
// prefix stays prose.
var djinniBulletMarkers = map[rune]bool{
	'•': true, '·': true, '‣': true, '◦': true, '▪': true,
	'-': true, '*': true, '–': true, '—': true,
}

// djinniBullet reports whether a trimmed line is a bullet, returning its text with the leading
// marker (and the space after it) removed.
func djinniBullet(line string) (string, bool) {
	r, size := utf8.DecodeRuneInString(line)
	if !djinniBulletMarkers[r] {
		return "", false
	}
	next, _ := utf8.DecodeRuneInString(line[size:])
	if !unicode.IsSpace(next) {
		return "", false // "-word" / "*Note" without a following space is prose, not a bullet
	}
	return strings.TrimSpace(line[size:]), true
}

// djinniDescriptionHTML rebuilds the structural HTML the {@html} consumer renders from Djinni's
// plain-text description. The feed carries the body as newline-delimited text — blank lines
// separate blocks, assorted leading glyphs mark bullets — with no markup, so rendered as-is every
// newline collapses into one unbroken wall of text. This reconstructs it: a run of bullet lines
// becomes a <ul>, a run of other text lines becomes a <p> (wrapped lines joined by <br>), and a
// blank line closes the open block. Text is HTML-escaped because it is literal prose, not markup.
func djinniDescriptionHTML(text string) string {
	var out strings.Builder
	var para, bullets []string
	flushPara := func() {
		if len(para) > 0 {
			out.WriteString("<p>" + strings.Join(para, "<br>") + "</p>")
			para = para[:0]
		}
	}
	flushBullets := func() {
		if len(bullets) > 0 {
			out.WriteString("<ul>")
			for _, li := range bullets {
				out.WriteString("<li>" + li + "</li>")
			}
			out.WriteString("</ul>")
			bullets = bullets[:0]
		}
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			flushBullets()
			flushPara()
			continue
		}
		if item, ok := djinniBullet(line); ok {
			flushPara()
			bullets = append(bullets, html.EscapeString(item))
			continue
		}
		flushBullets()
		para = append(para, html.EscapeString(line))
	}
	flushBullets()
	flushPara()
	return out.String()
}
