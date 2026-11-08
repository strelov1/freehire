package cvlifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

type captureSender struct {
	msgs []emailnotify.Message
	err  error
}

func (s *captureSender) Send(_ context.Context, m emailnotify.Message) error {
	s.msgs = append(s.msgs, m)
	return s.err
}

const letterUserID int64 = 42

func newMailer(sender emailnotify.Sender) *Mailer {
	links := emailprefs.NewLinks("cvlifecycle-test-secret-padded-to-32b", "https://freehire.me")
	return NewMailer(sender, "noreply@freehire.me", "ilya@freehire.me", "https://freehire.me/", links)
}

func letters() []Letter {
	return []Letter{
		{Step: StepATS, Overall: 61, Potential: 84, Gaps: []string{"Add a summary section"}, Keywords: []string{"Kubernetes"}},
		{Step: StepPotential, Overall: 61, Potential: 84},
		{Step: StepOffer, Offer: &Offer{Code: "ZZLETTER", Percent: 23}},
	}
}

func TestMailer_TagsEveryLetter(t *testing.T) {
	for _, l := range letters() {
		sender := &captureSender{}
		if err := newMailer(sender).Send(context.Background(), l, letterUserID, "reader@example.test"); err != nil {
			t.Fatalf("step %d: Send: %v", l.Step, err)
		}
		m := sender.msgs[0]
		if want := l.Step.Campaign(); m.Campaign != want {
			t.Errorf("step %d: Campaign = %q, want %q", l.Step, m.Campaign, want)
		}
		if m.Group != emailprefs.GroupNews {
			t.Errorf("step %d: Group = %v, want news", l.Step, m.Group)
		}
		if !strings.HasPrefix(m.UnsubscribeURL, "https://freehire.me/") || !strings.Contains(m.Text, m.UnsubscribeURL) || !strings.Contains(m.HTML, m.UnsubscribeURL) {
			t.Errorf("step %d: one-click unsubscribe link %q missing or not in both parts", l.Step, m.UnsubscribeURL)
		}
		if m.To != "reader@example.test" || m.ReplyTo != "ilya@freehire.me" || !strings.Contains(m.From, "Ilya from freehire") {
			t.Errorf("step %d: envelope %q / %q / %q", l.Step, m.From, m.To, m.ReplyTo)
		}
		if !strings.Contains(m.HTML, mailtpl.LinkedInURL) || !strings.Contains(m.Text, mailtpl.LinkedInURL) {
			t.Errorf("step %d: the founder's signature is missing", l.Step)
		}
	}
}

func TestMailer_LettersNameWhatTheyAreAbout(t *testing.T) {
	sender := &captureSender{}
	m := newMailer(sender)
	for _, l := range letters() {
		if err := m.Send(context.Background(), l, letterUserID, "reader@example.test"); err != nil {
			t.Fatal(err)
		}
	}
	first, second, third := sender.msgs[0], sender.msgs[1], sender.msgs[2]

	if first.Subject != "Your CV scored 61/100 in an ATS check" {
		t.Errorf("letter 1 subject = %q", first.Subject)
	}
	for _, want := range []string{"Add a summary section", "Kubernetes", "https://freehire.me/my/profile/cv-readiness?utm_source=email"} {
		if !strings.Contains(first.HTML, want) || !strings.Contains(first.Text, want) {
			t.Errorf("letter 1 does not carry %q in both parts", want)
		}
	}
	if second.Subject != "Your CV could score 84, not 61" || !strings.Contains(second.HTML, "https://freehire.me/features/tailor?utm_source=email") {
		t.Errorf("letter 2 = %q", second.Subject)
	}
	if third.Subject != "23% off your first month of Pro" {
		t.Errorf("letter 3 subject = %q", third.Subject)
	}
	for part, body := range map[string]string{"html": third.HTML, "text": third.Text} {
		if !strings.Contains(body, "23%") || !strings.Contains(body, "ZZLETTER") || !strings.Contains(body, "first month") {
			t.Errorf("letter 3 %s does not state the percent, the code and the first month", part)
		}
		if !strings.Contains(body, "https://freehire.me/pricing?utm_source=email") || !strings.Contains(body, "promo=ZZLETTER") {
			t.Errorf("letter 3 %s does not link the pricing page with the code", part)
		}
	}
}

func TestMailer_HostileStringsAreEscaped(t *testing.T) {
	hostile := `<script>alert(1)</script>"><img src=x onerror=alert(2)>`
	sender := &captureSender{}
	l := Letter{Step: StepATS, Overall: 10, Gaps: []string{hostile}, Keywords: []string{hostile}}
	if err := newMailer(sender).Send(context.Background(), l, letterUserID, "reader@example.test"); err != nil {
		t.Fatal(err)
	}
	html := sender.msgs[0].HTML
	if strings.Contains(html, "<script>alert(1)") || strings.Contains(html, "<img src=x") {
		t.Fatalf("an account-derived string reached the markup unescaped:\n%s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("the hostile string was dropped instead of escaped")
	}
}

func TestMailer_NoAddressOrContactInTheLetter(t *testing.T) {
	sender := &captureSender{}
	for _, l := range letters() {
		if err := newMailer(sender).Send(context.Background(), l, letterUserID, "reader@example.test"); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range sender.msgs {
		for _, body := range []string{m.HTML, m.Text, m.Subject} {
			if strings.Contains(body, "reader@example.test") {
				t.Errorf("%s: the recipient's address is in the letter", m.Campaign)
			}
		}
	}
}

func TestMailer_OfferLetterNeedsAConfirmedOffer(t *testing.T) {
	for _, offer := range []*Offer{nil, {Code: "", Percent: 15}, {Code: "ZZLETTER", Percent: 0}} {
		sender := &captureSender{}
		err := newMailer(sender).Send(context.Background(), Letter{Step: StepOffer, Offer: offer}, letterUserID, "reader@example.test")
		if err == nil || len(sender.msgs) != 0 {
			t.Errorf("offer %+v: err %v, sent %d, want a refusal before SES", offer, err, len(sender.msgs))
		}
	}
}

func TestMailer_RefusesAnUnknownStep(t *testing.T) {
	if err := newMailer(&captureSender{}).Send(context.Background(), Letter{Step: 9}, letterUserID, "x@example.test"); err == nil {
		t.Fatal("Send accepted an unknown step")
	}
}

func TestMailer_PropagatesTransportErrors(t *testing.T) {
	boom := errors.New("ses: down")
	err := newMailer(&captureSender{err: boom}).Send(context.Background(), letters()[1], letterUserID, "x@example.test")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the transport error", err)
	}
}

func TestMailer_UnsignedLinksFail(t *testing.T) {
	m := NewMailer(&captureSender{}, "a@freehire.me", "b@freehire.me", "https://freehire.me", emailprefs.NewLinks("short", "https://freehire.me"))
	if err := m.Send(context.Background(), letters()[1], letterUserID, "x@example.test"); err == nil {
		t.Fatal("Send went out without a working unsubscribe link")
	}
}
