package atsapply

import (
	"testing"

	"github.com/strelov1/freehire/internal/api/candidateprofile"
	"github.com/strelov1/freehire/internal/ingest/applyform"
)

func opts(labels ...string) []applyform.Option {
	out := make([]applyform.Option, len(labels))
	for i, l := range labels {
		out[i] = applyform.Option{Label: l, Value: "v" + string(rune('0'+i))}
	}
	return out
}

// The candidate's standing consent travels the same road every other screening fact does:
// candidateprofile projects it, Resolve reads it. A key that drifted between the two would
// park every one of these forms again, which is what this test exists to catch.
func TestResolve_StandingConsentAnswersWorkablesGDPRField(t *testing.T) {
	consented := candidateprofile.Profile{PrivacyConsent: "yes"}
	gdpr := []MergedField{{ID: "gdpr", Kind: "checkbox_group", Required: true}}

	plan := Resolve(gdpr, consented.FieldsWithBankedAnswers(), false)

	if !plan.FullyResolved() {
		t.Fatalf("unmapped = %+v, want the gdpr box ticked from the standing consent", plan.Unmapped)
	}
	if plan.Fields[0].Value != "yes" {
		t.Errorf("value = %q, want yes", plan.Fields[0].Value)
	}
}

// Without the consent the field parks exactly as before — agreeing to a third party's terms
// is the candidate's act, and nothing here assumes it.
func TestResolve_NoStandingConsentStillParks(t *testing.T) {
	gdpr := []MergedField{{ID: "gdpr", Kind: "checkbox_group", Required: true}}

	plan := Resolve(gdpr, candidateprofile.Profile{}.FieldsWithBankedAnswers(), false)

	if plan.FullyResolved() {
		t.Fatalf("fields = %+v, want the consent to park for a candidate who has not granted it", plan.Fields)
	}
}

// Labels measured on the captured forms of 2026-10-25, each a condition of applying.
func TestPrivacyConsent_RecognisesTheConditionOfApplying(t *testing.T) {
	for _, label := range []string{
		"Capco Job Candidate Privacy Notice Acknowledgement",
		"GDPR Notification",
		"Please acknowledge that you have read and agree to our Privacy Policy.",
		"I acknowledge and agree to the processing of my personal data in accordance with Okta's privacy policy",
		"Do you consent to Brex processing your personal information for the purpose of assessing your candidacy?",
		"Declaro que li e concordo com o Aviso de Privacidade e estou ciente de que meus dados pessoais serão coletados",
		"Ik heb de privacyverklaring van VolkerWessels gelezen en begrijp hoe mijn persoonsgegevens worden verwerkt.",
		"Processing of Personal Data",
	} {
		if !isPrivacyConsentField(MergedField{ID: "q", Label: label, Kind: "select", Required: true}) {
			t.Errorf("isPrivacyConsentField(%q) = false, want true", label)
		}
	}
}

// Privacy-worded fields on the same pages that are NOT the condition of applying. Agreeing
// to any of them on the candidate's behalf would sign them up for something they never
// asked for, or disclose something that is theirs alone.
func TestPrivacyConsent_LeavesOptInsAndDisclosuresAlone(t *testing.T) {
	for _, label := range []string{
		"Yes, TSMG can contact me about future job opportunities for up to 10 yearsPrivacy policy",
		"I consent to my personal data being stored and used for consideration in future job openings within WPP Media.",
		"I hereby consent to receive text messages from Encora subject to the terms of Encora's Privacy Policy.",
		"SMS Consent: Do you agree to receive mobile messages from us? Reply STOP to opt-out. Privacy policy applies.",
		"You can choose to receive recruitment marketing communications from us under our privacy notice.",
		"Do you have any agreement with your current employer (such as a non-disclosure/confidentiality agreement)?",
		"I consent to the processing of my personal data regarding my ethnic origin for diversity monitoring.",
	} {
		if isPrivacyConsentField(MergedField{ID: "q", Label: label, Kind: "select", Required: true}) {
			t.Errorf("isPrivacyConsentField(%q) = true, want false", label)
		}
	}
}

// A text field naming the privacy policy is asking for something other than agreement.
func TestPrivacyConsent_OnlyCheckboxesAndSelects(t *testing.T) {
	if isPrivacyConsentField(MergedField{ID: "q", Label: "Privacy Policy", Kind: "text", Required: true}) {
		t.Error("a text field qualified as a consent")
	}
}

func TestAffirmativeOption(t *testing.T) {
	cases := []struct {
		name   string
		field  MergedField
		want   string
		wantOK bool
	}{
		{"no options takes yes", MergedField{Kind: "checkbox_group"}, "yes", true},
		{"a lone checkbox takes its option", MergedField{Kind: "checkbox_group", Options: opts("1")}, "v0", true},
		{"acknowledge", MergedField{Kind: "select", Options: opts("I acknowledge")}, "v0", true},
		{"yes beside no", MergedField{Kind: "select", Options: opts("Yes", "No")}, "v0", true},
		{"agree beside do not agree", MergedField{Kind: "select", Options: opts("I do not agree", "I agree")}, "v1", true},
		{"portuguese", MergedField{Kind: "select", Options: opts("Sim", "Não")}, "v0", true},
		{"two agreeing options park", MergedField{Kind: "select", Options: opts("Yes", "I agree")}, "", false},
		{"a lone checkbox that refuses parks", MergedField{Kind: "checkbox_group", Options: opts("I do not agree")}, "", false},
		{"a select with no options parks", MergedField{Kind: "select"}, "", false},
		{"no agreeing option parks", MergedField{Kind: "select", Options: opts("동의합니다", "동의하지 않습니다")}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := affirmativeOption(c.field)
			if ok != c.wantOK || (ok && got != c.want) {
				t.Errorf("affirmativeOption = (%q, %v), want (%q, %v)", got, ok, c.want, c.wantOK)
			}
		})
	}
}

// A background-check consent addressed to the screening firm's privacy mailbox reads as a
// privacy field, in a language the agreeing words are not listed for. It must park rather
// than have an option picked for it — measured on 859 Korean forms of 2026-10-25.
func TestResolve_ConsentWithUnreadableOptionsParks(t *testing.T) {
	field := []MergedField{{
		ID: "question_6", Kind: "select", Required: true,
		Label:   "6. (필수) 지원자는 AIM Screening Limited(U.S., privacy@sterlingcheck.com)에게 i) 학력 조회, ii) 자격사항 조회",
		Options: opts("동의합니다", "동의하지 않습니다"),
	}}

	plan := Resolve(field, candidateprofile.Profile{PrivacyConsent: "yes"}.FieldsWithBankedAnswers(), false)

	if plan.FullyResolved() {
		t.Fatalf("fields = %+v, want the field to park", plan.Fields)
	}
}

// The permission covers the condition of applying. An optional consent is an extra the form
// offers, and a labelled gdpr field asking about marketing is a marketing opt-in.
func TestPrivacyConsent_OnlyTheRequiredConditionOfApplying(t *testing.T) {
	for name, f := range map[string]MergedField{
		"optional":                 {ID: "q", Label: "Privacy Policy", Kind: "select"},
		"gdpr id, marketing label": {ID: "gdpr", Label: "Send me marketing emails", Kind: "checkbox_group", Required: true},
	} {
		if isPrivacyConsentField(f) {
			t.Errorf("%s: qualified as a consent", name)
		}
	}
}
