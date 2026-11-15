package mailingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SESSource is the production InboundSource: SES stores the raw MIME in S3 and
// notifies SNS → SQS. Receive long-polls SQS and fetches each S3 object; Ack
// deletes the SQS message.
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
// envelope recipients (receipt.recipients) and where SES stored the raw MIME
// (receipt.action).
//
// It deliberately does not decode mail.destination, the recipient list taken from
// the MIME headers. Forwarded mail (a Sieve `redirect`) rewrites the envelope but
// leaves the headers naming the forwarding mailbox, and the headers are the SENDER's
// to write, so resolving a mailbox from them would let anyone deliver into another
// user's inbox by naming them in To:. A field that is not decoded cannot be read
// back by mistake.
type sesNotification struct {
	Receipt struct {
		Recipients []string `json:"recipients"`
		Action     struct {
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
		switch {
		case errors.Is(err, errMessageTooLarge):
			// Not retryable, so acked like an undecodable notification: the object is
			// the same size on every redelivery. The raw MIME stays in S3.
			log.Printf("mailingest: s3 object %s dropped: %v", note.Receipt.Action.ObjectKey, err)
			batch = append(batch, Inbound{AckHandle: *m.ReceiptHandle})
			continue
		case err != nil:
			// Skip just this message (leave it un-acked so SQS redelivers it after
			// the visibility timeout) rather than discarding the whole batch.
			log.Printf("mailingest: s3 fetch %s: %v", note.Receipt.Action.ObjectKey, err)
			continue
		}
		batch = append(batch, Inbound{
			Raw:        raw,
			Recipients: note.Receipt.Recipients,
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
	return readBounded(obj.Body, maxRawMessage)
}

// maxRawMessage caps one raw MIME message held in memory (Receive pulls ten at a
// time, before the recipient is resolved). It is SES's own inbound ceiling, so it
// only rejects a corrupt or planted object, never deliverable mail.
const maxRawMessage = 40 << 20 // 40 MiB

// errMessageTooLarge marks an object over maxRawMessage. It is an error, not a
// truncation: a half-read MIME message still parses and would be stored as whole.
var errMessageTooLarge = errors.New("mailingest: raw message exceeds the size limit")

// readBounded reads up to max bytes from r, refusing anything longer. It reads max+1
// to tell "at the limit" from "over it" without trusting a Content-Length.
func readBounded(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		// The real size is unknown, so the message names the limit.
		return nil, fmt.Errorf("%w: over %d bytes", errMessageTooLarge, max)
	}
	return b, nil
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
