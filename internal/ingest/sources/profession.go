package sources

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
)

// profession adapts Profession.hu, Hungary's dominant job board. It is one central
// catalogue with no per-tenant board, so it is a multi-company aggregator whose employer is read
// from each posting.
//
// Only the platform's own two IT category sitemaps (itdev, itops) are crawled, not the whole board.
// freehire's non-technical gate was written against tech-company ATS boards and leaks badly on a
// general-population board, so the platform's own facet picks the slice. The other 21 categories
// are refused: they hold some technical postings, but nothing in a title separates them from their
// neighbours, and what classify.IsTech lets through there is mostly non-tech roles (bare "analyst",
// project management, product, design, "Sales Engineer"). vocab.TechCategories names the job
// families of a tech company, the wrong reading on a general board. Making the gate work would need
// a craft-only predicate, an exclusion for bare "analyst"/"qa"/"architect" and Hungarian aliases,
// which the few technical postings there do not justify. See AGENTS.md for the table.

// Traps:
//
//   - jobLocationType "TELECOMMUTE" means NOT STRICTLY ONSITE, not remote. The posting page shows
//     those same postings as "Hibrid"; the badge above the address is the only thing that separates
//     hybrid from remote. A posting carrying that flag also has NO jobLocation in its ld+json, so
//     both facts are read off the page's address line.
//   - The address line's postal code and street number stop the location dictionary: "Budapest"
//     resolves, "1087 Budapest, Kerepesi út 21." resolves nothing. Digit runs are dropped.
//   - A taken-down posting answers 200, not 404 or 410: the URL redirects to the platform's category
//     listing, whose page carries no JobPosting block. A body-less read is therefore the ordinary
//     case; the posting is skipped and never stored.
//   - The ld+json description flattens the bullet lists into one run-on paragraph. The page carries
//     the same text in the sections the employer typed it into (#tasks, #requirements, ...), list
//     markup intact, so the body is read from those. #c_info is the employer's standing blurb and
//     #contact_info is where an address or a recruiter's mail lands; neither is part of the posting.
//   - The platform states no salary in this slice (data-item-variant reads "salary confidential"
//     and no ld+json field carries an amount). Nothing to map.
//   - occupationalCategory is a good SELECTOR and a bad LABEL: it picks the board and is not mapped
//     onto a category. Its IT leaves do not match the titles under them, and a structured Category
//     from an adapter takes precedence over the title dictionary (jobderive's category precedence),
//     so mapping it would flatten categories the dictionary already places more precisely. Left to
//     the dictionary and the enrichment pass.
type profession struct {
	http professionHTTP
	// index memoizes the sitemap index for the life of the adapter, which is one crawl: the
	// pipeline shares an adapter across every board it visits. See professionIndex.
	index *professionIndex
}

// professionIndex holds the platform's sitemap index once it has been read, so a crawl resolves
// it once however many categories it visits.
//
// resolveSubSitemap re-reads the index on every call; with all 23 categories that is 23 requests
// for one document, and the platform answers the first and closes the connection on the rest,
// which reads downstream as every category being unreachable.
//
// Only a successful read is cached, so one transient failure is not spent on every remaining board.
// The lock is held across the fetch on purpose: concurrent boards wait for the one request instead
// of racing to make their own.
type professionIndex struct {
	mu       sync.Mutex
	sitemaps map[string]string
}

// get returns each category id to the URL of its sitemap.
func (i *professionIndex) get(ctx context.Context, c XMLGetter) (map[string]string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.sitemaps != nil {
		return i.sitemaps, nil
	}
	sitemaps, err := ProfessionCategorySitemaps(ctx, c)
	if err != nil {
		return nil, err
	}
	i.sitemaps = sitemaps
	return sitemaps, nil
}

// professionHTTP is the transport profession needs: the sitemaps as XML, and each
// posting page as HTML for both its ld+json block and its rendered body sections.
type professionHTTP interface {
	XMLGetter
	HTMLGetter
}

// NewProfession builds the Profession.hu adapter over the given HTTP client.
func NewProfession(c professionHTTP) Source {
	return profession{http: c, index: &professionIndex{}}
}

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
// It is the id the ld+json states as well (@id ".../JobPosting/2990578"), and it is stable
// across the slug, which carries the title and is therefore not an identity.
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

// Fetch hydrates every listed posting, for a caller that cannot supply a seen set; the live
// pipeline prefers FetchNew. A sitemap states nothing but a URL, so there is no cheaper list-only
// tier: this is FetchNew with nothing seen.
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
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		id := professionExternalID(loc)
		if seen(id) {
			return Job{ExternalID: id, URL: loc, SeenRefresh: true}, true
		}
		return s.detail(ctx, loc, id)
	}), nil
}

