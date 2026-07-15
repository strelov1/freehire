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
	q := BuildQuery(1_700_000_000)
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
	q := BuildQuery(0)
	if strings.Contains(q, "after:") {
		t.Errorf("zero watermark should omit after:, got %q", q)
	}
}
