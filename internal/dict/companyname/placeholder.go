package companyname

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
)

// NewPlaceholderRegistry wires the resolvers for a board whose company is still its own board
// id — a "<host>/<site>" path or a GUID that a harvest wrote into boards.company verbatim and
// every posting then carried as its employer's public name. Unlike NewRegistry, the input is
// the board id itself (no URL to parse, no slug to compare against), and each resolver reads
// the name the ATS states for exactly that board, so AcceptPlaceholder needs no confidence
// match — the same argument join's id lookup makes in Accept.
//
// Measured 2026-10-23 on 15 random placeholder boards per provider:
//   - paylocity (2,069 boards): the listing page titles itself "{Name} - Job Opportunities", 15/15.
//   - ukgready (774): format-settings, which the adapter already reads for the currency, carries
//     company.name, 15/15.
//   - oracle (746): the candidate-experience site's og:site_name is whatever the employer named
//     the site — usually the employer, often with a careers tail, sometimes only its purpose
//     ("Student Casual"). ExtractOracleSiteName keeps 11/15 and refuses the rest. Oracle's own
//     recruitingCESites API was tried first and is worse: "Candidate Experience site" or nothing.
//
// Measured 2026-10-24 on 15 more each:
//   - dayforce (486): the careers page's Next.js data names the tenant as
//     candidateCorrespondenceClientName, the sender of the candidate's mail, 13/15 — the others
//     named a department ("Human Resources") or nothing; ExtractDayforceClientName refuses those.
//   - ukg (1,152): the job board's header logo carries the brand as its alt text, 11/15 — the
//     others had no logo or the unconfigured "Default Brand"; ExtractUKGBrandName refuses those.
//   - taleo (226, every one swept): the career section's footer copyright names the employer on
//     83 ("© 2018 by Hyatt Corporation"), and the header logo's alt text on 59 more or the same
//     ones ("Marcus Careers") — 113 boards between them. ExtractTaleoCopyright is preferred: a
//     legal line, where the alt text is whatever a designer typed ("The black version of the
//     Stantec logo"). The section's <title> is "Job Search" on every one.
//
// Deliberately absent, each having been tried against live boards: adp (neither the listing nor the requisition detail names the client), and workday (the
// detail's hiringOrganization is a per-posting legal entity — Dover's postings name
// "Engineered Controls International, LLC" — so it would split one employer per subsidiary).
func NewPlaceholderRegistry(http httpGetter) Registry {
	return Registry{
		"paylocity": newTitleResolver(http, "https://recruiting.paylocity.com/recruiting/jobs/All/%s", func(title string) string {
			return ExtractSuffixName(title, " - Job Opportunities")
		}),
		"ukgready": ukgreadyResolver{http: http},
		"oracle":   oracleResolver{http: http},
		"dayforce": dayforceResolver{http: http},
		"ukg":      ukgResolver{http: http},
		"taleo":    taleoResolver{http: http},
	}
}

// AcceptPlaceholder decodes and validates a candidate resolved for a placeholder board. No
// text relation to the board is required (see NewPlaceholderRegistry); what is refused is a
// candidate that would be no improvement: empty, junk, or the placeholder itself.
func AcceptPlaceholder(board, candidate string) (string, bool) {
	candidate = clean(html.UnescapeString(candidate))
	if candidate == "" || strings.EqualFold(candidate, board) {
		return "", false
	}
	low := strings.ToLower(candidate)
	for _, m := range junkMarkers {
		if strings.Contains(low, m) {
			return "", false
		}
	}
	return candidate, true
}

// splitHostBoard splits a "<host>/<rest>" board id, the shape ukgready and oracle share.
func splitHostBoard(board string) (host, rest string, ok bool) {
	host, rest, ok = strings.Cut(board, "/")
	return host, rest, ok && host != "" && rest != "" && !strings.Contains(rest, "/")
}

type ukgreadyResolver struct{ http jsonGetter }