// list resolves the board's category sitemap out of the index and returns every posting
// URL it holds. A category the index does not name is a board-level error: it is the
// difference between a mistyped board and an empty one, which nothing downstream could
// tell apart otherwise.
func (s profession) list(ctx context.Context, e CompanyEntry) ([]string, error) {
	// Refused before the index is even read: which categories the catalogue takes is a
	// fact about the catalogue, not something the platform has to be asked about.
	if !ProfessionCrawlsCategory(e.Board) {
		return nil, fmt.Errorf("profession: category %s is not one of the platform's IT sections, "+
			"and the general-population categories are deliberately not crawled (see profession.go)", e.Board)
	}
	sitemaps, err := s.index.get(ctx, s.http)
	if err != nil {
		return nil, fmt.Errorf("profession: category %s: %w", e.Board, err)
	}
	sitemap, ok := sitemaps[strings.ToLower(strings.TrimSpace(e.Board))]
	if !ok {
		return nil, fmt.Errorf("profession: category %s is not in the sitemap index", e.Board)
	}
	locs, err := sitemapJobLocs(ctx, s.http, sitemap, professionExternalID)
	if err != nil {
		return nil, fmt.Errorf("profession: category %s: %w", e.Board, err)
	}
	return locs, nil
}

// professionCategoryPattern captures the category id out of a sitemap file name. The
// index also lists the platform's company and article sitemaps, which name no category
// and must not become boards.
var professionCategoryPattern = regexp.MustCompile(`/sitemap-listings-([a-z0-9-]+)-hu\.xml$`)

// ProfessionCategorySitemaps reads the platform's sitemap index (ONE request) and returns every
// category it publishes against the URL of that category's sitemap. It is exported for
// cmd/harvest-boards: the index IS the catalogue of categories, so a list kept elsewhere would
// drift. It returns the URLs so a caller reading those sitemaps does not re-resolve the index per
// category, which the platform's rate limiting requires (see professionIndex). The URL shape lives
// in this file only.
func ProfessionCategorySitemaps(ctx context.Context, c XMLGetter) (map[string]string, error) {
	doc, err := getSitemap(ctx, c, professionSitemapIndex)
	if err != nil {
		return nil, fmt.Errorf("profession: sitemap index: %w", err)
	}
	out := map[string]string{}
	for _, s := range doc.Sitemaps {
		loc := strings.TrimSpace(s.Loc)
		if m := professionCategoryPattern.FindStringSubmatch(loc); m != nil {
			out[m[1]] = loc
		}
	}
	return out, nil
}

// ProfessionSitemapPostings returns how many postings a category sitemap lists: the cheap
// liveness measure a board harvest needs. It takes the sitemap URL so the index is not re-read.
func ProfessionSitemapPostings(ctx context.Context, c XMLGetter, sitemapURL string) (int, error) {
	locs, err := sitemapJobLocs(ctx, c, sitemapURL, professionExternalID)
	if err != nil {
		return 0, err
	}
	return len(locs), nil
}

// professionITBoards are the platform's two dedicated IT category sitemaps, and the only
// categories the catalogue crawls. A posting filed in one of them is stored on the platform's own
// filing and is never asked to satisfy freehire's title dictionary, which resolves a category for
// only about half of them. Every other category is a general-population board and is filtered.
var professionITBoards = map[string]bool{"itdev": true, "itops": true}

// ProfessionCrawlsCategory reports whether the catalogue crawls a category at all. Exported for
// cmd/harvest-boards, so the harvest cannot offer a board the crawl would refuse.
func ProfessionCrawlsCategory(board string) bool {
	return professionITBoards[strings.ToLower(strings.TrimSpace(board))]
}

// ProfessionITBoardNames returns the platform's dedicated IT board names, sorted, for a caller
// that must enumerate them: cmd/backfill-profession-it-tech scopes its SQL predicate to exactly
// these boards, so a third IT board reaches it with no second edit.
func ProfessionITBoardNames() []string {
	names := make([]string, 0, len(professionITBoards))
	for board := range professionITBoards {
		names = append(names, board)
	}
	slices.Sort(names)
	return names
}

// ProfessionConfirmsTech reports whether a STORED (source, external_id) pair belongs to one of
// Profession's dedicated IT boards: the "confirmed technical" fact detail() asserts fresh off the
// crawl, recomputed for a caller that only has the persisted row. cmd/backfill-derive is that
// caller; without this it would rewrite these rows' is_tech back to unknown on every pass. The
// board is recovered from the external_id's namespace prefix (internal/platform/externalid.Namespace);
// the source check guards against another provider's external_id sharing a board name.
func ProfessionConfirmsTech(source, externalID string) bool {
	if source != "profession" {
		return false
	}
	board, _, ok := strings.Cut(externalID, ":")
	return ok && ProfessionCrawlsCategory(board)
}

