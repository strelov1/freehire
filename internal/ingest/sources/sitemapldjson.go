package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"math/rand/v2"
	"net/url"
	"regexp"
	"strings"
)

const (
	sitemapLDJSONMaxNew   = 200
	sitemapLDJSONWorkers  = 2
	sitemapLDJSONMaxDocs  = 1000
	sitemapLDJSONMaxDepth = 3
)

type sitemapldjson struct {
	http   sitemapldjsonHTTP
	maxNew int
}

type sitemapldjsonHTTP interface {
	HTMLGetter
	LargeTextGetter
}

type sitemapldjsonHit struct {
	url string
	id  string
}

func NewSitemapLDJSON(c sitemapldjsonHTTP) Source {
	return sitemapldjson{http: c, maxNew: sitemapLDJSONMaxNew}
}

func (sitemapldjson) Provider() string { return "sitemapldjson" }

func (s sitemapldjson) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.crawl(ctx, e, func(string) bool { return false })
}

func (s sitemapldjson) FetchNew(ctx context.Context, e CompanyEntry, seen func(string) bool) ([]Job, error) {
	if seen == nil {
		seen = func(string) bool { return false }
	}
	return s.crawl(ctx, e, seen)
}

func (s sitemapldjson) limit() int {
	if s.maxNew <= 0 {
		return sitemapLDJSONMaxNew
	}
	return s.maxNew
}

func (s sitemapldjson) crawl(ctx context.Context, e CompanyEntry, seen func(string) bool) ([]Job, error) {
	board, err := sitemapLDJSONBoardURL(e.Board)
	if err != nil {
		return nil, err
	}
	hits, err := s.list(ctx, board, e.Region)
	if err != nil {
		return nil, fmt.Errorf("sitemapldjson: %w", err)
	}
	var refresh []Job
	var fresh []sitemapldjsonHit
	for _, hit := range hits {
		if seen(hit.id) {
			refresh = append(refresh, Job{
				ExternalID:  hit.id,
				URL:         hit.url,
				Company:     e.Company,
				SeenRefresh: true,
			})
			continue
		}
		fresh = append(fresh, hit)
	}
	rand.Shuffle(len(fresh), func(i, j int) {
		fresh[i], fresh[j] = fresh[j], fresh[i]
	})
	deferred := 0
	if len(fresh) > s.limit() {
		deferred = len(fresh) - s.limit()
		fresh = fresh[:s.limit()]
	}
	if deferred > 0 {
		log.Printf("sitemapldjson: hydration budget %d reached; %d new pages deferred to a later run", s.limit(), deferred)
	}
	jobs := fetchDetails(fresh, sitemapLDJSONWorkers, func(hit sitemapldjsonHit) (Job, bool) {
		return s.detail(ctx, e, hit)
	})
	return append(refresh, jobs...), nil
}