// ukgreadySettings is the subset of a tenant's format-settings this package reads.
type ukgreadySettings struct {
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
}

func (r ukgreadyResolver) Name(ctx context.Context, board string) (string, error) {
	host, tenant, ok := splitHostBoard(board)
	if !ok {
		return "", nil
	}
	var s ukgreadySettings
	url := fmt.Sprintf("https://%s/ta/rest/ui/bootstrap/companies/%%7C%s/format-settings", host, tenant)
	if err := r.http.GetJSON(ctx, url, &s); err != nil {
		return "", err
	}
	return s.Company.Name, nil
}

type oracleResolver struct{ http textGetter }

var ogSiteName = regexp.MustCompile(`(?i)<meta[^>]+property="og:site_name"[^>]+content="([^"]*)"`)

func (r oracleResolver) Name(ctx context.Context, board string) (string, error) {
	host, site, ok := splitHostBoard(board)
	if !ok {
		return "", nil
	}
	body, err := r.http.GetText(ctx, fmt.Sprintf("https://%s/hcmUI/CandidateExperience/en/sites/%s/requisitions", host, site))
	if err != nil {
		return "", err
	}
	m := ogSiteName.FindStringSubmatch(body)
	if m == nil {
		return "", nil
	}
	return ExtractOracleSiteName(html.UnescapeString(m[1])), nil
}

// oracleSiteLeads, oracleSiteMarkers and oracleSiteTails are what an Oracle site name wraps
// around the employer, measured on every site name the prod runs met (677 distinct, 2026-10-24):
// a lead-in ("Working at Signature Aviation", "Jobs @ Carmeuse"), a marker after which nothing
// names the employer any more ("EXL Talent Acquisition Team", "Mayo Career Site US", "Dollar
// General External Career Site"), or a tail ("Mountaire Jobs", "Chubb External"). A marker cuts
// the name at its FIRST occurrence, so what follows it never survives; every marker is a phrase
// no employer's own name carries. A tail is cut only at the END: a single word such as
// "Career" inside a name is part of it ("Summit Career College"), and cutting there would state
// a different employer — such a name is refused by its purpose word instead.
var (
	oracleSiteLeads = []string{
		"Working at ", "Job Listings at ", "Careers at ", "Jobs at ", "Jobs @ ", "Search Jobs - ",
	}
	oracleSiteMarkers = []string{
		" Careers Site", " Career Site", "CareerSite", " Career Portal", " Careers Portal",
		" Career Section", " Careers Section", " Career Website", " Career Connection",
		" Careers Center", " Careers(", " Opportunities", " Candidate Experience", " Candidate Site",
		" External Career", " External Site", " Talent Acquisition", " Recruitment Team",
		" Job Openings", " Job Listings", " Staff Positions", " Employment", " Human Resources",
		" Campus", " Experienced", " Referral", " Minimal", " - Carrière", " - Student", " - NEW",
		" Professional Careers", " Interns",
	}
	oracleSiteTails = []string{
		" Careers", " Career", " External", " Internal", " Jobs", " Job", " Site", " Portal",
	}
)

// oracleSiteWords mark a site named for its PURPOSE rather than its employer — "Student
// Casual", "Work with us", "Sitio de carrera Laureate". Checked after the wrapping is cut, so
// they only ever refuse a name with nothing but the purpose left in it.
var oracleSiteWords = map[string]bool{
	"career": true, "careers": true, "carrera": true, "sitio": true, "site": true, "page": true,
	"applicant": true, "applicants": true, "student": true, "casual": true, "graduate": true,
	"apprenticeship": true, "recruitment": true, "candidate": true, "experience": true,
	"external": true, "internal": true, "portal": true, "job": true, "jobs": true, "work": true,
	"talent": true, "talento": true, "search": true, "generic": true, "posting": true,
	"testing": true,
}

