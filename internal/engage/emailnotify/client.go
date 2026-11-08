package emailnotify

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// Compile-time guarantee that Client is a Sender.
var _ Sender = (*Client)(nil)

// ErrNoWayOut reports a mail that can be turned off but carries no link to turn it
// off with. It is refused rather than sent.
//
// This is the choke point the whole change hangs on. Every mail this product sends —
// whether it renders through internal/application/mailtpl or builds its own HTML, as
// internal/engage/mentorship does — passes through one Send, so a mail with no way
// out cannot ship no matter which path built it. A guard over the shared template
// would have missed the hand-built ones entirely while looking like coverage.
var ErrNoWayOut = errors.New("emailnotify: a silenceable mail with no unsubscribe URL")

// sesAPI is the slice of the SES v2 client the Client uses, so tests inject a fake
// in its place. *sesv2.Client satisfies it.
type sesAPI interface {
	SendEmail(ctx context.Context, in *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
	GetConfigurationSet(ctx context.Context, in *sesv2.GetConfigurationSetInput, optFns ...func(*sesv2.Options)) (*sesv2.GetConfigurationSetOutput, error)
}

// Message is one email. It replaced three near-identical send methods — plain,
// with-reply-to, and with-attachments — which had between them started to enumerate
// the combinations of their optional parts. A fourth for unsubscribe headers would
// have been the point where that stopped being a smell and became a matrix.
//
// The six same-typed positional strings those methods took are also a signature in
// which From and To can be swapped without the compiler noticing.
type Message struct {
	From    string
	To      string
	Subject string
	HTML    string
	Text    string

	// Group classifies the mail and decides whether it needs a way out. It has no
	// usable zero value on purpose: an unset Group is refused, so classifying is not
	// something a new sender can forget to do.
	Group emailprefs.Group

	// UnsubscribeURL is where this recipient turns this group off. Required when
	// Group is silenceable, and forbidden when it is not — an unsubscribe link on a
	// password-reset mail is an offer we cannot honour.
	UnsubscribeURL string

	// ReplyTo routes replies somewhere other than the sending address. Empty sends
	// no header at all, which is what an automated mail wants: replies to a digest
	// belong nowhere. The founder sequence and campaigns set it, because they ask a
	// question and the sending address is a no-reply feeding the mail parser — an
	// actual reply landing there would be read by a robot looking for interview
	// invitations.
	ReplyTo string

	// Attachments switch SES from its simple path to a raw RFC 5322 message. Almost
	// every mail is body-only; today only a mentorship calendar invitation is not.
	Attachments []Attachment

	// Campaign names the letter for engagement tracking. Empty, as on anything a person
	// is waiting for, the mail goes out with no rewritten links. Set
	// only on campaigns and digests. SES restricts tag values to ASCII letters, digits,
	// '_', '-' and '.'.
	Campaign string
	// Variant is the side of a split test the letter belongs to. Empty outside one.
	Variant string
}

// validate refuses a message that cannot be sent honestly.
func (m Message) validate() error {
	switch {
	case m.Group == "":
		return fmt.Errorf("emailnotify: message to %s names no group: %w", m.To, ErrNoWayOut)
	case emailprefs.Silenceable(m.Group) && m.UnsubscribeURL == "":
		return fmt.Errorf("emailnotify: %s mail to %s: %w", m.Group, m.To, ErrNoWayOut)
	case !emailprefs.Silenceable(m.Group) && m.UnsubscribeURL != "":
		return fmt.Errorf("emailnotify: %s mail to %s carries an unsubscribe URL it cannot honour", m.Group, m.To)
	// The footer saying who wrote and why, with the way out, must reach the reader in
	// both parts: SES Trust & Safety refused our appeal partly because the mail was hard
	// to attribute, and a sender building its own body can drop it without noticing.
	case emailprefs.Silenceable(m.Group) && !strings.Contains(m.Text, emailprefs.TextFooter(m.UnsubscribeURL)):
		return fmt.Errorf("emailnotify: %s mail to %s: text part lacks emailprefs.TextFooter: %w", m.Group, m.To, ErrNoWayOut)
	case emailprefs.Silenceable(m.Group) && !strings.Contains(m.HTML, html.EscapeString(m.UnsubscribeURL)):
		return fmt.Errorf("emailnotify: %s mail to %s: HTML part lacks the unsubscribe link: %w", m.Group, m.To, ErrNoWayOut)
	}
	return nil
}

// withSenderName gives a bare From address the product's name, so no mail shows up in
// a message list as just "notifications". A From that already names someone (the
// founder sequence's "Ilya from freehire") is kept.
func withSenderName(from string) string {
	if a, err := mail.ParseAddress(from); err == nil && a.Name == "" {
		return From(productName, from)
	}
	return from
}

// unsubscribeHeaders is the RFC 8058 pair, or nothing for mail that cannot be
// silenced. Built here rather than by each sender so the format has one author: a
// List-Unsubscribe that Gmail declines to parse fails silently, and it would fail
// once per sender that spelled it itself.
//
// No `mailto:` alternative. It is optional, one-click is what Gmail and Yahoo
// actually require of a bulk sender, and an advertised address that bounces is
// worse than an absent one.
func (m Message) unsubscribeHeaders() []types.MessageHeader {
	if m.UnsubscribeURL == "" {
		return nil
	}
	return []types.MessageHeader{
		{Name: aws.String("List-Unsubscribe"), Value: aws.String("<" + m.UnsubscribeURL + ">")},
		{Name: aws.String("List-Unsubscribe-Post"), Value: aws.String("List-Unsubscribe=One-Click")},
	}
}

// Client is the AWS SES v2 email transport. It is a thin adapter over SendEmail —
// the render logic it serves lives in the callers and is covered by their tests.
type Client struct {
	ses sesAPI
	// trackingSet is the SES configuration set that publishes engagement events. Empty
	// tracks nothing, whatever a message's Campaign says.
	trackingSet string
	// breaker is nil unless WithSendingBreaker was called.
	breaker *breaker
	// relay, when SMTP_URL is set, carries every message instead of SES.
	relay *smtpRelay
}

// WithEngagementTracking returns the Client with the SES configuration set that
// publishes clicks, deliveries, bounces and complaints. Only the workers sending
// campaigns and digests call it; every other worker's client cannot track by construction.
func (c *Client) WithEngagementTracking(configurationSet string) *Client {
	out := *c
	out.trackingSet = configurationSet
	return &out
}

// trackingCheckTimeout bounds CheckEngagementTracking: the workers calling it have no deadline
// of their own, and a stalled check must not hold up everything else they deliver.
const trackingCheckTimeout = 10 * time.Second

// CheckEngagementTracking reports whether SES will send through the configuration set. A send
// naming a set it does not know, or one with sending disabled, is rejected, which would fail a
// whole audience, so a sender checks once before its first letter. Without a set there is
// nothing to check.
func (c *Client) CheckEngagementTracking(ctx context.Context) error {
	if c.trackingSet == "" || c.relay != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, trackingCheckTimeout)
	defer cancel()
	out, err := c.ses.GetConfigurationSet(ctx, &sesv2.GetConfigurationSetInput{ConfigurationSetName: aws.String(c.trackingSet)})
	if err != nil {
		return fmt.Errorf("emailnotify: configuration set %s: %w", c.trackingSet, err)
	}
	if out.SendingOptions != nil && !out.SendingOptions.SendingEnabled {
		return fmt.Errorf("emailnotify: configuration set %s has sending disabled", c.trackingSet)
	}
	return nil
}

