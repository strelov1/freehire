package mailclassify

import "testing"

func TestKeywordStatus(t *testing.T) {
	cases := []struct {
		name    string
		subject string
		body    string
		want    StatusSignal
		ok      bool
	}{
		{
			name:    "explicit rejection fires",
			subject: "Regarding your application",
			body:    "Thank you for your interest. Unfortunately we have decided not to proceed with your application.",
			want:    SignalRejection, ok: true,
		},
		{
			name:    "rejection wins over the acknowledgement opener",
			subject: "Thank you for applying to Acme",
			body:    "Thank you for applying to Acme. We regret to inform you that we will not be moving forward.",
			want:    SignalRejection, ok: true,
		},
		{
			name:    "clear acknowledgement template",
			subject: "Thank you for your application to Binance",
			body:    "We have received your application and will review it shortly.",
			want:    SignalAcknowledgement, ok: true,
		},
		{
			name:    "explicit interview invitation",
			subject: "Next steps",
			body:    "We would like to invite you to interview. Please schedule a call using the link.",
			want:    SignalInterviewInvitation, ok: true,
		},
		{
			name:    "job offer",
			subject: "Great news",
			body:    "We are pleased to offer you the position of Senior Engineer.",
			want:    SignalOffer, ok: true,
		},
		{
			name:    "ambiguous interest opener alone defers",
			subject: "Thank you for your interest in Xata",
			body:    "Thank you for your interest in Xata!",
			want:    "", ok: false,
		},
		{
			name:    "unrelated content defers",
			subject: "Your sign-in code",
			body:    "Your one-time code is 123456.",
			want:    "", ok: false,
		},
		// The four phrasings below all resolved to a POSITIVE signal before the
		// contradiction rule, measured by running KeywordStatus. The last two were the
		// damaging pair: `offer` clears the stage-advance threshold at keyword
		// confidence, so a rejection moved the application from `applied` to `offer`.
		{
			name:    "rejection quoting offer vocabulary is not an offer",
			subject: "Your application to Acme",
			body:    "After careful consideration we are unable to extend an offer at this time.",
			want:    SignalRejection, ok: true,
		},
		{
			name:    "an offer extended to somebody else is a rejection",
			subject: "Update on your application",
			body:    "We have decided to extend an offer to another candidate for this role.",
			want:    SignalRejection, ok: true,
		},
		{
			name:    "the canonical rejection phrase",
			subject: "Regarding your application",
			body:    "We regret to inform you we cannot extend an offer.",
			want:    SignalRejection, ok: true,
		},
		{
			name:    "rejection after an assessment is a rejection, not an assessment",
			subject: "Coding challenge results",
			body:    "Thank you for completing the coding challenge. Unfortunately we are not moving forward with your application.",
			want:    SignalRejection, ok: true,
		},
		// The veto reads decisiveRejectionPhrases, a SUBSET, and these six are why. Each
		// holds a phrase from the wider rejection list inside unambiguously good news, and
		// vetoing on the whole list turned every one of them into `rejection` — which
		// SuggestStage then offered the candidate as a move to `rejected`, on the day they
		// were hired. All six were measured against both spellings of the rule.
		{
			name:    "an offer that also says it decided to move forward with you",
			subject: "Offer from Acme",
			body:    "We are pleased to offer you the role. We have decided to move forward with you.",
			want:    SignalOffer, ok: true,
		},
		{
			name:    "an invitation that mentions other candidates is still an invitation",
			subject: "Next steps",
			body:    "We would like to invite you to an interview. We are also speaking with other candidates this week.",
			want:    SignalInterviewInvitation, ok: true,
		},
		{
			name:    "an offer made because another candidate withdrew",
			subject: "Offer from Acme",
			body:    "We are pleased to offer you the role after another candidate withdrew.",
			want:    SignalOffer, ok: true,
		},
		{
			name:    "an invitation that says you were not the right fit for a DIFFERENT role",
			subject: "A different opening",
			body:    "We would like to schedule a call. You were not the right fit for the Staff role, but this one suits you.",
			want:    SignalInterviewInvitation, ok: true,
		},
		{
			name:    "an assessment that compares against other candidates",
			subject: "Take-home",
			body:    "Please complete the take-home so we can compare against other candidates' submissions.",
			want:    SignalAssessment, ok: true,
		},
		{
			name:    "moving forward WITH the candidate reads as the invitation it is",
			subject: "Next steps",
			body:    "We have decided to move forward with your application and would like to invite you to an interview.",
			want:    SignalInterviewInvitation, ok: true,
		},
		// The rule is one-directional: a decisive rejection phrase suppresses a positive
		// signal, never the other way round. A real offer with no rejection wording is
		// unaffected — pinned above by "job offer" — and so is an invitation.
		{
			name:    "an invitation with no rejection wording still resolves as one",
			subject: "Next steps at Acme",
			body:    "We would like to invite you to an interview next week — please book a time.",
			want:    SignalInterviewInvitation, ok: true,
		},
	}
	for _, c := range cases {
		got, ok := KeywordStatus(c.subject, c.body)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: KeywordStatus() = (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}
