package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// joppy adapts Joppy (joppy.me), a Spain-only tech hiring platform with no search API: postings
// are discovered by walking the platform's own directory. joppy.directory.xml lists every
// company profile URL (and, redundantly, every open posting's own URL under it); each company's
// page is a Next.js Pages Router app embedding that company's currently open postings IN FULL —
// title, HTML body, structured skills, place/work-mode flags, required languages, and a salary
// range — in its __NEXT_DATA__ script, so no separate per-posting detail request exists or is
// needed. See design.md in openspec/changes/add-joppy-source-adapter for the live-verified shape
// and the decisions behind the field mappings below.
type joppy struct {
	http joppyHTTP
}

// joppyHTTP is the transport joppy needs: the sitemap (XMLGetter) and each company's
// server-rendered page (TextGetter, read as raw text so bracketSlice can find __NEXT_DATA__).
type joppyHTTP interface {
	XMLGetter
	TextGetter
}

// NewJoppy builds the joppy adapter over the given HTTP client.
func NewJoppy(c joppyHTTP) Source { return joppy{http: c} }

func (joppy) Provider() string { return "joppy" }

// joppy is a single global directory, so its config entry carries no board.
func (joppy) boardless() {}

// joppy aggregates postings from many companies (company per posting), so it stays in the
// source facet.
func (joppy) aggregator() {}

const (
	joppyBaseURL       = "https://www.joppy.me"
	joppySitemapURL    = joppyBaseURL + "/sitemap.directory.xml"
	joppyDetailWorkers = 8
)

// joppyNextData is the slice of a company page's __NEXT_DATA__ payload we read.
type joppyNextData struct {
	Props struct {
		PageProps struct {
			Company *joppyCompany `json:"company"`
		} `json:"pageProps"`
	} `json:"props"`
}

// joppyCompany is one company page's payload: its display name and every open posting it
// currently lists, in full.
type joppyCompany struct {
	Name string     `json:"name"`
	Slug string     `json:"slug"`
	Jobs []joppyJob `json:"jobs"`
}

