package sources

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/dict/vocab"
)

// profession adapts Profession.hu, Hungary's dominant job board. It is one central
// catalogue with no per-employer tenancy, so it is a multi-company aggregator whose
// employer is read from each posting.
//
// THE BOARD IS ONE OF THE PLATFORM'S OWN CATEGORY SITEMAPS, and taking the technical
// ones rather than the whole board is the design. Profession.hu publishes 23 category
// sitemaps holding ~12k postings between them; two of those categories are IT
// (itdev, itops) and hold ~770. That is the AppliTrack/EDJOIN lesson applied before it
// costs anything — freehire's non-technical gate was written against tech-company ATS
// boards and leaks badly on a general-population board, so the platform's own facet
// picks the slice. Measured over 128 live postings from the two IT sitemaps,
// classify.ConfirmedNonTech rejected zero: the slice is already what the catalogue
// wants.
//
// THE OTHER 21 CATEGORIES ARE CRAWLED THROUGH TWO GATES, because the platform's facet
// is a good answer and not a complete one: technical postings are filed outside the IT
// sections too, and an unfiltered crawl of those categories would ingest 11,388 general
// postings to find them. The gates are ordered by what they cost. The slug of a posting
// URL is already in hand from the sitemap, so professionSlugCarriesTechnicalTerm decides
// whether the platform is asked for a detail page at all; the real title on that page
// then has to satisfy professionTitleIsTechnical before the posting is stored. Both read
// one dictionary (internal/dict/classify), so the cheap gate cannot come to mean
// something the expensive one does not.
//
// Measured live on 2026-09-06 over the 21 general categories: 11,388 postings listed, 45
// passed the slug gate, 30 were stored, 15 were turned away by their real title, and no
// candidate page failed to load. That is ~99.6% of the detail requests avoided, and 30
// postings on top of the 712 the two IT sections hold — about 4.2%. Read that 30 as a
// floor rather than a measurement of what is there: the slug gate can only find terms the
// dictionary carries, and Hungarian coverage, while no longer developer-only, is not
// complete. Widening it is dictionary work and lands for every source at once.
//
// Traps, all verified live on 2026-09-03:
//
//   - jobLocationType "TELECOMMUTE" means NOT STRICTLY ONSITE, not remote. It is set on
//     48% of the slice, and the posting page shows those same postings as "Hibrid" —
//     the badge above the address is the only thing that separates hybrid from remote.
//     Worse, a posting carrying that flag has NO jobLocation at all in its ld+json, so
//     reading only the structured block loses the city on half the crawl. Both facts are
//     read off the page's address line, which agreed with the flag on 128 of 128.
//   - The address line's postal code and street number are what stop the location
//     dictionary: "Budapest" resolves, "1087 Budapest, Kerepesi út 21." resolves nothing.
//     Dropping digit runs took the sample from 61 resolved cities to 92 of 128.
//   - A taken-down posting answers 200, not 404 or 410: the URL redirects to the
//     platform's category listing, whose page carries no JobPosting block. About 8% of
//     the URLs the sitemap lists are already in that state, so a body-less read here is
//     the ordinary case rather than a failure — the posting is skipped and never stored.
//   - The ld+json description flattens the posting's bullet lists into one run-on
//     paragraph. The page carries the same text in the sections the employer typed it
//     into (#tasks, #requirements, …), list markup intact and under the employer's own
//     headings, so the body is read from those. #c_info is the employer's standing blurb
//     (the employerOverview call edjoin and gusto already make) and #contact_info is
//     where an address or a recruiter's mail lands; neither is part of the posting.
//   - The platform states no salary anywhere in this slice: data-item-variant read
//     "salary confidential" on every one of 128 sampled postings and no ld+json field
//     carries an amount. Nothing to map.
//   - occupationalCategory is a good SELECTOR and a bad LABEL, so it is read to pick the
//     board and not mapped onto a category. Its 20 IT leaves were read against the titles
//     of every one of the 709 live postings: the largest, "AI és Automatizáció" (124),
//     holds Senior Java Developer, AWS Cloud Engineer and a Vice President of Government
//     Sales, and "IT tanácsadó, Elemző, Auditor" (73) holds a Backend Engineer, an Energy
//     Economist and a Technical Writer. Even the honest leaves lose by being read: a
//     structured Category from an adapter takes precedence over the title dictionary
//     (jobderive's category precedence), and the dictionary already resolves 47% of this
//     slice — so the best map anyone could write from this vocabulary added a category to
//     132 postings while flattening 76 the dictionary had already placed more precisely
//     (backend, fullstack, frontend, mobile → software_engineering). Left to the
//     dictionary and the enrichment pass.
type profession struct {
	http professionHTTP
}

