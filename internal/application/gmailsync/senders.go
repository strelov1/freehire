package gmailsync

import (
	"fmt"
	"strings"
	"time"
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
	// Mail-relay domains that differ from a platform's board domain; applicant mail arrives
	// from these, so the board-domain entries above do not cover them.
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

// JobBoardSenders are the application-mail addresses of job boards whose host also sends
// everything else (alerts, feeds, digests), so the host fails the learned-domain precision
// bar (MaxOtherShare) and is on no allowlist. Gmail's from: takes a full address as readily as a
// domain, and these addresses are nearly all hiring mail where the hosts are mostly `other`.
//
// No LinkedIn address: its InMail and "just messaged you" mail is recruiter outreach, not an
// application, and MaxOtherShare keeps linkedin.com out of the learned cache.
var JobBoardSenders = []string{
	"indeedapply@indeed.com",
	"noreply@indeed.com",
	"no-reply-message@indeed.com",
	"applyonline@dice.com",
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

	// Near-miss wordings ("we've received your … application", "interview invite"). Employers
	// phrase one idea several ways: when adding, add the SIBLINGS of a wording, not the wording.
	"received your application",
	"interview invite",
	"interview invitation",
	"next steps",
	"schedule a call",
	"приглашение на собеседование", // ru: interview invitation
	"собеседование",                // ru: interview
}

// BuildQueryFor builds a Gmail search query for job-application mail newer than the
// given Unix watermark, excluding the connected address. It ORs the hardcoded ATS sender
// core with any learned domains (extraDomains, promoted by the self-learning cache) and
// multilingual application phrases, so non-ATS-domain mail is still synced; the whole
// union is time-bounded by after:. A zero watermark omits the time clause for a first-run
// backfill.
//
// The candidate's own replies match these phrasings too, and the worker drops them before
// storing, so fetching them only costs a body retrieval and page room. An empty address adds no
// clause.
//
// The exclusion sits OUTSIDE the alternation: inside it, `-from:` would be one more thing a
// message could match rather than a condition every message must meet.
func BuildQueryFor(selfAddr string, afterUnix int64, extraDomains []string) string {
	senders := make([]string, 0, len(ATSDomains)+len(JobBoardSenders)+len(extraDomains))
	senders = append(senders, ATSDomains...)
	senders = append(senders, JobBoardSenders...)
	senders = append(senders, extraDomains...)

	clauses := []string{
		"from:(" + strings.Join(senders, " OR ") + ")",
	}
	for _, p := range recallPhrases {
		clauses = append(clauses, `"`+p+`"`)
	}

	q := "(" + strings.Join(clauses, " OR ") + ")"
	if selfAddr != "" {
		q += " -from:" + selfAddr
	}
	if afterUnix > 0 {
		q += fmt.Sprintf(" after:%d", afterUnix)
	}
	return q + " -in:sent -in:drafts"
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

// recallHiringTerms is the "this is about a job" half of the recall gate, in the languages
// the phrase list above already admits.
var recallHiringTerms = []string{
	"application", "applying", "interview", "recruiter", "recruiting", "candidate",
	`"the role"`, `"the position"`, "hiring", "вакансия", "отклик", "собеседование",
	"candidatura", "entrevista", `"processo seletivo"`,
}

// recallMeetingTerms recovers what hiring vocabulary alone demonstrably drops. filename:ics
// is the exact member: every calendar invitation carries that part whatever its subject
// language, and an invitation is the only route an interview has onto the calendar view.
var recallMeetingTerms = []string{
	"filename:ics", "invite", "invitation", "meeting", `"intro call"`,
	"напоминание", `"назначена встреча"`, "созвон",
}

// BuildRecallQuery builds the mailbox search for one application: the employer, inside the
// window, gated to job-shaped mail.
//
// The gate is a privacy boundary: the product holds only job mail, and "Apple" or "Ramp" would
// pull personal correspondence, so a message qualifies only if it names the employer AND is
// job-shaped.
//
// All three members of that gate are load-bearing: hiring vocabulary alone dropped real mail
// (calendar invitations, a thread whose subject was only the role title). Hence filename:ics
// and the role title too.
//
// The employer is spelled twice, as the catalogue writes it and with spaces closed up
// ("Blend 360" vs the sender's "Blend360").
func BuildRecallQuery(company, role string, since, until time.Time) string {
	employer := quotedForms(company)
	if employer == "" {
		return ""
	}
	gate := append([]string{}, recallHiringTerms...)
	gate = append(gate, recallMeetingTerms...)
	if r := quoteTerm(role); r != "" {
		gate = append(gate, r)
	}
	return fmt.Sprintf("after:%d before:%d (%s) (%s)",
		since.Unix(), until.Unix(), employer, strings.Join(gate, " OR "))
}

// quotedForms spells the employer the ways a sender might: as written, and de-spaced.
func quotedForms(company string) string {
	as := quoteTerm(company)
	if as == "" {
		return ""
	}
	forms := []string{as}
	if squashed := quoteTerm(strings.ReplaceAll(company, " ", "")); squashed != as && squashed != "" {
		forms = append(forms, squashed)
	}
	return strings.Join(forms, " OR ")
}

// quoteTerm renders one caller-supplied term as a single search term. Quotes are stripped, not
// escaped: they are the only characters that could end the term early and let the rest act as
// query syntax. An empty term yields "" so the caller leaves it out (empty quotes match nothing).
func quoteTerm(s string) string {
	cleaned := strings.TrimSpace(strings.ReplaceAll(s, `"`, ""))
	if cleaned == "" {
		return ""
	}
	return `"` + cleaned + `"`
}