// joppyJob is one posting, carried complete on its company's page — no detail request needed.
type joppyJob struct {
	UID              string          `json:"uid"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	IsSalaryPublic   bool            `json:"isSalaryPublic"`
	OnlyEuCandidates bool            `json:"onlyEuCandidates"`
	SponsorVisa      bool            `json:"sponsorVisa"`
	RelocationPack   bool            `json:"relocationPack"`
	Skills           []joppySkill    `json:"skills"`
	Place            joppyPlace      `json:"place"`
	Languages        []joppyLanguage `json:"languages"`
	SalaryMin        *int            `json:"salaryMin"`
	SalaryMax        *int            `json:"salaryMax"`
}

// joppySkill is one required skill; IsMandatory is the platform's own must-have/nice-to-have
// flag. Job.Skills has no slot for that flag, so it is folded into the description instead
// (see joppyDescriptionExtras) rather than dropped.
type joppySkill struct {
	Name        string `json:"name"`
	IsMandatory bool   `json:"isMandatory"`
}

// joppyPlace is the posting's work-arrangement statement: three independently-set booleans
// (not mutually exclusive — see joppyWorkMode) plus whatever place text/cities the posting
// states.
type joppyPlace struct {
	IsRemote bool     `json:"isRemote"`
	IsHybrid bool     `json:"isHybrid"`
	IsOffice bool     `json:"isOffice"`
	Located  string   `json:"located"`
	Cities   []string `json:"cities"`
}

// joppyLanguage is one required language and the platform's own 1-5 proficiency scale. There is
// no authoritative equivalence between that scale and freehire's CEFR-based EnglishLevel
// vocabulary (see design.md), so it is folded into the description as a plain "name (level N/5)"
// statement rather than forced into a guessed CEFR bucket.
type joppyLanguage struct {
	Name  string `json:"name"`
	Level int    `json:"level"`
}

// Fetch walks the whole Joppy directory: it reads the sitemap for every company slug, then
// fetches every company's page and returns one Job per open posting found across all of them.
// The sitemap itself failing is a board-level error; one company's page failing is isolated and
// skipped so the rest of the directory still crawls.
func (s joppy) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	doc, err := getSitemap(ctx, s.http, joppySitemapURL)
	if err != nil {
		return nil, fmt.Errorf("joppy: sitemap: %w", err)
	}
	slugs := joppyCompanySlugs(doc)

	var (
		mu   sync.Mutex
		jobs []Job
		wg   sync.WaitGroup
		sem  = make(chan struct{}, joppyDetailWorkers)
	)
	for _, slug := range slugs {
		wg.Add(1)
		sem <- struct{}{}
		go func(slug string) {
			defer wg.Done()
			defer func() { <-sem }()
			companyJobs, err := s.fetchCompany(ctx, slug)
			if err != nil {
				log.Printf("joppy: company %s: %v", slug, err)
				return
			}
			mu.Lock()
			jobs = append(jobs, companyJobs...)
			mu.Unlock()
		}(slug)
	}
	wg.Wait()
	return jobs, nil
}

// fetchCompany fetches one company's page and maps its open postings to Jobs.
func (s joppy) fetchCompany(ctx context.Context, slug string) ([]Job, error) {
	body, err := s.http.GetText(ctx, joppyBaseURL+"/companies/"+slug)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	raw, ok := bracketSlice(body, "__NEXT_DATA__", '{', '}')
	if !ok {
		return nil, fmt.Errorf("no __NEXT_DATA__")
	}
	var data joppyNextData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if data.Props.PageProps.Company == nil {
		return nil, fmt.Errorf("no company payload")
	}
	company := data.Props.PageProps.Company

	jobs := make([]Job, 0, len(company.Jobs))
	for _, j := range company.Jobs {
		if job, ok := joppyMapJob(company, j); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// joppyMapJob maps one posting onto the catalogue's Job shape, dropping it when it carries no
// stable identity (no uid or no title).
func joppyMapJob(company *joppyCompany, j joppyJob) (Job, bool) {
	uid := strings.TrimSpace(j.UID)
	title := strings.TrimSpace(j.Title)
	if uid == "" || title == "" {
		return Job{}, false
	}

	workMode := joppyWorkMode(j.Place.IsRemote, j.Place.IsHybrid, j.Place.IsOffice)
	location := joppyLocation(j.Place)

	description := sanitizeHTML(j.Description) + joppyDescriptionExtras(j)

	job := Job{
		ExternalID:  uid,
		URL:         joppyBaseURL + "/companies/" + company.Slug + "/" + uid,
		Title:       title,
		Company:     company.Name,
		Location:    location,
		Description: description,
		Remote:      workMode == "remote" || isRemote(location),
		WorkMode:    workMode,
		Skills:      skilltag.Canonicalize(joppySkillNames(j.Skills)),
	}
	if j.IsSalaryPublic && (j.SalaryMin != nil || j.SalaryMax != nil) {
		job.SalaryMin = j.SalaryMin
		job.SalaryMax = j.SalaryMax
		job.SalaryCurrency = "EUR"
		job.SalaryPeriod = "year"
	}
	return job, true
}

// joppySkillNames extracts the bare skill names, dropping the mandatory flag (canonicalized
// separately into Job.Skills; the flag itself is folded into the description text instead).
func joppySkillNames(skills []joppySkill) []string {
	names := make([]string, 0, len(skills))
	for _, sk := range skills {
		names = append(names, sk.Name)
	}
	return names
}

// joppyWorkMode derives a work mode from Joppy's three place booleans, which live inspection
// showed are NOT mutually exclusive (a genuinely hybrid posting sets all three). isHybrid is the
// platform's own explicit "hybrid" statement and always wins; when it is unset, exactly one of
// remote/office being true is unambiguous. Both set with hybrid false has no clean single-word
// reading and is left "" for the pipeline's location-text heuristic to decide, rather than
// guessing between remote and onsite for an arrangement the platform itself did not disambiguate.
func joppyWorkMode(remote, hybrid, office bool) string {
	switch {
	case hybrid:
		return "hybrid"
	case remote && !office:
		return "remote"
	case office && !remote:
		return "onsite"
	default:
		return ""
	}
}

// joppyLocation builds the free-text location from the structured cities list, falling back to
// the posting's own free-text "located" statement.
func joppyLocation(p joppyPlace) string {
	if len(p.Cities) > 0 {
		return strings.Join(p.Cities, "; ")
	}
	return strings.TrimSpace(p.Located)
}

// joppyLanguageLevelMax is the highest level Joppy's own scale is observed to use.
const joppyLanguageLevelMax = 5

// joppyDescriptionExtras renders the posting facts Job has no dedicated field for — visa
// sponsorship, a relocation package, EU-candidates-only eligibility, the must-have/nice-to-have
// skill split, and required languages — as sanitized HTML the caller can append directly to the
// (already-HTML) description, so a real platform-stated fact is never silently dropped just
// because no column exists for it. Language levels are rendered as the platform's own raw "N/5"
// figure rather than mapped onto a CEFR bucket Joppy never actually committed to (see
// design.md).
func joppyDescriptionExtras(j joppyJob) string {
	return sanitizeHTML(markdownToHTML(joppyDescriptionExtrasMarkdown(j)))
}

// joppyDescriptionExtrasMarkdown builds the extras as blank-line-separated Markdown paragraphs;
// joppyDescriptionExtras converts the result to HTML before it reaches a caller.
func joppyDescriptionExtrasMarkdown(j joppyJob) string {
	var b strings.Builder

	var mandatory, niceToHave []string
	for _, sk := range j.Skills {
		name := strings.TrimSpace(sk.Name)
		if name == "" {
			continue
		}
		if sk.IsMandatory {
			mandatory = append(mandatory, name)
		} else {
			niceToHave = append(niceToHave, name)
		}
	}
	if len(mandatory) > 0 {
		fmt.Fprintf(&b, "\n\nMust-have skills: %s.", strings.Join(mandatory, ", "))
	}
	if len(niceToHave) > 0 {
		fmt.Fprintf(&b, "\n\nNice-to-have skills: %s.", strings.Join(niceToHave, ", "))
	}

	if len(j.Languages) > 0 {
		var langs []string
		for _, l := range j.Languages {
			name := strings.TrimSpace(l.Name)
			if name == "" {
				continue
			}
			langs = append(langs, fmt.Sprintf("%s (level %d/%d)", name, l.Level, joppyLanguageLevelMax))
		}
		if len(langs) > 0 {
			fmt.Fprintf(&b, "\n\nRequired languages: %s.", strings.Join(langs, ", "))
		}
	}

	if j.SponsorVisa {
		b.WriteString("\n\nThe employer sponsors a work visa for this role.")
	}
	if j.RelocationPack {
		b.WriteString("\n\nA relocation package is offered.")
	}
	if j.OnlyEuCandidates {
		b.WriteString("\n\nOpen only to candidates already eligible to work in the EU.")
	}

	return b.String()
}

// joppyCompanySlugs extracts every distinct company slug named in the sitemap, from either a
// bare company-profile URL or a per-posting URL nested under one. Every slug is walked — not
// only the ones with a posting sub-entry — because trusting the sitemap's own job entries as a
// prefilter risks the same class of staleness bug this codebase has hit before on other
// sitemap-driven boards (see design.md); a company's own page is the ground truth.
func joppyCompanySlugs(doc sitemapDoc) []string {
	seen := make(map[string]struct{})
	const prefix = joppyBaseURL + "/companies/"
	for _, u := range doc.URLs {
		rest, ok := strings.CutPrefix(u.Loc, prefix)
		if !ok || rest == "" {
			continue
		}
		slug, _, _ := strings.Cut(rest, "/")
		if slug != "" {
			seen[slug] = struct{}{}
		}
	}
	slugs := make([]string, 0, len(seen))
	for slug := range seen {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return slugs
}
