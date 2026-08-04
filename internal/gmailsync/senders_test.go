package gmailsync

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIsATSSender(t *testing.T) {
	yes := []string{
		"no-reply@us.greenhouse-mail.io",
		"Sardine <no-reply@ashbyhq.com>",
		"Oowlish <no-reply@hire.lever.co>", // subdomain of lever.co
		"web@myworkday.com",
	}
	for _, s := range yes {
		if !IsATSSender(s) {
			t.Errorf("IsATSSender(%q) = false, want true", s)
		}
	}
	no := []string{"friend@gmail.com", "news@substack.com", "", "not-an-address"}
	for _, s := range no {
		if IsATSSender(s) {
			t.Errorf("IsATSSender(%q) = true, want false", s)
		}
	}
}

// TestIsATSSender_RelayDomains locks in the ATS mail-relay domains observed in a
// real inbox that the original allow-list missed — the mail domains differ from
// the platforms' board domains (e.g. Workable sends from workablemail.com, not
// workable.com), so applicant mail from them was never synced.
func TestIsATSSender_RelayDomains(t *testing.T) {
	yes := []string{
		"GigaBrands <x@candidates.workablemail.com>", // Workable (board domain is workable.com)
		"y@inbound.workablemail.com",
		"Avenga Careers <x@avenga.teamtailor-mail.com>",              // Teamtailor relay
		"Moon Active <no-reply@moonactive.comeet-notifications.com>", // Comeet relay
		"Bitfinex <noreply@join.com>",                                // Join
		"x@recruitee-mailbox.com",                                    // Recruitee relay
		"careers@getambush-talent.freshteam.com",                     // Freshteam
		"G42 Careers <g42+autoreply@talent.icims.eu>",                // iCIMS EU
		"Wiser <system@successfactors.eu>",                           // SuccessFactors EU
		"no-reply@gupy.com.br",                                       // Gupy
		"jobs@m.personio.com",                                        // Personio relay
		"do-not-reply@mail.paylocity.com",                            // Paylocity relay
		"Linh <linh@m.talentlyft.com>",                               // TalentLyft relay
	}
	for _, s := range yes {
		if !IsATSSender(s) {
			t.Errorf("IsATSSender(%q) = false, want true", s)
		}
	}
}

func TestBuildQuery(t *testing.T) {
	q := BuildQuery(1_700_000_000, nil)
	if !strings.Contains(q, "from:(") {
		t.Errorf("query missing from:() clause: %q", q)
	}
	if !strings.Contains(q, "greenhouse-mail.io") || !strings.Contains(q, "ashbyhq.com") {
		t.Errorf("query missing ATS domains: %q", q)
	}
	if !strings.Contains(q, "after:1700000000") {
		t.Errorf("query missing after: watermark: %q", q)
	}
}

func TestBuildQueryNoWatermark(t *testing.T) {
	// A zero watermark (first run) omits the after: clause → full backfill.
	q := BuildQuery(0, nil)
	if strings.Contains(q, "after:") {
		t.Errorf("zero watermark should omit after:, got %q", q)
	}
}

// TestBuildQueryLearnedDomains locks in that promoted self-learning domains are
// unioned into the sender clause, so the query grows without hardcoding.
func TestBuildQueryLearnedDomains(t *testing.T) {
	q := BuildQuery(0, []string{"teamex.io", "ceipalmail.com"})
	if !strings.Contains(q, "teamex.io") || !strings.Contains(q, "ceipalmail.com") {
		t.Errorf("learned domains not unioned into query: %q", q)
	}
	// A nil learned set must not change the hardcoded core.
	if strings.Contains(BuildQuery(0, nil), "teamex.io") {
		t.Error("nil learned set should not inject domains")
	}
}

// TestBuildQueryRecallSignals locks in the non-ATS-domain recall class measured as
// missed by the sender allowlist alone: multilingual application phrases matched by
// Gmail full-text. Everything the query pulls is LLM-classified downstream, so the
// query is recall-first.
func TestBuildQueryRecallSignals(t *testing.T) {
	q := BuildQuery(1_700_000_000, nil)
	wants := []string{
		`"thank you for applying"`,    // strong English phrase
		`"recebemos sua candidatura"`, // pt: multilingual recall
	}
	for _, w := range wants {
		if !strings.Contains(q, w) {
			t.Errorf("query missing recall signal %q: %q", w, q)
		}
	}
	// The recall clauses are OR-ed inside one group; after: applies to the whole
	// group, so it must sit outside the parenthesised union.
	if !strings.HasPrefix(q, "(") || !strings.Contains(q, ") after:1700000000") {
		t.Errorf("after: must apply to the whole OR-group: %q", q)
	}
	// LinkedIn is no longer a recall source — no linkedin.com senders in the query.
	if strings.Contains(q, "linkedin.com") {
		t.Errorf("query should not reference LinkedIn: %q", q)
	}
}