// oracleSiteLoneWords are names that are a real word but no employer when they stand alone —
// "Global", "Schools", "Church" — the site's audience, not its owner.
var oracleSiteLoneWords = map[string]bool{
	"global": true, "schools": true, "church": true, "executive": true, "clerk": true,
	"llp": true, "aerospace": true, "anywhere": true, "all": true, "corporate": true,
	"vision": true, "academy": true,
}

// ExtractOracleSiteName turns an Oracle candidate-experience site name into the employer's
// name. A name split by "|" is read part by part, and the parts that name an employer must
// agree: "Careers | BXP" has one, "Verisk Careers | Verisk" two that match, while "Local Truck
// Driving Jobs | Centerline Drivers" has two that differ — and so would "Careers at Acme |
// North America", where choosing either side by its shape could state a region as the
// employer. Disagreement is refused. Refusing costs the board nothing: it simply keeps the
// placeholder it already had.
func ExtractOracleSiteName(name string) string {
	found := ""
	for _, part := range strings.Split(name, "|") {
		got := extractOracleSitePart(part)
		switch {
		case got == "":
		case found == "":
			found = got
		case !strings.EqualFold(found, got):
			return ""
		}
	}
	return found
}

// extractOracleSitePart drops a lead-in, cuts at the first marker and a tail, then refuses what
// is left if it still names the site's purpose, stands alone as a generic word, or is a
// configuration identifier (an underscore — "POLK_BoCC_Career Site", "Innergex_EN").
func extractOracleSitePart(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if strings.Contains(name, "_") {
		return ""
	}
	for _, lead := range oracleSiteLeads {
		if len(name) > len(lead) && strings.EqualFold(name[:len(lead)], lead) {
			name = name[len(lead):]
			break
		}
	}
	// A lowercase article is wrapping ("the Myriad Genetics Candidate site"); a capitalised one
	// is the employer's own ("The Kroger Co.").
	name = strings.TrimPrefix(name, "the ")
	low := strings.ToLower(name)
	for _, marker := range oracleSiteMarkers {
		if i := strings.Index(low, strings.ToLower(marker)); i >= 0 {
			name, low = name[:i], low[:i]
		}
	}
	name = strings.TrimRight(name, " -–—")
	// Tails stack ("Chubb External Jobs"), so cut until none is left.
	for cut := true; cut; {
		cut = false
		for _, tail := range oracleSiteTails {
			if rest, ok := cutSuffixFold(name, tail); ok {
				name, cut = strings.TrimRight(rest, " -–—"), true
				break
			}
		}
	}
	tokens := words.FindAllString(name, -1)
	for _, w := range tokens {
		if oracleSiteWords[strings.ToLower(w)] {
			return ""
		}
	}
	if len(tokens) == 1 && oracleSiteLoneWords[strings.ToLower(tokens[0])] {
		return ""
	}
	return name
}

type dayforceResolver struct{ http textGetter }

// dayforceClientName matches the field as a JSON string, escapes included, so the value can be
// unquoted exactly rather than guessed at.
var dayforceClientName = regexp.MustCompile(`"candidateCorrespondenceClientName"\s*:\s*("(?:[^"\\]|\\.)*")`)

func (r dayforceResolver) Name(ctx context.Context, board string) (string, error) {
	if _, _, ok := splitHostBoard(board); !ok { // "<tenant>/<site>", the same two-part shape
		return "", nil
	}
	body, err := r.http.GetText(ctx, "https://jobs.dayforcehcm.com/en-US/"+board)
	if err != nil {
		return "", err
	}
	m := dayforceClientName.FindStringSubmatch(body)
	if m == nil {
		return "", nil
	}
	var name string
	if err := json.Unmarshal([]byte(m[1]), &name); err != nil {
		return "", nil
	}
	return ExtractDayforceClientName(name), nil
}

