package mailingest

import "context"

// Inbound is one received email as handed off by an InboundSource: the raw MIME
// bytes, the envelope recipients (used to resolve the target mailbox), the S3
// object key (stored on the message and the dedup fallback when no Message-ID),
// and an opaque handle passed back to Ack once the message is durably stored.
type Inbound struct {
	Raw        []byte
	Recipients []string
	S3Key      string
	AckHandle  string
}

// InboundSource is the transport abstraction the worker consumes. Receive returns
// a batch (possibly empty); Ack marks a message done so it is not redelivered.
// Keeping the worker behind this interface makes the transport swappable (SES
// today) and the pipeline testable without AWS.
type InboundSource interface {
	Receive(ctx context.Context) ([]Inbound, error)
	Ack(ctx context.Context, handle string) error
}
