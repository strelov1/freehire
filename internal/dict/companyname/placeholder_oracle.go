package companyname

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
)

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
// around the employer:
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
