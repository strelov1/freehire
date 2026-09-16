package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// japandev adapts Japan Dev, a curated multi-company technology job board for Japan.
// The public sitemap is the cheap listing: it names almost the whole live catalogue without
// requiring Algolia credentials or client-side execution. Each posting page is server-rendered
// and carries its primary job as structured Nuxt SSR state, including the employer, body,
// compensation, work arrangement and (when the employer supplied one) the official ATS apply URL.
//
// The sitemap and /jobs count are close but not identical (290 posting URLs vs 294 jobs measured
// live 2026-09-12), so this adapter deliberately does NOT claim fullCatalog/fullBoardListing.
// Missing a URL from this listing must never be treated as structural proof that the posting died.
type japandev struct {
	http japandevHTTP
}

type japandevHTTP interface {
	XMLGetter
	HTMLGetter
}

const japanDevSitemapIndexURL = "https://japan-dev.com/sitemap.xml"

func NewJapanDev(c japandevHTTP) Source { return japandev{http: c} }

func (japandev) Provider() string { return "japandev" }
func (japandev) boardless()       {}
func (japandev) aggregator()      {}

// japanDevListing is the identity the sitemap gives us for one posting. ExternalID is the
// posting slug, not Japan Dev's numeric database id: the slug is available before hydration,
// which is what lets HydratingSource skip a detail request for every already-seen posting.
type japanDevListing struct {
	ExternalID string
	URL        string
}

func (s japandev) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, list, func(string) bool { return false }), nil
}

func (s japandev) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, list, seen), nil
}

// list follows Japan Dev's sitemap index to its CDN urlset and keeps only canonical posting URLs.
// The root indirection is followed rather than hard-coding the CDN path so a future sitemap move is
// harmless. An empty/shape-drifted sitemap is a board failure, not an empty-success catalogue.
func (s japandev) list(ctx context.Context) ([]japanDevListing, error) {
	index, err := getSitemap(ctx, s.http, japanDevSitemapIndexURL)
	if err != nil {
		return nil, fmt.Errorf("japandev: sitemap index: %w", err)
	}
	var leaf string
	for _, sm := range index.Sitemaps {
		if strings.Contains(sm.Loc, "/sitemaps/") {
			leaf = sm.Loc
			break
		}
	}
	if leaf == "" {
		return nil, fmt.Errorf("japandev: sitemap index contains no posting sitemap")
	}
	doc, err := getSitemap(ctx, s.http, leaf)
	if err != nil {
		return nil, fmt.Errorf("japandev: posting sitemap: %w", err)
	}
	seen := map[string]bool{}
	out := make([]japanDevListing, 0, len(doc.URLs))
	for _, entry := range doc.URLs {
		id := japanDevJobID(entry.Loc)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, japanDevListing{ExternalID: id, URL: entry.Loc})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("japandev: posting sitemap contains no canonical job URLs")
	}
	return out, nil
}

// japanDevJobID accepts only https://japan-dev.com/jobs/<company>/<posting> and returns the
// posting slug. Category/filter pages under /jobs have a different path depth and are ignored.
func japanDevJobID(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "japan-dev.com") {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "jobs" || parts[1] == "" || parts[2] == "" {
		return ""
	}
	return parts[2]
}

// hydrate spends a page request only on a posting the catalogue does not already hold. A detail
// failure defers that new posting rather than storing a body-less row which would become permanently
// SeenRefresh on the next crawl and never get another chance to hydrate.
func (s japandev) hydrate(ctx context.Context, list []japanDevListing, seen func(string) bool) []Job {
	return fetchDetails(list, defaultDetailWorkers, func(item japanDevListing) (Job, bool) {
		if seen(item.ExternalID) {
			return Job{ExternalID: item.ExternalID, URL: item.URL, SeenRefresh: true}, true
		}
		root, err := s.http.GetHTML(ctx, item.URL)
		if err != nil {
			log.Printf("japandev: detail %s failed; deferring: %v", item.ExternalID, err)
			return Job{}, false
		}
		job, ok := parseJapanDevJob(root, item)
		if !ok {
			log.Printf("japandev: detail %s has no usable primary job; deferring", item.ExternalID)
			return Job{}, false
		}
		return job, true
	})
}

// japanDevNuxtJob is the structured subset of Japan Dev's primary SSR job object. The page uses
// Nuxt/devalue's reference table rather than a plain nested JSON object; parseJapanDevNuxtJob
// resolves only these scalar/list fields and intentionally ignores the rest of the application state.
type japanDevNuxtJob struct {
	Title             string
	Slug              string
	Company           string
	Location          string
	RawContent        string
	ApplicationURL    string
	PublishedAt       string
	UpdatedAt         string
	RemoteLevel       string
	EmploymentType    string
	SeniorityLevel    string
	CandidateLocation string
	SponsorsVisas     string
	JapaneseLevel     string
	EnglishLevel      string
	SalaryMin         *int
	SalaryMax         *int
	Skills            []string
}

