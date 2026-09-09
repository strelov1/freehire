package emailnotify

import (
	"context"
	"errors"
	"fmt"

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
	}
	return nil
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
}

// NewClient builds a Client from AWS config resolved via the default chain
// (SSO / IAM role / env) — credentials never come from app config, matching the
// apply service's inbound SES adapter.
func NewClient(ctx context.Context, region string) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("emailnotify: aws config: %w", err)
	}
	return &Client{ses: sesv2.NewFromConfig(cfg)}, nil
}

// Send delivers one message. A send error is returned so the caller (a delivery
// loop) retries and eventually dead-letters rather than dropping the notification.
func (c *Client) Send(ctx context.Context, m Message) error {
	if err := m.validate(); err != nil {
		return err
	}
	in, err := c.input(m)
	if err != nil {
		return err
	}
	if _, err := c.ses.SendEmail(ctx, in); err != nil {
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
