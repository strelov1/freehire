package mailingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SESSource is the production InboundSource: SES receives mail for the domain,
// stores the raw MIME in S3, and notifies SNS → SQS. Receive long-polls SQS,
// reads the S3 object each notification points at, and yields it; Ack deletes the
// SQS message. It is a thin AWS adapter — the parse/resolve/store logic it feeds
// is covered by the worker tests over the fake source.
type SESSource struct {
	sqs      *sqs.Client
	s3       *s3.Client
	queueURL string
	// bucket is the fallback S3 bucket when a notification omits one.
	bucket string
}

// NewSESSource builds the source from AWS config resolved via the default chain
// (SSO / IAM role / env) — credentials never come from app config.
func NewSESSource(ctx context.Context, region, queueURL, bucket string) (*SESSource, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	return &SESSource{
		sqs:      sqs.NewFromConfig(cfg),
		s3:       s3.NewFromConfig(cfg),
		queueURL: queueURL,
		bucket:   bucket,
	}, nil
}

// snsEnvelope is the SNS notification wrapping the SES payload in an SQS message.
type snsEnvelope struct {
	Message string `json:"Message"`
}

// sesNotification is the subset of the SES "Received" notification we need: the
// envelope recipients (mail.destination) and where SES stored the raw MIME
// (receipt.action).
type sesNotification struct {
	Mail struct {
		Destination []string `json:"destination"`
	} `json:"mail"`
	Receipt struct {
		Action struct {
			Type       string `json:"type"`
			BucketName string `json:"bucketName"`
			ObjectKey  string `json:"objectKey"`
		} `json:"action"`
	} `json:"receipt"`
}

// Receive long-polls SQS and turns each notification into an Inbound by fetching
// the referenced S3 object.
func (s *SESSource) Receive(ctx context.Context) ([]Inbound, error) {
	out, err := s.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            &s.queueURL,
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     20,
	})
	if err != nil {
		return nil, fmt.Errorf("sqs receive: %w", err)
	}

	var batch []Inbound
	for _, m := range out.Messages {
		note, err := decodeNotification(*m.Body)
		if err != nil {
			// A notification we can't decode is not retryable; drop it (ack) so it
			// doesn't wedge the queue. The raw is still in S3 if it landed there.
			batch = append(batch, Inbound{AckHandle: *m.ReceiptHandle})
			continue
		}
		bucket := note.Receipt.Action.BucketName
		if bucket == "" {
			bucket = s.bucket
		}
		raw, err := s.fetch(ctx, bucket, note.Receipt.Action.ObjectKey)
		if err != nil {
			// Skip just this message (leave it un-acked so SQS redelivers it after
			// the visibility timeout) rather than discarding the whole batch.
			log.Printf("mailingest: s3 fetch %s: %v", note.Receipt.Action.ObjectKey, err)
			continue
		}
		batch = append(batch, Inbound{
			Raw:        raw,
			Recipients: note.Mail.Destination,
			S3Key:      note.Receipt.Action.ObjectKey,
			AckHandle:  *m.ReceiptHandle,
		})
	}
	return batch, nil
}

// Ack deletes the SQS message so it is not redelivered.
func (s *SESSource) Ack(ctx context.Context, handle string) error {
	_, err := s.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      &s.queueURL,
		ReceiptHandle: &handle,
	})
	return err
}

func (s *SESSource) fetch(ctx context.Context, bucket, key string) ([]byte, error) {
	obj, err := s.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return nil, err
	}
	defer obj.Body.Close()
	return io.ReadAll(obj.Body)
}

// decodeNotification unwraps the SNS envelope and then the SES notification.
func decodeNotification(body string) (sesNotification, error) {
	var env snsEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return sesNotification{}, err
	}
	// SES-over-SNS wraps the notification in a JSON string; a raw SES→SQS setup
	// would deliver it directly, so fall back to the body itself.
	payload := env.Message
	if payload == "" {
		payload = body
	}
	var note sesNotification
	if err := json.Unmarshal([]byte(payload), &note); err != nil {
		return sesNotification{}, err
	}
	return note, nil
}