func parseJapanDevJob(root *html.Node, item japanDevListing) (Job, bool) {
	d, ok := parseJapanDevNuxtJob(root)
	if !ok || strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Company) == "" {
		return Job{}, false
	}
	// A sitemap URL and the page's primary job must agree. Without this check a stale route that
	// starts rendering a fallback/recommended job could silently re-key the wrong vacancy.
	if d.Slug != "" && d.Slug != item.ExternalID {
		return Job{}, false
	}

	description := strings.TrimSpace(d.RawContent)
	if description == "" {
		if n := firstByClass(root, "job-detail-main-content"); n != nil {
			description = innerHTML(n)
		}
	}
	if strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	description = structuredJapanDevFacts(d) + description
	description = sanitizeHTML(description)

	applyURL := item.URL
	if validHTTPURL(d.ApplicationURL) {
		// Preserve Japan Dev's source/UTM query parameters. The destination is the employer's
		// official ATS/application host and attribution belongs to the curator that found it.
		applyURL = d.ApplicationURL
	}

	posted := parseRFC3339(firstNonEmpty(d.PublishedAt, d.UpdatedAt))
	workMode := japanDevWorkMode(d.RemoteLevel)
	job := Job{
		ExternalID:     item.ExternalID,
		URL:            applyURL,
		Title:          strings.TrimSpace(d.Title),
		Company:        strings.TrimSpace(d.Company),
		Location:       strings.TrimSpace(d.Location),
		Description:    description,
		Remote:         workMode == "remote",
		PostedAt:       posted,
		WorkMode:       workMode,
		Seniority:      japanDevSeniority(d.SeniorityLevel),
		EmploymentType: japanDevEmploymentType(d.EmploymentType),
		Countries:      japanDevCountries(d),
		SalaryMin:      d.SalaryMin,
		SalaryMax:      d.SalaryMax,
		SalaryCurrency: "JPY",
		SalaryPeriod:   "year",
		IsTechHint:     true,
	}
	// A salary is structured only when at least one bound exists. Do not manufacture JPY/year on
	// a posting that stated no compensation at all.
	if job.SalaryMin == nil && job.SalaryMax == nil {
		job.SalaryCurrency = ""
		job.SalaryPeriod = ""
	}
	return job, true
}

func parseJapanDevNuxtJob(root *html.Node) (japanDevNuxtJob, bool) {
	raw := scriptTextByID(root, "__NUXT_DATA__")
	if strings.TrimSpace(raw) == "" {
		return japanDevNuxtJob{}, false
	}
	var table []any
	if err := json.Unmarshal([]byte(raw), &table); err != nil {
		return japanDevNuxtJob{}, false
	}
	var primary map[string]any
	for _, cell := range table {
		m, ok := cell.(map[string]any)
		if !ok {
			continue
		}
		ref, exists := m["job"]
		if !exists {
			continue
		}
		candidate, ok := japanDevDeref(table, ref).(map[string]any)
		if !ok {
			continue
		}
		if _, title := candidate["title"]; title {
			if _, slug := candidate["slug"]; slug {
				primary = candidate
				break
			}
		}
	}
	if primary == nil {
		return japanDevNuxtJob{}, false
	}

	company := ""
	if cm, ok := japanDevDeref(table, primary["company"]).(map[string]any); ok {
		company = japanDevString(table, cm["name"])
	}
	return japanDevNuxtJob{
		Title:             japanDevString(table, primary["title"]),
		Slug:              japanDevString(table, primary["slug"]),
		Company:           company,
		Location:          japanDevString(table, primary["location"]),
		RawContent:        japanDevString(table, primary["raw_content"]),
		ApplicationURL:    japanDevString(table, primary["application_url"]),
		PublishedAt:       japanDevString(table, primary["published_at"]),
		UpdatedAt:         japanDevString(table, primary["updated_at"]),
		RemoteLevel:       japanDevString(table, primary["remote_level"]),
		EmploymentType:    japanDevString(table, primary["employment_type"]),
		SeniorityLevel:    japanDevString(table, primary["seniority_level"]),
		CandidateLocation: japanDevString(table, primary["candidate_location"]),
		SponsorsVisas:     japanDevString(table, primary["sponsors_visas"]),
		JapaneseLevel:     japanDevString(table, primary["japanese_level_enum"]),
		EnglishLevel:      japanDevString(table, primary["english_level_enum"]),
		SalaryMin:         japanDevInt(table, primary["salary_min"]),
		SalaryMax:         japanDevInt(table, primary["salary_max"]),
		Skills:            japanDevSkillNames(table, primary["skills"]),
	}, true
}