// professionHTTP is the transport profession needs: the sitemaps as XML, and each
// posting page as HTML for both its ld+json block and its rendered body sections.
type professionHTTP interface {
	XMLGetter
	HTMLGetter
}

// NewProfession builds the Profession.hu adapter over the given HTTP client.
func NewProfession(c professionHTTP) Source { return profession{http: c} }

func (profession) Provider() string { return "profession" }

// Every posting names its own employer, so profession stays in the source facet and in
// the cross-source duplicate suppression set.
func (profession) aggregator() {}

const (
	professionBaseURL = "https://www.profession.hu"
	// professionSitemapIndex lists one sitemap per category. The crawl reads it rather
	// than composing a sub-sitemap URL from the board so that a board naming a category
	// the platform no longer publishes fails as a board error instead of a 404 mid-crawl.
	professionSitemapIndex = professionBaseURL + "/sitemap-listings-index-hu.xml"
)

// professionIDPattern captures the posting id the platform ends every /allas/ URL with.
// It is the id the ld+json states as well (@id ".../JobPosting/2990578"), and it is
// stable across the slug, which carries the title and is therefore not an identity.
var professionIDPattern = regexp.MustCompile(`/allas/[^/]+-(\d+)(?:/|$)`)

// professionExternalID returns the platform's posting id for a listing URL, or "" for a
// URL that is not a posting — which is what filters the sitemap.
func professionExternalID(loc string) string {
	m := professionIDPattern.FindStringSubmatch(loc)
	if m == nil {
		return ""
	}
	return m[1]
}

// professionLDPosting selects the fields the posting page's schema.org block is read
// for. The description is deliberately absent: the block's copy has lost its list
// markup, so the body comes from the page's own sections instead.
type professionLDPosting struct {
	Title              string `json:"title"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	ExperienceReqs     string `json:"experienceRequirements"`
	EducationReqs      string `json:"educationRequirements"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address struct {
			Country string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
	ApplicantLocationRequirements []struct {
		Name string `json:"name"`
	} `json:"applicantLocationRequirements"`
}

// Fetch hydrates every listed posting. It is what a caller that cannot supply a seen set
// gets; the live pipeline always prefers FetchNew (see internal/ingest/pipeline.fetchBoard).
// There is no cheaper list-only tier to fall back on — a sitemap states nothing but a URL —
// so this is FetchNew with nothing seen rather than a second walk.
func (s profession) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew fetches a posting page only for a posting the catalogue does not already have.
// A seen posting yields a liveness refresh, which costs no request and leaves the body
// hydrated when the posting was new in place. That refresh carries no title, because the
// listing is a sitemap and states none — so a stored posting the dictionary would now turn
// away is not re-judged here. It ages out through the ordinary unseen sweep instead.
func (s profession) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	locs, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	filtered := professionFiltersBoard(e.Board)
	if filtered {
		locs = slices.DeleteFunc(locs, func(loc string) bool {
			return !professionSlugCarriesTechnicalTerm(loc)
		})
	}
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		id := professionExternalID(loc)
		if seen(id) {
			return Job{ExternalID: id, URL: loc, SeenRefresh: true}, true
		}
		job, ok := s.detail(ctx, loc, id)
		if !ok {
			return Job{}, false
		}
		// The title gate runs on new postings only, for the same reason the seen
		// refresh above carries no title: a stored posting is not re-judged here. The
		// slug gate, by contrast, runs before the seen check and so applies to both —
		// which costs nothing, because a URL's slug never changes.
		if filtered && !professionTitleIsTechnical(job.Title) {
			return Job{}, false
		}
		return job, true
	}), nil
}

