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
		// The rule is one-directional: a rejection phrase suppresses a positive signal,
		// never the other way round. A real offer with no rejection wording is unaffected
		// — pinned above by "job offer" — and so is an invitation.
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
