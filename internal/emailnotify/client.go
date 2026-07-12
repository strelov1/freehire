package emailnotify

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// Compile-time guarantee that Client is a Sender.
var _ Sender = (*Client)(nil)

// sesAPI is the slice of the SES v2 client the Client uses, so tests inject a fake
// in its place. *sesv2.Client satisfies it.
type sesAPI interface {
	SendEmail(ctx context.Context, in *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

// Client is the AWS SES v2 email transport. It is a thin adapter over SendEmail —
// the render logic it serves lives in Notifier and is covered by the render tests.
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

// Send delivers one email with both an HTML and a plain-text body via SES
// SendEmail. A send error is returned so the caller (the notify delivery loop)
// retries and eventually dead-letters rather than dropping the notification.
func (c *Client) Send(ctx context.Context, from, to, subject, htmlBody, textBody string) error {
	_, err := c.ses.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(from),
		Destination:      &types.Destination{ToAddresses: []string{to}},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String(subject)},
				Body: &types.Body{
					Html: &types.Content{Data: aws.String(htmlBody)},
					Text: &types.Content{Data: aws.String(textBody)},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("emailnotify: ses send to %s: %w", to, err)
	}
	return nil
}
