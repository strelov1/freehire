package gmailsync

import (
	"strings"
	"testing"
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
