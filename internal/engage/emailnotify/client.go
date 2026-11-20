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
	"github.com/strelov1/freehire/internal/platform/mailmeta"
)

// Compile-time guarantee that Client is a Sender.
var _ Sender = (*Client)(nil)

// ErrNoWayOut reports a mail that can be turned off but carries no link to turn it
// off with. It is refused rather than sent.
//
// This is the choke point: every mail, whether rendered through
// internal/application/mailtpl or hand-built (internal/engage/mentorship), passes
// one Send, so a mail with no way out cannot ship from any path.
var ErrNoWayOut = errors.New("emailnotify: a silenceable mail with no unsubscribe URL")

// sesAPI is the slice of the SES v2 client the Client uses, so tests inject a fake
// in its place. *sesv2.Client satisfies it.
type sesAPI interface {
	SendEmail(ctx context.Context, in *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
	GetConfigurationSet(ctx context.Context, in *sesv2.GetConfigurationSetInput, optFns ...func(*sesv2.Options)) (*sesv2.GetConfigurationSetOutput, error)
}

// Message is one email. A struct rather than positional strings, in which From
// and To could be swapped without the compiler noticing.
type Message struct {
	From    string
	To      string
	Subject string
	// NotificationKind identifies product summaries that inbound readers must not re-ingest.
	NotificationKind string
	HTML             string
	Text             string

	// Group classifies the mail and decides whether it needs a way out. It has no
	// usable zero value on purpose: an unset Group is refused.
	Group emailprefs.Group

	// UnsubscribeURL is where this recipient turns this group off. Required when
	// Group is silenceable, and forbidden when it is not — an unsubscribe link on a
	// password-reset mail is an offer we cannot honour.
	UnsubscribeURL string

	// ReplyTo routes replies away from the sending address. Empty sends no header,
	// which an automated mail wants. Mail that asks a question sets it: the sending
	// address is a no-reply feeding the mail parser.
	ReplyTo string

	// Attachments switch SES from its simple path to a raw RFC 5322 message.
	Attachments []Attachment

	// Campaign names the letter for engagement tracking. Empty (anything a person is
	// waiting for) goes out with no rewritten links. SES restricts tag values to
	// ASCII letters, digits, '_', '-' and '.'.
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
	// The footer (who wrote and why, with the way out) must reach the reader in both
	// parts; a sender building its own body could drop it unnoticed.
	case emailprefs.Silenceable(m.Group) && !strings.Contains(m.Text, emailprefs.TextFooter(m.UnsubscribeURL)):
		return fmt.Errorf("emailnotify: %s mail to %s: text part lacks emailprefs.TextFooter: %w", m.Group, m.To, ErrNoWayOut)
	case emailprefs.Silenceable(m.Group) && !strings.Contains(m.HTML, html.EscapeString(m.UnsubscribeURL)):
		return fmt.Errorf("emailnotify: %s mail to %s: HTML part lacks the unsubscribe link: %w", m.Group, m.To, ErrNoWayOut)
	}
	return nil
}

// withSenderName gives a bare From address the product's name. A From that
// already names someone is kept.
func withSenderName(from string) string {
	if a, err := mail.ParseAddress(from); err == nil && a.Name == "" {
		return From(productName, from)
	}
	return from
}

// unsubscribeHeaders is the RFC 8058 pair, or nothing for mail that cannot be
// silenced. Built here so the format has one author. No `mailto:` alternative:
// one-click is what Gmail and Yahoo require, and an advertised address that
// bounces is worse than none.
func (m Message) unsubscribeHeaders() []types.MessageHeader {
	if m.UnsubscribeURL == "" {
		return nil
	}
	return []types.MessageHeader{
		{Name: aws.String("List-Unsubscribe"), Value: aws.String("<" + m.UnsubscribeURL + ">")},
		{Name: aws.String("List-Unsubscribe-Post"), Value: aws.String("List-Unsubscribe=One-Click")},
	}
}

// Client is the AWS SES v2 email transport, a thin adapter over SendEmail.
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

// trackingCheckTimeout bounds CheckEngagementTracking, whose callers have no
// deadline of their own.
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

// NewClient builds a Client from AWS config resolved via the default chain. With
// SMTP_URL set it sends through that relay and SES is never called.
func NewClient(ctx context.Context, region string) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("emailnotify: aws config: %w", err)
	}
	c := &Client{ses: sesv2.NewFromConfig(cfg)}
	// Read here so every worker that builds a Client follows the switch.
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

// input picks the SES shape: a message with attachments goes out as a raw
// RFC 5322 message, everything else uses the simple path.
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
				Body:    &types.Body{Text: &types.Content{Data: aws.String(m.Text)}},
				Headers: m.headers(),
			},
		},
	}
	// An empty HTML part still wins in mail clients, which then show a blank letter.
	if m.HTML != "" {
		in.Content.Simple.Body.Html = &types.Content{Data: aws.String(m.HTML)}
	}
	if m.ReplyTo != "" {
		in.ReplyToAddresses = []string{m.ReplyTo}
	}
	return in, nil
}

// track tags every mail with its group, so a bounce or complaint from the
// identity's default configuration set says which kind of mail drew it. A
// campaign's mail is also routed through the engagement set and tagged with the
// campaign (all internal/engage/emailevents learns about the letter).
func (c *Client) track(in *sesv2.SendEmailInput, m Message) {
	in.EmailTags = []types.MessageTag{{Name: aws.String("group"), Value: aws.String(string(m.Group))}}
	if c.trackingSet == "" || m.Campaign == "" {
		return
	}
	in.ConfigurationSetName = aws.String(c.trackingSet)
	in.EmailTags = append(in.EmailTags, types.MessageTag{Name: aws.String("campaign"), Value: aws.String(m.Campaign)})
	if m.Variant != "" {
		in.EmailTags = append(in.EmailTags, types.MessageTag{Name: aws.String("variant"), Value: aws.String(m.Variant)})
	}
}

func (m Message) headers() []types.MessageHeader {
	headers := m.unsubscribeHeaders()
	if m.NotificationKind != "" {
		headers = append(headers, types.MessageHeader{Name: aws.String("Auto-Submitted"), Value: aws.String("auto-generated")}, types.MessageHeader{Name: aws.String(mailmeta.NotificationHeader), Value: aws.String(m.NotificationKind)})
	}
	return headers
}
