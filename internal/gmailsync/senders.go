package gmailsync

import (
	"fmt"
	"strings"
)

// ATSDomains is the curated set of ATS sender domains, mirroring the source
// registry's spirit: adding a platform is one line here. It scopes both the
// Gmail search query and IsATSSender.
var ATSDomains = []string{
	"greenhouse-mail.io",
	"greenhouse.io",
	"lever.co", // covers hire.lever.co
	"ashbyhq.com",
	"myworkday.com",
	"smartrecruiters.com",
	"jobvite.com",
	"icims.com",
	"teamtailor.com",
	"recruitee.com",
	"bamboohr.com",
	"wellfound.com",
	"workable.com",
	"applytojob.com",
	"getmatch.ru",
	"successfactors.com",
	"taleo.net",
	"eightfold.ai",
	"gem.com",
	"rippling.com",
}

// BuildQuery builds a Gmail search query for ATS mail newer than the given Unix
// watermark. A zero watermark omits the time clause for a first-run backfill.
func BuildQuery(afterUnix int64) string {
	q := "from:(" + strings.Join(ATSDomains, " OR ") + ")"
	if afterUnix > 0 {
		q += fmt.Sprintf(" after:%d", afterUnix)
	}
	return q
}

// IsATSSender reports whether a From header (address or "Name <addr>") comes from
// a known ATS domain, matching the host exactly or as a subdomain.
func IsATSSender(from string) bool {
	host := hostOf(from)
	if host == "" {
		return false
	}
	for _, d := range ATSDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func hostOf(from string) string {
	s := from
	if i := strings.LastIndexByte(s, '<'); i >= 0 {
		s = s[i+1:]
		if j := strings.IndexByte(s, '>'); j >= 0 {
			s = s[:j]
		}
	}
	s = strings.TrimSpace(s)
	at := strings.LastIndexByte(s, '@')
	if at < 0 || at == len(s)-1 {
		return ""
	}
	return strings.ToLower(s[at+1:])
}