func (s sitemapldjson) list(ctx context.Context, board, pattern string) ([]sitemapldjsonHit, error) {
	seenDoc := map[string]bool{}
	seenID := map[string]bool{}
	match := urlPattern(pattern)
	var hits []sitemapldjsonHit
	var walk func(pageURL string, depth int) error
	walk = func(pageURL string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seenDoc[pageURL] {
			return nil
		}
		if len(seenDoc) >= sitemapLDJSONMaxDocs {
			return fmt.Errorf("more than %d sitemap documents", sitemapLDJSONMaxDocs)
		}
		seenDoc[pageURL] = true
		body, err := s.http.GetLargeText(ctx, pageURL)
		if err != nil {
			return fmt.Errorf("sitemap %s: %w", pageURL, err)
		}
		doc, err := decodeSitemapBody([]byte(body))
		if err != nil {
			return fmt.Errorf("sitemap %s: %w", pageURL, err)
		}
		if len(doc.Sitemaps) > 0 {
			if depth >= sitemapLDJSONMaxDepth {
				return fmt.Errorf("sitemap index nested past %d at %s", sitemapLDJSONMaxDepth, pageURL)
			}
			for _, sm := range doc.Sitemaps {
				child := resolveSitemapLoc(pageURL, sm.Loc)
				if child == "" {
					continue
				}
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		}
		for _, entry := range doc.URLs {
			loc := resolveSitemapLoc(pageURL, entry.Loc)
			if loc == "" || !match(loc) {
				continue
			}
			id := sitemapLDJSONID(loc)
			if id == "" || seenID[id] {
				continue
			}
			seenID[id] = true
			hits = append(hits, sitemapldjsonHit{url: loc, id: id})
		}
		return nil
	}
	if err := walk(board, 0); err != nil {
		return nil, err
	}
	return hits, nil
}

func (s sitemapldjson) detail(ctx context.Context, e CompanyEntry, hit sitemapldjsonHit) (Job, bool) {
	root, err := s.http.GetHTML(ctx, hit.url)
	if err != nil {
		log.Printf("sitemapldjson: detail %s: %v", hit.url, err)
		return Job{}, false
	}
	var p sitemapldjsonPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	company := strings.TrimSpace(p.HiringOrganization.Name)
	if company == "" {
		company = e.Company
	}
	title := strings.TrimSpace(p.Title)
	if title == "" || company == "" {
		return Job{}, false
	}
	remote := strings.EqualFold(strings.TrimSpace(p.JobLocationType), "TELECOMMUTE")
	loc := p.location()
	return Job{
		ExternalID:     hit.id,
		URL:            hit.url,
		Title:          title,
		Company:        company,
		Location:       loc,
		Countries:      p.countries(),
		Description:    sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:         remote || isRemote(loc),
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: schemaEmploymentRaw(p.EmploymentType),
		PostedAt:       parseRFC3339OrDate(p.DatePosted),
	}, true
}

type sitemapldjsonPosting struct {
	Title                         string              `json:"title"`
	Description                   string              `json:"description"`
	DatePosted                    string              `json:"datePosted"`
	EmploymentType                json.RawMessage     `json:"employmentType"`
	JobLocationType               string              `json:"jobLocationType"`
	JobLocation                   sitemapldjsonPlaces `json:"jobLocation"`
	ApplicantLocationRequirements schemaNamedAreas    `json:"applicantLocationRequirements"`
	HiringOrganization            sitemapldjsonOrg    `json:"hiringOrganization"`
}

type sitemapldjsonPlaces struct {
	schemaPlaces
}

func (p *sitemapldjsonPlaces) UnmarshalJSON(b []byte) error {
	var places schemaPlaces
	if err := json.Unmarshal(b, &places); err != nil {
		return nil
	}
	p.schemaPlaces = places
	return nil
}

type sitemapldjsonOrg struct {
	Name string
}

func (o *sitemapldjsonOrg) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var name string
		if json.Unmarshal(b, &name) == nil {
			o.Name = name
		}
		return nil
	}
	var obj struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(b, &obj) == nil {
		o.Name = obj.Name
	}
	return nil
}

func (p sitemapldjsonPosting) location() string {
	var parts []string
	for _, place := range p.JobLocation.schemaPlaces {
		if loc := place.Address.Location(); loc != "" {
			parts = append(parts, loc)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "; ")
	}
	return strings.Join(p.ApplicantLocationRequirements.Names(), ", ")
}

func (p sitemapldjsonPosting) countries() []string {
	var codes []string
	located := false
	for _, place := range p.JobLocation.schemaPlaces {
		if place.Address.Location() == "" {
			continue
		}
		located = true
		codes = append(codes, place.Address.AddressCountry)
	}
	if located {
		return countriesFromCodes(codes)
	}
	return countriesFromCodes(p.ApplicantLocationRequirements.Names())
}

func schemaEmploymentRaw(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '[' {
		var values []string
		if json.Unmarshal(raw, &values) != nil {
			return ""
		}
		for _, value := range values {
			if mapped := schemaEmploymentType(value); mapped != "" {
				return mapped
			}
		}
		return ""
	}
	var one string
	if json.Unmarshal(raw, &one) != nil {
		return ""
	}
	return schemaEmploymentType(one)
}

func sitemapLDJSONBoardURL(board string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(board))
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("sitemapldjson: board %q is not an absolute http(s) sitemap URL", board)
	}
	return u.String(), nil
}

func resolveSitemapLoc(base, loc string) string {
	loc = strings.TrimSpace(loc)
	if loc == "" {
		return ""
	}
	u, err := url.Parse(loc)
	if err != nil {
		return ""
	}
	if !u.IsAbs() {
		b, err := url.Parse(base)
		if err != nil {
			return ""
		}
		u = b.ResolveReference(u)
	}
	if u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.String()
}

func sitemapLDJSONID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	u.RawQuery = ""
	u.Fragment = ""
	if u.Path != "/" {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	return u.String()
}

func urlMatchesPattern(raw, pattern string) bool {
	return urlPattern(pattern)(raw)
}

func urlPattern(pattern string) func(string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return func(string) bool { return true }
	}
	if !strings.Contains(pattern, "*") {
		return func(raw string) bool { return strings.Contains(raw, pattern) }
	}
	var b strings.Builder
	b.WriteByte('^')
	for i, part := range strings.Split(pattern, "*") {
		if i > 0 {
			b.WriteString(".*")
		}
		b.WriteString(regexp.QuoteMeta(part))
	}
	b.WriteByte('$')
	return regexp.MustCompile(b.String()).MatchString
}
