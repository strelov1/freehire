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
	}
	for _, c := range cases {
		got, ok := KeywordStatus(c.subject, c.body)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: KeywordStatus() = (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}
