package companyname

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
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
//
// Deliberately absent, each having been tried against live boards: taleo (every sampled
// career section titles itself "Job Search", and nothing else on the page names the employer),
// adp (neither the listing nor the requisition detail names the client), and workday (the
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

// oracleSiteLeads and oracleSiteMarkers are what an Oracle site name wraps around the
// employer, measured on the 613 names the first prod dry-run resolved (2026-10-24): a lead-in
// ("Working at Signature Aviation"), a marker after which nothing names the employer any more
// ("EXL Talent Acquisition Team", "Crawford Job Listings - Global", "BDO USA Campus"), or a
// careers tail ("Akamai Career Site"). A marker cuts the name at its FIRST occurrence, so what
// follows it never survives. A tail is cut only at the END: "Career" inside a name is part of
// it ("Summit Career College"), and cutting there would state a different employer — such a
// name is refused by its purpose word instead.
var (
	oracleSiteLeads   = []string{"Working at ", "Job Listings at ", "Careers at ", "Jobs at "}
	oracleSiteTails   = []string{" Careers Site", " Career Site", "CareerSite", " Careers", " Career"}
	oracleSiteMarkers = []string{
		" Talent Acquisition", " Job Openings", " Job Listings", " Staff Positions", " Employment",
		" Human Resources", " Campus", " Experienced", " Referral", " Minimal", " - Carrière",
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
	"talent": true, "talento": true, "search": true, "generic": true,
}

// oracleSiteLoneWords are names that are a real word but no employer when they stand alone —
// "Global", "Schools", "Church" — the site's audience, not its owner.
var oracleSiteLoneWords = map[string]bool{
	"global": true, "schools": true, "church": true, "executive": true, "clerk": true,
	"llp": true, "aerospace": true,
}

// ExtractOracleSiteName turns an Oracle candidate-experience site name into the employer's
// name: drop a lead-in, cut at the first marker and a careers tail, then refuse what is left if it still names the
// site's purpose, stands alone as a generic word, or is a configuration identifier (an
// underscore — "POLK_BoCC_Career Site"). Refusing costs the board nothing: it simply keeps the
// placeholder it already had.
func ExtractOracleSiteName(name string) string {
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
	low := strings.ToLower(name)
	for _, marker := range oracleSiteMarkers {
		if i := strings.Index(low, strings.ToLower(marker)); i >= 0 {
			name, low = name[:i], low[:i]
		}
	}
	name = strings.TrimRight(name, " -–—")
	for _, tail := range oracleSiteTails {
		if rest, ok := cutSuffixFold(name, tail); ok {
			name = strings.TrimRight(rest, " -–—")
			break
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