// NewClient builds a Client from AWS config resolved via the default chain
// (SSO / IAM role / env). With SMTP_URL set it sends through that relay instead
// and SES is never called — credentials never come from app config, matching the
// apply service's inbound SES adapter.
func NewClient(ctx context.Context, region string) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("emailnotify: aws config: %w", err)
	}
	c := &Client{ses: sesv2.NewFromConfig(cfg)}
	// Read here rather than threaded through config so every worker that builds a
	// Client follows the switch without a change of its own.
	if raw := os.Getenv("SMTP_URL"); raw != "" {
		if c.relay, err = parseSMTPURL(raw); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Send delivers one message. A send error is returned so the caller (a delivery
// loop) retries and eventually dead-letters rather than dropping the notification.
func (c *Client) Send(ctx context.Context, m Message) error {
	m.From = withSenderName(m.From)
	if err := m.validate(); err != nil {
		return err
	}
	if c.breaker != nil && c.breaker.tripped.Load() {
		return ErrSendingPaused
	}
	if c.relay != nil {
		raw, err := buildRawMessage(m)
		if err != nil {
			return err
		}
		err = c.relay.send(ctx, m.From, m.To, raw)
		if c.breaker != nil && errors.Is(err, errRelayDown) {
			c.breaker.tripped.Store(true)
			return fmt.Errorf("%w: %w", ErrSendingPaused, err)
		}
		return err
	}
	in, err := c.input(m)
	if err != nil {
		return err
	}
	c.track(in, m)
	if _, err := c.ses.SendEmail(ctx, in); err != nil {
		if c.breaker != nil && accountRefused(err) {
			c.breaker.tripped.Store(true)
			return fmt.Errorf("emailnotify: ses send: %w: %w", ErrSendingPaused, err)
		}
		return fmt.Errorf("emailnotify: ses send to %s: %w", m.To, err)
	}
	return nil
}

// input picks the SES shape. A message with attachments goes out as a raw RFC 5322
// message we assemble ourselves; everything else uses the simple path, which SES
// bills and validates differently.
func (c *Client) input(m Message) (*sesv2.SendEmailInput, error) {
	if len(m.Attachments) > 0 {
		raw, err := buildRawMessage(m)
		if err != nil {
			return nil, err
		}
		return &sesv2.SendEmailInput{
			Content: &types.EmailContent{Raw: &types.RawMessage{Data: raw}},
		}, nil
	}
	in := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(m.From),
		Destination:      &types.Destination{ToAddresses: []string{m.To}},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String(m.Subject)},
				Body: &types.Body{
					Html: &types.Content{Data: aws.String(m.HTML)},
					Text: &types.Content{Data: aws.String(m.Text)},
				},
				Headers: m.unsubscribeHeaders(),
			},
		},
	}
	if m.ReplyTo != "" {
		in.ReplyToAddresses = []string{m.ReplyTo}
	}
	return in, nil
}

// track routes a campaign's mail through the configuration set and tags it, which is all
// the event consumer (internal/engage/emailevents) learns about the letter.
func (c *Client) track(in *sesv2.SendEmailInput, m Message) {
	if c.trackingSet == "" || m.Campaign == "" {
		return
	}
	in.ConfigurationSetName = aws.String(c.trackingSet)
	in.EmailTags = []types.MessageTag{{Name: aws.String("campaign"), Value: aws.String(m.Campaign)}}
	if m.Variant != "" {
		in.EmailTags = append(in.EmailTags, types.MessageTag{Name: aws.String("variant"), Value: aws.String(m.Variant)})
	}
}
