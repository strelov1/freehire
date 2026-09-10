package atsapply

import "testing"

// Lever's forms carry an invisible hCaptcha — measured 2026-09-10 on 10 of 12 live
// postings — and this package's own headless Chrome passes it about one attempt in eight,
// with nothing about the browser moving the odds. The cloud agent runs a hardened browser
// behind residential proxies and solves supported captchas itself, so Lever goes there
// instead of down a fill path that mostly cannot finish.
func TestLeverGoesThroughTheCloudAgent(t *testing.T) {
	if fillProviders["lever"] {
		t.Error("lever is still a fill provider; its own Chrome loses the captcha coin toss seven times in eight")
	}
	if !browserUseProviders["lever"] {
		t.Error("lever is not routed to the cloud agent, so it would park instead of being submitted")
	}
}

// The agent is told which page to open, and for Lever that is not the posting URL: the
// posting carries the description and no form at all. A live attempt already parked as
// unrecognized_form_layout for exactly that reason on the Chrome path — handing the same
// wrong URL to the agent would repeat it, only slower and for money.
func TestCloudAgentIsSentTheFormsOwnPage(t *testing.T) {
	posting := "https://jobs.lever.co/acme/abc-123"

	got := agentTargetURL("lever", posting)

	if want := posting + "/apply"; got != want {
		t.Errorf("agentTargetURL = %q, want %q", got, want)
	}
}

// A provider whose posting URL IS the form page is handed it unchanged.
func TestCloudAgentTargetIsUnchangedWhereTheFormLivesOnThePosting(t *testing.T) {
	posting := "https://jobs.ashbyhq.com/acme/abc-123"

	if got := agentTargetURL("ashby", posting); got != posting {
		t.Errorf("agentTargetURL = %q, want it unchanged", got)
	}
}
