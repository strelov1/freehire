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

	// Added 2026-08-02 from a measurement rather than from imagination. Over 120 days on a
	// live mailbox this list fetched 431 messages where a hiring-shaped query found 1151,
	// and the misses were near misses: an acknowledgement reading "we've received your …
	// application" (a16z) where the list knew only "your application at", and invitations
	// reading "interview invite" (micro1) where it knew only "invite you to interview".
	//
	// The lesson generalises past these six. Each entry above was one canonical wording of
	// a thing employers phrase several ways, so the list's shape — one phrasing per idea —
	// was the defect. When adding, add the SIBLINGS of a wording, not the wording.
	"received your application",
	"interview invite",
	"interview invitation",
	"next steps",
	"schedule a call",
	"приглашение на собеседование", // ru: interview invitation
	"собеседование",                // ru: interview
}

// BuildQuery builds a Gmail search query for job-application mail newer than the
// given Unix watermark. It ORs the hardcoded ATS sender core with any learned
// domains (extraDomains, promoted by the self-learning cache) and multilingual
// application phrases, so non-ATS-domain mail is still synced; the whole union is
// time-bounded by after:. A zero watermark omits the time clause for a first-run
// backfill.
func BuildQuery(afterUnix int64, extraDomains []string) string {
	return BuildQueryFor("", afterUnix, extraDomains)
}

// BuildQueryFor is BuildQuery with the connected address excluded.
//
// The candidate's own replies match these phrasings as readily as an employer's — they are
// replies to them — and fetching one buys nothing: the worker drops a message whose sender
// is the connected address before storing it. What it costs is a listed id and a full body
// retrieval each, and room in the page a widened query returns. An empty address adds no
// clause, so a caller that has none is unchanged.
//
// The exclusion sits OUTSIDE the alternation. Inside it, `-from:` would be one more thing a
// message could match rather than a condition every message must meet.
func BuildQueryFor(selfAddr string, afterUnix int64, extraDomains []string) string {
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
	if selfAddr != "" {
		q += " -from:" + selfAddr
	}
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

// recallHiringTerms is the "this is about a job" half of the recall gate, in the languages
// the phrase list above already admits.
var recallHiringTerms = []string{
	"application", "applying", "interview", "recruiter", "recruiting", "candidate",
	`"the role"`, `"the position"`, "hiring", "вакансия", "отклик", "собеседование",
	"candidatura", "entrevista", `"processo seletivo"`,
}

// recallMeetingTerms recovers what hiring vocabulary alone demonstrably drops. filename:ics
// is the exact member: every calendar invitation carries that part whatever language its
// subject uses, and an invitation is the ONLY route an interview has onto the calendar view.
var recallMeetingTerms = []string{
	"filename:ics", "invite", "invitation", "meeting", `"intro call"`,
	"напоминание", `"назначена встреча"`, "созвон",
}

// BuildRecallQuery builds the mailbox search for one application: the employer, inside the
// window, gated to job-shaped mail.
//
// The gate is not caution. Searching a mailbox for a company name reaches past the boundary
// the sync draws — the product holds only job mail, and "Apple" or "Ramp" would pull
// personal correspondence — so a message qualifies only if it names the employer AND is
// job-shaped.
//
// All three members of that gate are load-bearing, and the narrow version was measured and
// rejected. Over 20 applications, hiring vocabulary alone cut 53 candidates to 41 and eight
// of the twelve it dropped were real: both calendar invitations for one interview, and a
// live thread whose entire subject was the role. Adding filename:ics and the role title
// brought it to 47, the six remaining drops being payment slips and job-board digests.
//
// The employer is spelled twice — as the catalogue writes it and with the spaces closed up.
// The catalogue says "Blend 360"; the sender signs "Blend360". That is the same class of
// mismatch that retired the calendar title tier, which compared against a hyphenated slug
// no human ever types.
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

// quoteTerm renders one caller-supplied term as a single search term. The quotes are
// stripped rather than escaped: they are the only characters here that could end the term
// early and let the rest of the string act as query syntax, and no employer or role needs
// one. An empty term yields "" so the caller can leave it out entirely — an empty pair of
// quotes in a query is a term that matches nothing.
func quoteTerm(s string) string {
	cleaned := strings.TrimSpace(strings.ReplaceAll(s, `"`, ""))
	if cleaned == "" {
		return ""
	}
	return `"` + cleaned + `"`
}
