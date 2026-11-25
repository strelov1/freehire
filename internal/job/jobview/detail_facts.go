package jobview

import "net/url"

// Salary is a posting's pay range: either bound may be absent, never both.
type Salary struct {
	Min      *int   `json:"min,omitempty"`
	Max      *int   `json:"max,omitempty"`
	Currency string `json:"currency,omitempty"`
	Period   string `json:"period,omitempty"`
}

// companyLogoBase is the logo proxy the SPA uses (web/src/lib/logo.ts). It resolves a mark
// by name, and by the domain when one is given; a miss answers 404.
const companyLogoBase = "https://logo.freehire.me/"

// DetailFacts lifts the facts a single-posting reader expects at the top level: the
// employment type and pay range already folded into Enrichment, and the employer's site and
// logo from its company record (website as stated, host its bare domain). The logo is named
// only beside a known domain, because a name alone resolves to a namesake's mark often
// enough (see cmd/publish-logo-domains).
func (j *Job) DetailFacts(website, host string) {
	j.EmploymentType = j.Enrichment.EmploymentType
	if e := j.Enrichment; e.SalaryMin != nil || e.SalaryMax != nil {
		j.Salary = &Salary{Min: e.SalaryMin, Max: e.SalaryMax, Currency: e.SalaryCurrency, Period: e.SalaryPeriod}
	}
	if website == "" || host == "" {
		return
	}
	j.CompanyWebsite = website
	name := j.Company
	if name == "" {
		name = host
	}
	j.CompanyLogo = companyLogoBase + url.PathEscape(name) + "?domain=" + url.QueryEscape(host)
}