// Nuxt serializes its SSR state as a top-level JSON reference table. Object values and array
// members are indexes into that table; scalar table cells are the actual values. These helpers
// resolve just one hop at a time, enough for the primary job and its skill/company children.
func japanDevDeref(table []any, v any) any {
	n, ok := v.(float64)
	if !ok || n != float64(int(n)) {
		return v
	}
	i := int(n)
	if i < 0 || i >= len(table) {
		return nil
	}
	return table[i]
}

func japanDevString(table []any, v any) string {
	v = japanDevDeref(table, v)
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func japanDevInt(table []any, v any) *int {
	v = japanDevDeref(table, v)
	var n int
	switch x := v.(type) {
	case float64:
		if x <= 0 || x != float64(int(x)) {
			return nil
		}
		n = int(x)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil || parsed <= 0 {
			return nil
		}
		n = parsed
	default:
		return nil
	}
	return &n
}

func japanDevSkillNames(table []any, v any) []string {
	items, ok := japanDevDeref(table, v).([]any)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := japanDevDeref(table, item).(map[string]any)
		if !ok {
			continue
		}
		name := japanDevString(table, m["name"])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func japanDevCountries(d japanDevNuxtJob) []string {
	// A worldwide-remote posting whose structured location is literally Remote states no
	// workplace country. Every other Japan Dev posting is in the board's Japan market and either
	// names a Japanese place or explicitly scopes remote work to Japan.
	if d.RemoteLevel == "remote_level_full_worldwide" && strings.EqualFold(strings.TrimSpace(d.Location), "Remote") {
		return nil
	}
	return []string{"jp"}
}

func japanDevWorkMode(v string) string {
	switch v {
	case "remote_level_partial":
		return "hybrid"
	case "remote_level_full_japan", "remote_level_full_worldwide", "remote_level_around_office":
		return "remote"
	case "remote_level_none":
		return "onsite"
	default:
		return ""
	}
}

func japanDevEmploymentType(v string) string {
	switch v {
	case "employment_type_full_time":
		return "full_time"
	case "employment_type_part_time":
		return "part_time"
	case "employment_type_contract":
		return "contract"
	case "employment_type_internship":
		return "internship"
	default:
		return ""
	}
}

func japanDevSeniority(v string) string {
	switch v {
	case "seniority_level_junior":
		return "junior"
	case "seniority_level_mid_level":
		return "middle"
	case "seniority_level_senior":
		return "senior"
	default:
		return ""
	}
}

// structuredJapanDevFacts retains source-native eligibility/language facts that Job has no
// dedicated structured fields for. Keeping them in the description makes them searchable and
// available to downstream requirement extraction without pretending Japan Dev's labels are CEFR.
func structuredJapanDevFacts(d japanDevNuxtJob) string {
	var facts []string
	switch d.CandidateLocation {
	case "candidate_location_anywhere":
		facts = append(facts, "Applications: candidates may apply from outside Japan")
	case "candidate_location_japan_only":
		facts = append(facts, "Applications: candidates must already be in Japan")
	}
	switch d.SponsorsVisas {
	case "sponsors_visas_yes":
		facts = append(facts, "Visa sponsorship: available")
	case "sponsors_visas_no":
		facts = append(facts, "Visa sponsorship: not available")
	}
	if level := japanDevLevelLabel(d.JapaneseLevel); level != "" {
		facts = append(facts, "Japanese: "+level)
	}
	if level := japanDevLevelLabel(d.EnglishLevel); level != "" {
		facts = append(facts, "English: "+level)
	}
	if len(d.Skills) > 0 {
		facts = append(facts, "Japan Dev tags: "+strings.Join(d.Skills, ", "))
	}
	if len(facts) == 0 {
		return ""
	}
	return "<p>" + strings.Join(facts, ". ") + ".</p>"
}

func japanDevLevelLabel(v string) string {
	switch v {
	case "japanese_level_not_required", "english_level_not_required":
		return "Not required"
	case "japanese_level_conversational", "english_level_conversational":
		return "Conversational"
	case "japanese_level_business_level", "english_level_business_level":
		return "Business level"
	case "japanese_level_fluent", "english_level_fluent":
		return "Fluent"
	default:
		return ""
	}
}

// textFromHTML is only a validity check for a source body before sanitizing it. Parsing through
// the shared HTML parser avoids treating markup-only content such as <p><br></p> as a description.
func textFromHTML(markup string) string {
	root, err := html.Parse(strings.NewReader("<body>" + markup + "</body>"))
	if err != nil {
		return ""
	}
	return textContent(root)
}