// list resolves the board's category sitemap out of the index and returns every posting
// URL it holds. A category the index does not name is a board-level error: it is the
// difference between a mistyped board and an empty one, which nothing downstream could
// tell apart otherwise.
func (s profession) list(ctx context.Context, e CompanyEntry) ([]string, error) {
	sitemap, err := resolveSubSitemap(ctx, s.http, professionSitemapIndex, professionSitemapNeedle(e.Board))
	if err != nil {
		return nil, fmt.Errorf("profession: category %s: %w", e.Board, err)
	}
	if sitemap == "" {
		return nil, fmt.Errorf("profession: category %s is not in the sitemap index", e.Board)
	}
	locs, err := sitemapJobLocs(ctx, s.http, sitemap, professionExternalID)
	if err != nil {
		return nil, fmt.Errorf("profession: category %s: %w", e.Board, err)
	}
	return locs, nil
}

// professionITBoards are the platform's two dedicated IT category sitemaps. A posting
// filed in one of them is stored on the platform's own filing and is never asked to
// satisfy freehire's title dictionary as well: measured over 128 live postings from these
// two, classify.ConfirmedNonTech rejected none, and the dictionary resolves a category for
// only about half — so filtering here would drop technical postings to no purpose. Every
// other category is a general-population board and is filtered.
var professionITBoards = map[string]bool{"itdev": true, "itops": true}

// professionFiltersBoard reports whether a board's postings must clear both technical
// gates before they are stored.
func professionFiltersBoard(board string) bool {
	return !professionITBoards[strings.ToLower(strings.TrimSpace(board))]
}

// professionSlugPattern captures the slug a posting URL carries ahead of its id. The slug
// is the platform's own rendering of the title, the employer and the city, which is why it
// is not an identity (professionExternalID is) and why matching it can only ever be a
// prefilter: the employer's name is in there too.
var professionSlugPattern = regexp.MustCompile(`/allas/([^/]+)-\d+(?:/|$)`)

// professionSlugTerms are the technical title terms in the shape a URL slug states them:
// transliterated to ASCII and hyphen-separated, because the platform's slug is ASCII and
// the dictionary's Hungarian is not ("szoftverfejlesztő" is never in a URL).
//
// The vocabulary is classify's, restricted to the technical categories, rather than a
// second list kept here. That is the whole reason this stays honest — a term added for any
// source reaches this prefilter, and the gate below reads the same dictionary against the
// real title, so the cheap gate cannot drift into meaning something the expensive one does
// not. It is not a perfect shadow of that gate: classify.IsTech also admits a handful of
// generalist titles ("programmer", "founding engineer") through terms it does not export,
// so a posting whose slug states only one of those is not offered. That costs recall on a
// board we otherwise would not crawl at all, which is why it is a seam and not a bug.
var professionSlugTerms = sync.OnceValue(func() []string {
	seen := map[string]struct{}{}
	var out []string
	for category, aliases := range classify.CategoryAliases() {
		if !slices.Contains(vocab.TechCategories, category) {
			continue
		}
		for _, alias := range aliases {
			term := normalize.Slug(alias)
			if term == "" {
				continue
			}
			if _, dup := seen[term]; dup {
				continue
			}
			seen[term] = struct{}{}
			out = append(out, term)
		}
	}
	return out
})

// professionSlugCarriesTechnicalTerm reports whether a posting URL's slug states a term the
// technical dictionary knows. It is the cheap gate: a sitemap yields nothing but URLs, and
// a general category holds thousands of postings, so this decides whether the platform is
// asked for a detail page at all. Measured against the live catalogue on 2026-09-06 it let
// 45 of 11,388 non-IT postings through, avoiding about 99.6% of the detail requests taking
// those boards unfiltered would have cost.
//
// Terms match whole hyphen runs, never substrings. Hungarian is agglutinative and writes
// closed compounds, so a substring match would fire on every word that merely contains a
// term — "üzlethálózati" (a chain of shops) contains "hálózati", and the retail trainer it
// names is not a network engineer.
func professionSlugCarriesTechnicalTerm(loc string) bool {
	m := professionSlugPattern.FindStringSubmatch(loc)
	if m == nil {
		return false
	}
	padded := "-" + strings.ToLower(m[1]) + "-"
	for _, term := range professionSlugTerms() {
		if strings.Contains(padded, "-"+term+"-") {
			return true
		}
	}
	return false
}

