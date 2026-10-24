package companyname

import (
	"context"
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

// oracleSiteTails are the careers tails an Oracle site name appends to the employer, longest
// first so "Career Site" is not cut as "Site".
var oracleSiteTails = []string{" Careers Site", " Career Site", " Careers", " Career"}

// oracleSiteWords mark a site named for its PURPOSE rather than its employer — "Student
// Casual", "Career Site - New Applicants", "Dallah Recruitment System". Checked after the tail
// is cut, so they only ever refuse a name with nothing but the purpose left in it.
var oracleSiteWords = map[string]bool{
	"career": true, "careers": true, "site": true, "applicant": true, "applicants": true,
	"student": true, "casual": true, "recruitment": true, "candidate": true, "experience": true,
	"external": true, "internal": true, "portal": true, "jobs": true,
}

// ExtractOracleSiteName turns an Oracle candidate-experience site name into the employer's
// name: cut a careers tail, then refuse what is left if it still names the site's purpose or
// is a configuration identifier (an underscore — "POLK_BoCC_Career Site"). Refusing costs the
// board nothing: it simply keeps the placeholder it already had.
func ExtractOracleSiteName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if strings.Contains(name, "_") {
		return ""
	}
	for _, tail := range oracleSiteTails {
		if rest, ok := cutSuffixFold(name, tail); ok {
			name = rest
			break
		}
	}
	for _, w := range words.FindAllString(name, -1) {
		if oracleSiteWords[strings.ToLower(w)] {
			return ""
		}
	}
	return strings.TrimSpace(name)
}
