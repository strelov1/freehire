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
	// Mail-relay domains that differ from a platform's board domain — applicant
	// mail arrives from these, so they must be listed to be synced (observed in a
	// real inbox; the board-domain entries above do not cover them).
	"workablemail.com",         // Workable (board domain: workable.com)
	"teamtailor-mail.com",      // Teamtailor (board domain: teamtailor.com)
	"comeet-notifications.com", // Comeet
	"recruitee-mailbox.com",    // Recruitee (also uses recruitee.com)
	"freshteam.com",            // Freshteam
	"gupy.com.br",              // Gupy
	"talentlyft.com",           // TalentLyft
	"join.com",                 // Join
	"icims.eu",                 // iCIMS EU (also icims.com)
	"successfactors.eu",        // SuccessFactors EU (also successfactors.com)
	"m.personio.com",           // Personio applicant relay (subdomain: avoid product mail)
	"mail.paylocity.com",       // Paylocity applicant relay (subdomain: avoid product mail)
}

// Only vendor/protocol-level signals belong hardcoded here — they generalise to
// any job seeker. Niche one-off ATS domains observed in a single inbox are NOT
// hardcoded; they enter the query via the self-learning cache (see learn.go), so
// the allowlist grows from real classifications instead of curation-by-anecdote.

// recallPhrases are strong application/interview phrases matched by Gmail
// full-text, catching job mail from senders on no allowlist (direct company
// domains, personal recruiters) that the ATS sender allowlist alone misses.
// Multilingual for a multilingual inbox. Everything the query pulls is
// LLM-classified downstream, so the query is recall-first; precision is the
// classifier's job, not the gate's.
var recallPhrases = []string{
	"thank you for applying",
	"thanks for applying",
	"your application at",
	"we regret to inform",
	"invite you to interview",
	"complete your interview",
	"recebemos sua candidatura",  // pt: we received your application
	"convite para entrevista",    // pt: interview invitation
	"ваш отклик",                 // ru: your application
	"приглашаем вас",             // ru: we invite you
	"hemos recibido tu",          // es: we have received your
	"invitación a la entrevista", // es: interview invitation
}

// BuildQuery builds a Gmail search query for job-application mail newer than the
// given Unix watermark. It ORs the hardcoded ATS sender core with any learned
// domains (extraDomains, promoted by the self-learning cache) and multilingual
// application phrases, so non-ATS-domain mail is still synced; the whole union is
// time-bounded by after:. A zero watermark omits the time clause for a first-run
// backfill.
func BuildQuery(afterUnix int64, extraDomains []string) string {
	senders := make([]string, 0, len(ATSDomains)+len(extraDomains))
	senders = append(senders, ATSDomains...)
	senders = append(senders, extraDomains...)

	clauses := []string{
		"from:(" + strings.Join(senders, " OR ") + ")",
	}
	for _, p := range recallPhrases {
		clauses = append(clauses, `"`+p+`"`)
	}

	q := "(" + strings.Join(clauses, " OR ") + ")"
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