// dayforceDepartments are the sender names that name a department rather than the employer —
// what a tenant configures when its mail goes out from HR, not from the company.
var dayforceDepartments = map[string]bool{
	"human resources": true, "hr": true, "recruiting": true, "recruiting team": true,
	"recruitment": true, "talent acquisition": true, "talent acquisition team": true,
	"careers": true, "hiring team": true, "no reply": true, "noreply": true,
}

// ExtractDayforceClientName cleans a Dayforce correspondence sender name and refuses one that
// names a department instead of the employer.
func ExtractDayforceClientName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if dayforceDepartments[strings.ToLower(name)] {
		return ""
	}
	return name
}

type ukgResolver struct{ http textGetter }

// ukgLogo is the job board's header logo; its alt text is the brand the board is configured
// under. The page renders it twice (small and large), identically.
var (
	ukgLogo    = regexp.MustCompile(`<img[^>]*class="[^"]*\blogo\b[^"]*"[^>]*>`)
	ukgLogoAlt = regexp.MustCompile(`alt="([^"]*)"`)
)

func (r ukgResolver) Name(ctx context.Context, board string) (string, error) {
	parts := strings.Split(board, "/") // "<host>/<tenant>/<guid>", as the ukg adapter parses it
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", nil
	}
	body, err := r.http.GetText(ctx, fmt.Sprintf("https://%s/%s/JobBoard/%s/", parts[0], parts[1], parts[2]))
	if err != nil {
		return "", err
	}
	img := ukgLogo.FindString(body)
	if img == "" {
		return "", nil
	}
	m := ukgLogoAlt.FindStringSubmatch(img)
	if m == nil {
		return "", nil
	}
	return ExtractUKGBrandName(m[1]), nil
}

// ExtractUKGBrandName turns a UKG logo's alt text into the employer's name: the platform names
// a brand "<Name> Brand" when a tenant leaves the label as configured, and "Default Brand" when
// it configures nothing at all.
func ExtractUKGBrandName(alt string) string {
	name := strings.Join(strings.Fields(html.UnescapeString(alt)), " ")
	if rest, ok := cutSuffixFold(name, " Brand"); ok {
		name = rest
	}
	if strings.EqualFold(name, "default") || strings.EqualFold(name, "brand") {
		return ""
	}
	return name
}

type taleoResolver struct{ http textGetter }

var (
	taleoCopyrightLine = regexp.MustCompile(`(?i)(?:&copy;|&#169;|&#xa9;|©)\s*([^<"]{2,120})`)
	taleoLogo          = regexp.MustCompile(`<img[^>]*class="[^"]*logo[^"]*"[^>]*>`)
)

func (r taleoResolver) Name(ctx context.Context, board string) (string, error) {
	host, section, ok := splitHostBoard(board) // "<host>/<section>", as the taleo adapter parses it
	if !ok {
		return "", nil
	}
	body, err := r.http.GetText(ctx, fmt.Sprintf("https://%s/careersection/%s/jobsearch.ftl?lang=en", host, section))
	if err != nil {
		return "", err
	}
	// Every copyright line is read, not only the first: a page may also carry the platform's own
	// ("© Oracle"), which ExtractTaleoCopyright refuses.
	for _, m := range taleoCopyrightLine.FindAllStringSubmatch(body, -1) {
		if name := ExtractTaleoCopyright(html.UnescapeString(m[1])); name != "" {
			return name, nil
		}
	}
	if img := taleoLogo.FindString(body); img != "" {
		if m := ukgLogoAlt.FindStringSubmatch(img); m != nil {
			return ExtractTaleoLogoAlt(m[1]), nil
		}
	}
	return "", nil
}

