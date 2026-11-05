package emailevents

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SQSSource is the queue SES publishes to through SNS. It is a thin AWS adapter.
type SQSSource struct {
	client   *sqs.Client
	queueURL string
}

// NewSQSSource builds the source from the default AWS credential chain.
func NewSQSSource(ctx context.Context, region, queueURL string) (*SQSSource, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("emailevents: aws config: %w", err)
	}
	return &SQSSource{client: sqs.NewFromConfig(cfg), queueURL: queueURL}, nil
}

// Receive waits briefly for a batch. An empty result ends the drain, so the wait is long
// enough to ride out SQS's sampling of its partitions and short enough for a cron worker.
func (s *SQSSource) Receive(ctx context.Context) ([]Received, error) {
	out, err := s.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            &s.queueURL,
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     2,
	})
	if err != nil {
		return nil, fmt.Errorf("emailevents: sqs receive: %w", err)
	}
	batch := make([]Received, 0, len(out.Messages))
	for _, m := range out.Messages {
		batch = append(batch, Received{Body: *m.Body, AckHandle: *m.ReceiptHandle})
	}
	return batch, nil
}

// Ack deletes a message so it is not redelivered.
func (s *SQSSource) Ack(ctx context.Context, handle string) error {
	if _, err := s.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: &s.queueURL, ReceiptHandle: &handle}); err != nil {
		return fmt.Errorf("emailevents: sqs delete: %w", err)
	}
	return nil
}
