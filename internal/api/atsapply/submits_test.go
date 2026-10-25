package atsapply

import (
	"context"
	"testing"

	aiplan "github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/autoapply"
)

// Drafting follows what can actually be sent. A Workable application is sent by the
// browser-use executor, so with it wired and enforcing its answers are drafted — the review
// screen promised exactly that, and an approved application parked on fourteen undrafted
// answers when they were not.
func TestSubmits_FollowsEveryPathThatCompletesAnApplication(t *testing.T) {
	executor := &BrowserUseExecutor{spend: &runSpendGuard{}}

	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "1")
	if !(&Client{browserUse: executor}).submits("workable") {
		t.Error("workable with an enforcing browser-use executor: submits = false")
	}
	if !(&Client{}).submits("greenhouse") {
		t.Error("greenhouse: submits = false")
	}
	if (&Client{}).submits("workable") {
		t.Error("workable with no executor: submits = true — a draft nothing will send")
	}

	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "")
	if (&Client{browserUse: executor}).submits("workable") {
		t.Error("workable in shadow mode: submits = true — shadow mode sends nothing")
	}
}

// Workable names its own identity fields firstname/lastname/address.
func TestResolve_WorkableIdentityFields(t *testing.T) {
	fields := []MergedField{
		{ID: "firstname", Label: "First name", Kind: "text", Required: true},
		{ID: "lastname", Label: "Last name", Kind: "text", Required: true},
		{ID: "address", Label: "Address", Kind: "text", Required: true},
	}

	plan := Resolve(fields, map[string]string{"first_name": "Ilya", "last_name": "Strelov", "location": "São Paulo, Brazil"}, false)

	if !plan.FullyResolved() {
		t.Fatalf("unmapped = %+v", plan.Unmapped)
	}
	if plan.Fields[0].Value != "Ilya" || plan.Fields[1].Value != "Strelov" || plan.Fields[2].Value != "São Paulo, Brazil" {
		t.Errorf("fields = %+v", plan.Fields)
	}
}

// A run whose browser-use budget is spent cannot send a Workable application, so it does not
// pay to draft one either.
func TestSubmits_NotOnceTheBrowserUseBudgetIsSpent(t *testing.T) {
	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "1")
	spent := &BrowserUseExecutor{spend: &runSpendGuard{limitUSD: 1, spentUSD: 1}}

	if (&Client{browserUse: spent}).submits("workable") {
		t.Error("submits = true with the day's browser-use budget spent")
	}
}

// A form the candidate's own answers already cover drafts nothing, so it is not refused over
// a lapsed plan it would never have used.
func TestClientResolve_AFullyAnsweredFormNeedsNoPaidPlan(t *testing.T) {
	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "1")
	lapsed := &fakeEntitlement{err: aiplan.ErrPaymentRequired}
	c := &Client{atoms: &recordingAtomReader{}, entitlement: lapsed, browserUse: &BrowserUseExecutor{spend: &runSpendGuard{}}}
	fields := []MergedField{{ID: "email", Label: "Email", Kind: "text", Required: true}}

	plan, err := c.resolve(context.Background(), autoapply.Claimed{Provider: "workable", UserID: 42}, fields, map[string]string{"email": "a@b.c"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !plan.FullyResolved() || lapsed.asked != 0 {
		t.Errorf("plan = %+v, plan asked %d times — want resolved without asking", plan, lapsed.asked)
	}
}