// professionTitleIsTechnical reports whether a posting's real title states technical work.
// It is the expensive gate, run on the title the detail page's ld+json states rather than
// on the slug — the slug carries the employer's name, so "Értékesítési munkatárs" at
// "DevOps Solutions Kft." reaches this point and is turned away here. On the live audit it
// rejected 15 of the 45 postings the slug gate offered.
func professionTitleIsTechnical(title string) bool {
	return classify.IsTech(title) ||
		slices.Contains(vocab.TechCategories, classify.Parse(title).Category)
}

// professionSitemapNeedle is the file-name fragment a category's sitemap ends with. It
// carries both delimiters so a short board id cannot match a longer category's name.
func professionSitemapNeedle(board string) string {
	return "-" + strings.ToLower(strings.TrimSpace(board)) + "-hu.xml"
}

// detail fetches one posting page and maps it. It reports ok=false — so the caller skips
// just this posting — when the page carries no JobPosting block, which is what a
// taken-down posting looks like (its URL redirects to a category listing and answers
// 200), and when the page yields no body. Deferring a posting by one crawl is
// recoverable; storing it body-less is not, because a stored row is `seen` and so is
// never hydrated again once it ages past the pipeline's hydration-retry window.
func (s profession) detail(ctx context.Context, loc, externalID string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var ld professionLDPosting
	if !LDJobPosting(root, &ld) {
		return Job{}, false
	}
	description := sanitizeHTML(professionBody(root))
	title := strings.TrimSpace(ld.Title)
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	if description == "" || title == "" || company == "" {
		return Job{}, false
	}
	workMode, place := professionPlace(professionAddressLine(root))
	education, english := professionEducationAndLanguage(ld.EducationReqs)
	return Job{
		ExternalID:         externalID,
		URL:                loc,
		Title:              title,
		Company:            company,
		Location:           place,
		Description:        description,
		Remote:             workMode == "remote",
		WorkMode:           workMode,
		Countries:          professionCountries(ld),
		EmploymentType:     professionEmploymentType(ld.EmploymentType),
		ExperienceYearsMin: professionExperienceYears(ld.ExperienceReqs),
		EducationLevel:     education,
		EnglishLevel:       english,
		PostedAt:           parseDate(ld.DatePosted),
	}, true
}

// professionBodySections are the page's body sections, in the order the platform renders
// them. They are selected by element id rather than by heading, because the heading is
// the employer's own words — the same #tasks section is headed "Feladatok", "A pozíció
// leírása", "Responsibilities" and "MI LESZ A FELADATOD?" across the live sample — so it
// identifies nothing and is instead carried through as part of what the employer wrote.
//
// #c_info and #contact_info are absent on purpose. The first is the employer's standing
// blurb, repeated verbatim across its postings (the employerOverview call edjoin and
// gusto make); the second is where a postal address or a recruiter's mail lands.
var professionBodySections = []string{"tasks", "requirements", "other", "offer", "workplace_extras"}

// professionBody assembles the posting's body from the page's own sections, each under
// the heading its box states. The ld+json's description holds the same text with its list
// markup flattened into one paragraph, which is why it is not read.
func professionBody(root *html.Node) string {
	var b strings.Builder
	for _, id := range professionBodySections {
		section := firstByID(root, id)
		if section == nil {
			continue
		}
		body := strings.TrimSpace(innerHTML(section))
		if body == "" {
			continue
		}
		if heading := professionSectionHeading(section); heading != "" {
			fmt.Fprintf(&b, "<h3>%s</h3>", html.EscapeString(heading))
		}
		b.WriteString(body)
	}
	return b.String()
}