// detail fetches one posting page and maps it. It reports ok=false, so the caller skips just
// this posting, when the page carries no JobPosting block (a taken-down posting) or yields no body.
// Deferring a posting by one crawl is recoverable; storing it body-less is not, because a stored
// row is `seen` and is never hydrated again once past the pipeline's hydration-retry window.
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
		// detail is reachable only after list() refused every board but itdev/itops, so every posting
		// here is filed under the platform's IT sections. That evidence (see professionITBoards) is
		// asserted for is_tech too, so these postings enter the enrichment queue instead of stalling when
		// the title dictionary cannot resolve a category.
		IsTechHint: true,
	}, true
}

// professionBodySections are the page's body sections, in the order the platform renders them.
// They are selected by element id rather than by heading, because the heading is the employer's
// own words ("Feladatok", "A pozíció leírása", "Responsibilities", ...). #c_info and #contact_info
// are absent on purpose: the first is the employer's standing blurb repeated across its postings,
// the second is where a postal address or a recruiter's mail lands.
var professionBodySections = []string{"tasks", "requirements", "other", "offer", "workplace_extras"}

// professionBody assembles the posting's body from the page's own sections, each under the
// heading its box states. The ld+json description has its list markup flattened, so it is not read.
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
		if n.Type != html.ElementNode || !HasClass(n, "box-container") {
			continue
		}
		if h2 := firstByTag(n, "h2"); h2 != nil {
			return strings.Join(strings.Fields(textContent(h2)), " ")
		}
		return ""
	}
	return ""
}

// professionWorkModeBadges maps the badge the page prints ahead of the address onto the work
// arrangement. It is the only thing that separates a hybrid posting from a remote one; the ld+json
// spends a single TELECOMMUTE flag on both. "Országos lefedettség" ("nationwide coverage") names
// no arrangement and no place, so it resolves to neither.
var professionWorkModeBadges = map[string]string{
	"Hibrid":               "hybrid",
	"Távmunka / Remote":    "remote",
	"Országos lefedettség": "",
}

// professionDigits matches a run of digits: a postal code or a street number.
var professionDigits = regexp.MustCompile(`\d+`)

// professionPlace splits the page's address line into the work mode its badge states and the
// place the location dictionary can read. A line with no badge is an ordinary sited posting.
//
// The digits go because the postal code glued to the city inside one comma-token stops the
// dictionary: it resolves "Budapest" and nothing in "1087 Budapest, Kerepesi út 21.". The street
// name is left in place, since removing it would mean guessing which token is the city.
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

// professionCountries reads the country the platform states, in one of two mutually exclusive
// places: a sited posting carries jobLocation.address.addressCountry ("HU"), a not-strictly-onsite
// one carries applicantLocationRequirements, naming the country in Hungarian or English. Both go
// through the shared dictionary, so an unknown name (a posting sited abroad, "Külföld") yields
// nothing rather than a guess.
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

// professionExperienceLevels maps the platform's closed experience picklist onto the low end of
// the band it names. The two values that name no band mean none: "Nem kell tapasztalat" and
// "Pályakezdő/friss diplomás".
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

// professionEmploymentType maps the platform's employment picklist onto freehire's vocabulary.
// The field is multi-valued and mixes a SCHEDULE ("Teljes munkaidő" / "Részmunkaidő") with a
// CONTRACT FORM ("Alkalmazotti jogviszony", employee status, which names no schedule). Only the
// schedule is mapped, and a posting ticking both schedules names neither.
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

// professionAddressLine returns the text of the page's address line: the work-mode badge, the
// separator and the place. It is read as one line because the badge sits OUTSIDE the schema.org
// jobLocation span beside it.
func professionAddressLine(root *html.Node) string {
	n := firstByClass(root, "address-data")
	if n == nil {
		return ""
	}
	return textContent(n)
}

// professionSchoolLevels maps the platform's school picklist onto freehire's education
// vocabulary. főiskola (three-to-four year college) is the bachelor's-equivalent and egyetem
// (five-year university) the master's-equivalent, a distinction kept from before Bologna.
// Everything below them states that no degree is required.
//
// "Felsőoktatási szakképzés" is deliberately absent: a two-year tertiary programme with no member
// in freehire's four-value vocabulary yields nothing rather than rounding to a neighbour (a wrong
// value in a faceted field is worse than a missing one).
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
// state-recognised language-exam levels, defined as B1/B2/C1 (Government Decree 137/2008). Reading
// them by their everyday sense puts each posting a level off, and the trio has no name for C2.
var professionEnglishLevels = map[string]string{
	"Angol alapfok":          "b1",
	"Angol középfok":         "b2",
	"Angol felsőfok":         "c1",
	"Angol anyanyelvi szint": "native",
}

// professionNoLanguageRequired is the platform's own way of stating that the posting asks for
// no language at all, which is a statement about English.
const professionNoLanguageRequired = "Nem kell nyelvtudás"

// professionEducationAndLanguage reads the educationRequirements picklist, which comma-joins
// zero or more LANGUAGE levels and one SCHOOL level ("Angol középfok, Német középfok, Egyetem").
// Each half is resolved independently and a token outside the closed vocabulary yields nothing,
// so a platform-side change reads as silence rather than a wrong facet.
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
