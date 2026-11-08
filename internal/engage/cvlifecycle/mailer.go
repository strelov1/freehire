package cvlifecycle

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/url"
	"strings"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// senderName is what the recipient's list shows. The letters are first-person from the
// founder and invite a reply, so the sender reads as a person, as in the onboarding mails.
const senderName = "Ilya from freehire"

// Mailer renders and sends the letters.
type Mailer struct {
	sender  emailnotify.Sender
	from    string
	replyTo string
	baseURL string
	layout  *mailtpl.Layout
	links   *emailprefs.Links
}

// NewMailer builds a Mailer. replyTo is the human inbox that answers: the letters ask for
// a reply, so the caller refuses to run without one.
func NewMailer(sender emailnotify.Sender, from, replyTo, baseURL string, links *emailprefs.Links) *Mailer {
	base := strings.TrimRight(baseURL, "/")
	return &Mailer{
		sender:  sender,
		from:    emailnotify.From(senderName, from),
		replyTo: replyTo,
		baseURL: base,
		layout:  mailtpl.New(base),
		links:   links,
	}
}

// Send implements Sender: it delivers one composed letter, tagged with the step's campaign
// so SES engagement tracking reports each letter on its own.
func (m *Mailer) Send(ctx context.Context, letter Letter, userID int64, to string) error {
	unsubscribe, err := m.links.For(userID, emailprefs.GroupNews)
	if err != nil {
		return fmt.Errorf("cvlifecycle: unsubscribe link for user %d: %w", userID, err)
	}
	mail, err := m.render(letter, unsubscribe)
	if err != nil {
		return err
	}
	return m.sender.Send(ctx, emailnotify.Message{
		From: m.from, To: to, ReplyTo: m.replyTo, Subject: mail.subject,
		HTML: mail.html, Text: mail.text,
		Group: emailprefs.GroupNews, UnsubscribeURL: unsubscribe,
		Campaign: letter.Step.Campaign(),
	})
}

type rendered struct {
	subject, html, text string
}

// data is what the body templates render from. Everything that came from an account
// (skill names, check wording) reaches the markup as a field of this struct, so
// html/template escapes it; no template is ever built from those strings.
type data struct {
	Overall, Potential int
	Gaps, Keywords     []string
	Percent            int16
	Code               string
	ReportURL          string
	TailorURL          string
	OfferURL           string
	PortraitURL        string
	LinkedInIcon       string
	LinkedInURL        string
}

func (m *Mailer) data(l Letter) (data, error) {
	d := data{
		Overall: l.Overall, Potential: l.Potential, Gaps: l.Gaps, Keywords: l.Keywords,
		ReportURL:    m.baseURL + "/my/cvs?utm_source=email",
		TailorURL:    m.baseURL + "/features/tailor?utm_source=email",
		PortraitURL:  m.baseURL + "/ilya.jpg",
		LinkedInIcon: m.baseURL + "/email-icon-linkedin.png",
		LinkedInURL:  mailtpl.LinkedInURL,
	}
	if l.Step == StepOffer {
		// A letter that states a discount must have one confirmed by the promo service;
		// there is no honest version of this letter without it.
		if l.Offer == nil || l.Offer.Code == "" || l.Offer.Percent <= 0 {
			return data{}, fmt.Errorf("cvlifecycle: step %d needs a confirmed offer", l.Step)
		}
		d.Percent = l.Offer.Percent
		d.Code = l.Offer.Code
		d.OfferURL = m.baseURL + "/pricing?utm_source=email&promo=" + url.QueryEscape(l.Offer.Code)
	}
	return d, nil
}

func (m *Mailer) render(l Letter, unsubscribe string) (rendered, error) {
	spec, ok := specs[l.Step]
	if !ok {
		return rendered{}, fmt.Errorf("cvlifecycle: unknown step %d", l.Step)
	}
	d, err := m.data(l)
	if err != nil {
		return rendered{}, err
	}
	var body bytes.Buffer
	if err := spec.body.Execute(&body, d); err != nil {
		return rendered{}, fmt.Errorf("cvlifecycle: rendering step %d: %w", l.Step, err)
	}
	html := m.layout.Render(mailtpl.Body{
		Preheader:      spec.preheader(d),
		Heading:        spec.heading,
		Content:        template.HTML(body.String()), //nolint:gosec // html/template output: every account-derived value was escaped as a field of data
		Footer:         "You’re getting this because you uploaded a CV to freehire.",
		UnsubscribeURL: unsubscribe,
	})
	text := spec.text(d) + "\n— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n" + emailprefs.TextFooter(unsubscribe)
	return rendered{subject: spec.subject(d), html: html, text: text}, nil
}