// professionSectionHeading returns the heading the section's box container states, or ""
// when the page renders none. The heading is a sibling of the section rather than a
// child, so it is reached by walking up to the box and back down to its <h2>.
func professionSectionHeading(section *html.Node) string {
	for n := section.Parent; n != nil; n = n.Parent {
		if n.Type != html.ElementNode || !hasClass(n, "box-container") {
			continue
		}
		if h2 := firstByTag(n, "h2"); h2 != nil {
			return strings.Join(strings.Fields(textContent(h2)), " ")
		}
		return ""
	}
	return ""
}

// professionWorkModeBadges maps the badge the page prints ahead of the address onto the
// work arrangement. It is the platform's own three-valued control and the only thing that
// separates a hybrid posting from a remote one — the ld+json spends a single TELECOMMUTE
// flag on both. "Országos lefedettség" ("nationwide coverage") is not a work mode: it
// says the work is spread over the whole country, which names no arrangement and no
// place, so it resolves to neither.
var professionWorkModeBadges = map[string]string{
	"Hibrid":               "hybrid",
	"Távmunka / Remote":    "remote",
	"Országos lefedettség": "",
}

// professionDigits matches a run of digits: a postal code or a street number.
var professionDigits = regexp.MustCompile(`\d+`)

// professionPlace splits the page's address line into the work mode its badge states and
// the place the location dictionary can read. A line with no badge is an ordinary sited
// posting, which the platform says by printing the address alone.
//
// The digits go because they are what stops the dictionary: it resolves "Budapest" and
// nothing at all in "1087 Budapest, Kerepesi út 21.", since the postal code is glued to
// the city inside one comma-token. Dropping them took a 128-posting sample from 61
// resolved cities to 92. The street name is left in place — it resolves to nothing, and
// removing it would mean guessing which token is the city.
func professionPlace(addressLine string) (workMode, place string) {
	line := strings.Join(strings.Fields(addressLine), " ")
	badge, rest, separated := strings.Cut(line, "•")
	badge, rest = strings.TrimSpace(badge), strings.TrimSpace(rest)
	mode, isBadge := professionWorkModeBadges[badge]
	switch {
	case isBadge && separated:
		return mode, professionStripDigits(rest)
	case isBadge:
		return mode, ""
	}
	return "onsite", professionStripDigits(line)
}

// professionStripDigits removes digit runs and tidies the whitespace they leave behind.
func professionStripDigits(place string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(professionDigits.ReplaceAllString(place, " ")), " "))
}

// professionCountries reads the country the platform states, which it does in one of two
// mutually exclusive places: a sited posting carries jobLocation.address.addressCountry
// ("HU"), a not-strictly-onsite one carries applicantLocationRequirements instead, naming
// the country in Hungarian or in English ("Magyarország", "Hungary"). Both go through the
// shared dictionary, so a name it does not know yields nothing rather than a guess — which
// is what a posting sited abroad ("Külföld") gets.
func professionCountries(ld professionLDPosting) []string {
	if code := location.NormalizeCountry(ld.JobLocation.Address.Country); code != "" {
		return []string{code}
	}
	var out []string
	for _, req := range ld.ApplicantLocationRequirements {
		if code := location.NormalizeCountry(req.Name); code != "" && !slices.Contains(out, code) {
			out = append(out, code)
		}
	}
	return out
}

// professionExperienceLevels maps the platform's experience picklist onto the low end of
// the band it names. It is a closed list — six values across the whole IT slice — and the
// two that name no band are the two that mean none: "Nem kell tapasztalat" ("no
// experience needed") and "Pályakezdő/friss diplomás" ("entry level / new graduate").
var professionExperienceLevels = map[string]int{
	"Nem kell tapasztalat":      0,
	"Pályakezdő/friss diplomás": 0,
	"1-3 év tapasztalat":        1,
	"3-5 év tapasztalat":        3,
	"5-10 év tapasztalat":       5,
	">10 év tapasztalat":        10,
}

// professionExperienceYears resolves the picklist, nil for a value outside it.
func professionExperienceYears(level string) *int {
	years, ok := professionExperienceLevels[strings.TrimSpace(level)]
	if !ok {
		return nil
	}
	return &years
}

