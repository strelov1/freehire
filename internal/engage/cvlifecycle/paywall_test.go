package cvlifecycle

import (
	"context"
	"strings"
	"testing"
)

func sendPaywall(t *testing.T, p *Paywall) (subject, html, text string) {
	t.Helper()
	sender := &captureSender{}
	if err := newMailer(sender).Send(context.Background(), Letter{Step: StepPaywall, Paywall: p}, letterUserID, "reader@example.test"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	m := sender.msgs[0]
	return m.Subject, m.HTML, m.Text
}

func TestPaywallLetter_NamesTheFeatureAndThePosting(t *testing.T) {
	subject, html, text := sendPaywall(t, &Paywall{Feature: "tailor", JobTitle: "Go Engineer", Company: "Acme", JobSlug: "go-engineer-acme-1"})

	if subject != "You wanted to tailor your CV" {
		t.Errorf("subject = %q", subject)
	}
	for part, body := range map[string]string{"html": html, "text": text} {
		for _, want := range []string{"tailor your CV for Go Engineer at Acme", "Pro", "https://freehire.me/pricing?utm_source=email", "https://freehire.me/jobs/go-engineer-acme-1?utm_source=email"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s does not carry %q", part, want)
			}
		}
	}
}

func TestPaywallLetter_WithoutAPostingNamesOnlyTheFeature(t *testing.T) {
	_, html, text := sendPaywall(t, &Paywall{Feature: "tailor"})
	for part, body := range map[string]string{"html": html, "text": text} {
		if !strings.Contains(body, "wanted to tailor your CV.") {
			t.Errorf("%s does not name the feature alone", part)
		}
		if strings.Contains(body, "/jobs/") || strings.Contains(body, "tailor your CV for") {
			t.Errorf("%s names a posting that the hit did not carry", part)
		}
	}
}

func TestPaywallLetter_EveryGateLabelHasWordsAndAnUnknownOneStillGetsALetter(t *testing.T) {
	labels := []string{"tailor", "match", "assistant", "dictation", "cover-letter", "auto-apply", "ats_review", "mail_recall", "autofill"}
	seen := map[string]bool{}
	for _, label := range labels {
		if _, ok := attempts[label]; !ok {
			t.Errorf("no words for the gate's %q label", label)
		}
		subject, _, _ := sendPaywall(t, &Paywall{Feature: label, JobTitle: "Go Engineer", Company: "Acme"})
		if seen[subject] {
			t.Errorf("%q: subject %q is shared with another feature", label, subject)
		}
		seen[subject] = true
	}
	for _, label := range []string{"ai_other", "a-route-added-later"} {
		subject, _, text := sendPaywall(t, &Paywall{Feature: label})
		if subject != "You wanted to use one of the AI features" || !strings.Contains(text, "Pro") {
			t.Errorf("%q: subject %q", label, subject)
		}
	}
}

func TestPaywallLetter_PostingFromTheCatalogueIsEscaped(t *testing.T) {
	hostile := `<script>alert(1)</script>`
	_, html, _ := sendPaywall(t, &Paywall{Feature: "match", JobTitle: hostile, Company: hostile, JobSlug: `x"><b>`})
	if strings.Contains(html, "<script>alert(1)") || strings.Contains(html, `"><b>`) {
		t.Fatalf("a posting field reached the markup unescaped:\n%s", html)
	}
}

func TestPaywallLetter_NeedsTheHitItAnswers(t *testing.T) {
	sender := &captureSender{}
	if err := newMailer(sender).Send(context.Background(), Letter{Step: StepPaywall}, letterUserID, "x@example.test"); err == nil || len(sender.msgs) != 0 {
		t.Fatalf("err %v, sent %d, want a refusal before SES", err, len(sender.msgs))
	}
}

func TestPaywallLetter_FooterDoesNotClaimAnUpload(t *testing.T) {
	_, html, _ := sendPaywall(t, &Paywall{Feature: "tailor"})
	if strings.Contains(html, "uploaded a CV") {
		t.Error("the paywall letter's footer says the account uploaded a CV")
	}
}