// The employer clause has to spell the name the way a SENDER would, not the way the
// catalogue does. "Blend 360" signs itself "Blend360" — the same class of mismatch that
// killed the calendar title tier, which compared against a hyphenated slug.
func TestBuildRecallQuerySpellsTheEmployerBothWays(t *testing.T) {
	since := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	q := BuildRecallQuery("Blend 360", "Senior AI Engineer", since, until)

	for _, want := range []string{`"Blend 360"`, `"Blend360"`} {
		if !strings.Contains(q, want) {
			t.Errorf("query does not spell the employer as %s:\n%s", want, q)
		}
	}
	if !strings.Contains(q, fmt.Sprintf("after:%d", since.Unix())) ||
		!strings.Contains(q, fmt.Sprintf("before:%d", until.Unix())) {
		t.Errorf("query is not bounded by the window:\n%s", q)
	}
}

// The gate is what stands between an employer's name and the caller's private mail, and it
// takes all three members. Hiring vocabulary ALONE was measured against 20 applications: it
// cut 53 candidates to 41 and dropped both calendar invitations for an interview plus a
// live recruiter thread whose only subject was the role.
func TestBuildRecallQueryGateCarriesAllThreeMembers(t *testing.T) {
	q := BuildRecallQuery("Derq", "Full-Stack Engineer", time.Now(), time.Now())

	if !strings.Contains(q, "filename:ics") {
		t.Error("no filename:ics — every calendar invitation carries it, whatever language " +
			"its subject uses, and an invitation has no other route into the product")
	}
	if !strings.Contains(q, `"Full-Stack Engineer"`) {
		t.Error("the role is not in the gate — mail whose only subject is the role would be " +
			"dropped, and that is a measured, real class")
	}
	if !strings.Contains(q, "interview") {
		t.Error("no hiring vocabulary in the gate")
	}
}

// A quote in either field would end the term early and let the rest of the string act as
// query syntax. They are the only caller-supplied text in it.
func TestBuildRecallQueryStripsQuotes(t *testing.T) {
	q := BuildRecallQuery(`Ac"me`, `Sen"ior Dev`, time.Now(), time.Now())
	if strings.Contains(q, `Ac"me`) || strings.Contains(q, `Sen"ior`) {
		t.Errorf("a quote survived into the query:\n%s", q)
	}
	if !strings.Contains(q, `"Acme"`) || !strings.Contains(q, `"Senior Dev"`) {
		t.Errorf("stripping mangled the terms:\n%s", q)
	}
}

// An application with no role recorded still has an employer, and the gate keeps working.
func TestBuildRecallQueryWithoutARole(t *testing.T) {
	q := BuildRecallQuery("Derq", "", time.Now(), time.Now())
	if strings.Contains(q, `""`) {
		t.Errorf("an empty role left an empty term in the query:\n%s", q)
	}
	if !strings.Contains(q, "filename:ics") {
		t.Errorf("the rest of the gate went missing with the role:\n%s", q)
	}
}

// The list knew one canonical wording of each thing and employers use several. These are
// the wordings a live mailbox proved were being missed: an acknowledgement reading
// "we've received your … application" where the list knew only "your application at", and
// an invitation reading "interview invite" where it knew only "invite you to interview".
func TestBuildQueryCoversTheWordingsEmployersActuallyUse(t *testing.T) {
	q := BuildQuery(0, nil)
	for _, phrase := range []string{
		"received your application", "interview invite", "interview invitation",
	} {
		if !strings.Contains(q, `"`+phrase+`"`) {
			t.Errorf("the query does not look for %q — measured as missed on a live mailbox", phrase)
		}
	}
	// The wordings that were already there must survive: this list only ever grows, and a
	// phrase silently dropped is mail silently missed for as long as nobody notices.
	for _, phrase := range []string{
		"thank you for applying", "we regret to inform", "recebemos sua candidatura",
		"ваш отклик", "hemos recibido tu",
	} {
		if !strings.Contains(q, `"`+phrase+`"`) {
			t.Errorf("the query lost the existing phrase %q", phrase)
		}
	}
}

// Fetching the candidate's own replies buys nothing: worker.go drops them before storing.
// What it costs is a body retrieval each, and room in the page the widened query returns.
func TestBuildQueryExcludesTheConnectedAddress(t *testing.T) {
	q := BuildQueryFor("me@example.test", 0, nil)
	if !strings.Contains(q, "-from:me@example.test") {
		t.Errorf("the query does not exclude the connected address:\n%s", q)
	}
	// The exclusion sits OUTSIDE the OR group. Inside it, it would be one more alternative
	// and would match everything rather than removing anything.
	if strings.Contains(q, `OR -from:`) {
		t.Errorf("the exclusion was OR-ed into the alternatives:\n%s", q)
	}
	if plain := BuildQuery(0, nil); strings.Contains(plain, "-from:") {
		t.Errorf("an address nobody gave leaked into the query:\n%s", plain)
	}
}