// professionEmploymentType maps the platform's employment picklist onto freehire's
// vocabulary. The field is multi-valued and mixes two different things: a SCHEDULE
// ("Teljes munkaidő" / "Részmunkaidő") and a CONTRACT FORM ("Alkalmazotti jogviszony",
// employee status, which 81% of the slice states and which names no schedule at all —
// the ukgready employee_type trap in a second language). Only the schedule is mapped, and
// a posting ticking both schedules names neither.
func professionEmploymentType(field string) string {
	var full, part, internship bool
	for _, ticked := range strings.Split(field, ",") {
		switch strings.TrimSpace(ticked) {
		case "Teljes munkaidő":
			full = true
		case "Részmunkaidő":
			part = true
		case "Diákmunka":
			internship = true
		}
	}
	switch {
	case full && part:
		return ""
	case full:
		return "full_time"
	case part:
		return "part_time"
	case internship:
		return "internship"
	}
	return ""
}

// professionAddressLine returns the text of the page's address line: the work-mode badge,
// the separator the platform prints between them, and the place. It is one line rather
// than two fields on the page, and the badge sits OUTSIDE the schema.org jobLocation span
// beside it — which is the whole reason the line is read rather than the microdata.
func professionAddressLine(root *html.Node) string {
	n := firstByClass(root, "address-data")
	if n == nil {
		return ""
	}
	return textContent(n)
}

// professionSchoolLevels maps the platform's school picklist onto freehire's education
// vocabulary. The Hungarian pair is the load-bearing part: főiskola (a three-to-four year
// college programme) is the bachelor's-equivalent and egyetem (the five-year university
// programme) the master's-equivalent — the distinction the picklist preserves from before
// Bologna. Everything below them is the platform stating that no degree is required.
//
// "Felsőoktatási szakképzés" is deliberately absent. It is a two-year tertiary programme,
// above secondary school and below a bachelor's, and freehire's four-value vocabulary has
// no member for it — so it yields nothing rather than rounding to a neighbour, on the
// jobfacts doctrine that a wrong value in a faceted field is worse than a missing one. It
// is 28 of the 709 live postings.
var professionSchoolLevels = map[string]string{
	"Általános iskola":              "none",
	"Szakiskola / szakmunkás képző": "none",
	"Középiskola":                   "none",
	"Főiskola":                      "bachelor",
	"Egyetem":                       "master",
}

// professionEnglishLevels maps the platform's language picklist onto CEFR, for English.
//
// THE LEVELS ARE NOT "basic/medium/advanced". alapfok/középfok/felsőfok are Hungary's
// state-recognised language-exam levels and are defined as B1/B2/C1 (Government Decree
// 137/2008; the same equivalence every accredited exam centre publishes). Reading them by
// their everyday sense puts each posting a level off — and the trio has no name for C2 at
// all, which is why "felsőfok" is so often rendered as one.
var professionEnglishLevels = map[string]string{
	"Angol alapfok":          "b1",
	"Angol középfok":         "b2",
	"Angol felsőfok":         "c1",
	"Angol anyanyelvi szint": "native",
}

// professionNoLanguageRequired is the platform's own way of stating that the posting asks
// for no language at all — which is a statement about English, unlike naming some other
// language and staying silent about it.
const professionNoLanguageRequired = "Nem kell nyelvtudás"

// professionEducationAndLanguage reads the educationRequirements picklist, which
// comma-joins two different things: zero or more LANGUAGE levels and one SCHOOL level
// ("Angol középfok, Német középfok, Egyetem"). Each half is resolved independently and a
// token outside the closed vocabulary — read off all 709 live postings — yields nothing,
// so a change on the platform's side reads as silence rather than as a wrong facet.
//
// Only English answers the English facet. A posting requiring German and saying nothing
// about English states no English level; a posting requiring no language at all states
// "none".
func professionEducationAndLanguage(field string) (education, english string) {
	for _, token := range strings.Split(field, ",") {
		token = strings.TrimSpace(token)
		switch {
		case token == professionNoLanguageRequired:
			english = "none"
		case professionEnglishLevels[token] != "":
			english = professionEnglishLevels[token]
		case professionSchoolLevels[token] != "":
			education = professionSchoolLevels[token]
		}
	}
	return education, english
}