var (
	// taleoCopyrightLead is what precedes the holder: "Copyright", a year, a comma, "by".
	taleoCopyrightLead = regexp.MustCompile(`(?i)^(?:copyright\s*)?(?:(?:19|20)\d\d\s*)?,?\s*(?:by\s+)?`)
	taleoYearTail      = regexp.MustCompile(`\s+(?:19|20)\d\d$`)
	taleoAddress       = regexp.MustCompile(`^\d+\s+\pL`)
	// taleoRightsMarkers end the holder's name; the earliest one found wins.
	taleoRightsMarkers = []string{" all right", " – all", " - all", ", po box", " is an equal opportunity"}
	// taleoAbbreviations keep their trailing period — "Moxa Inc.", "D.R. Horton, Inc." — where
	// any other final period is the end of a sentence.
	taleoAbbreviations = map[string]bool{"inc.": true, "co.": true, "corp.": true, "ltd.": true, "st.": true}
)

// ExtractTaleoCopyright pulls the copyright holder out of a Taleo career section's footer
// line: drop the "Copyright 2025, by" lead, cut at the rights statement or an address, keep the
// first part of a piped line, and refuse a line that is only a year or an address.
func ExtractTaleoCopyright(line string) string {
	s := strings.Join(strings.Fields(line), " ")
	s = taleoCopyrightLead.ReplaceAllString(s, "")
	// Searched with a leading space so a statement with no holder before it ("2025 All rights
	// reserved.") cuts to nothing rather than becoming the name.
	low, cut := " "+strings.ToLower(s), len(s)
	for _, m := range taleoRightsMarkers {
		if i := strings.Index(low, m); i >= 0 && i < cut {
			cut = max(i-1, 0)
		}
	}
	s = s[:cut]
	first := ""
	for _, part := range strings.Split(s, "|") {
		if part = strings.TrimSpace(part); part != "" {
			first = part
			break
		}
	}
	if taleoAddress.MatchString(first) {
		return "" // "2016 | 8000 York Road | Towson, MD": the footer names a place, not a holder
	}
	s = first
	// "West Virginia University. West Virginia University" — the holder, then a sentence that
	// repeats it.
	if before, after, ok := strings.Cut(s, ". "); ok && strings.HasPrefix(after, before) {
		s = before
	}
	s = taleoYearTail.ReplaceAllString(strings.TrimRight(s, " ,-–"), "")
	if f := strings.Fields(s); len(f) > 0 && strings.HasSuffix(s, ".") && !taleoAbbreviations[strings.ToLower(f[len(f)-1])] {
		s = strings.TrimSuffix(s, ".")
	}
	if !strings.ContainsFunc(s, unicode.IsLetter) {
		return ""
	}
	for _, w := range words.FindAllString(s, -1) {
		if w := strings.ToLower(w); w == "oracle" || w == "taleo" {
			return "" // the platform's copyright, not the employer's
		}
	}
	return s
}

var (
	taleoAltLeads = []string{"Careers at ", "The black version of the ", "The white version of the "}
	taleoAltTails = []string{" logo", " Taleo", " Careers", " Jobs"}
	// taleoAltWords refuse an alt text naming the platform or the page rather than the employer.
	taleoAltWords = map[string]bool{"taleo": true, "oracle": true, "staging": true, "search": true, "job": true, "jobs": true, "careers": true}
)

// ExtractTaleoLogoAlt turns a Taleo header logo's alt text into the employer's name: drop a
// lead-in, cut stacked tails, refuse what names the platform or the page.
func ExtractTaleoLogoAlt(alt string) string {
	s := strings.Join(strings.Fields(html.UnescapeString(alt)), " ")
	for _, lead := range taleoAltLeads {
		if len(s) > len(lead) && strings.EqualFold(s[:len(lead)], lead) {
			s = s[len(lead):]
			break
		}
	}
	for cut := true; cut; {
		cut = false
		for _, tail := range taleoAltTails {
			if rest, ok := cutSuffixFold(s, tail); ok {
				s, cut = rest, true
				break
			}
		}
	}
	for _, w := range words.FindAllString(s, -1) {
		if taleoAltWords[strings.ToLower(w)] {
			return ""
		}
	}
	return strings.TrimSpace(s)
}
