package mailingest

import (
	"context"
	"log"
	"net/mail"
	"strings"
	"time"
)

// pollInterval is how long Run waits after an empty receive before polling again.
const pollInterval = 5 * time.Second

// opTimeout bounds the store+ack of one already-received message, on a context
// detached from the receive context so a pulled message is still persisted during
// shutdown, while a stuck DB cannot hang shutdown forever.
const opTimeout = 30 * time.Second

// HostedMessage is a received message ready to store, keyed by (user, external id).
type HostedMessage struct {
	UserID     int64
	ExternalID string
	S3Key      string
	FromAddr   string
	FromName   string
	Subject    string
	BodyText   string
	BodyHTML   string
	ReceivedAt time.Time
}

// Store is the db-free persistence the worker needs (faked in tests). A db-backed
// adapter maps these to the unified mail-store queries.
type Store interface {
	// MailboxByAddress resolves a recipient address to its owning user; ok=false
	// when no mailbox holds that address.
	MailboxByAddress(ctx context.Context, address string) (userID int64, ok bool, err error)
	// InsertMessage stores a hosted message, idempotent by (user, external id).
	InsertMessage(ctx context.Context, m HostedMessage) error
}

// Worker drains an InboundSource: for each message it parses the MIME, resolves
// the recipient to a mailbox, stores it idempotently, and acks. Mail that fails
// to parse or targets an unknown mailbox is logged and acked (the raw stays in
// S3); a store error is left un-acked so the transport redelivers it.
type Worker struct {
	src    InboundSource
	store  Store
	domain string
}

// NewWorker builds a worker over a source and store for the given mail domain.
func NewWorker(src InboundSource, store Store, domain string) *Worker {
	return &Worker{src: src, store: store, domain: domain}
}

// Run polls the source until ctx is cancelled. Returns when ctx is done so it can
// be driven by the server's signal context.
func (w *Worker) Run(ctx context.Context) {
	for {
		if err := w.RunOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("mailingest: receive batch: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}

// RunOnce processes a single received batch — the deterministic unit the tests drive.
func (w *Worker) RunOnce(ctx context.Context) error {
	batch, err := w.src.Receive(ctx)
	if err != nil {
		return err
	}
	for _, in := range batch {
		// Detach store+ack from the receive context so a message already pulled from
		// the source is persisted even during shutdown; the timeout bounds a stuck DB.
		opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opTimeout)
		if err := w.handle(opCtx, in); err != nil {
			// Transient/store error: do NOT ack, let the transport redeliver.
			log.Printf("mailingest: handle message (%s): %v", in.S3Key, err)
			cancel()
			continue
		}
		if err := w.src.Ack(opCtx, in.AckHandle); err != nil {
			log.Printf("mailingest: ack %s: %v", in.AckHandle, err)
		}
		cancel()
	}
	return nil
}

// handle stores one inbound message. It returns nil (so the caller acks) for
// messages it deliberately drops — an unparseable body or an unknown recipient —
// since redelivering them would never succeed; the raw MIME remains in S3.
func (w *Worker) handle(ctx context.Context, in Inbound) error {
	parsed, err := Parse(in.Raw)
	if err != nil {
		log.Printf("mailingest: unparseable message %s dropped: %v", in.S3Key, err)
		return nil
	}

	recipient, ok := pickDomainRecipient(in.Recipients, w.domain)
	if !ok {
		log.Printf("mailingest: message %s has no %s recipient, dropped", in.S3Key, w.domain)
		return nil
	}

	userID, ok, err := w.store.MailboxByAddress(ctx, recipient)
	if err != nil {
		return err
	}
	if !ok {
		log.Printf("mailingest: unknown recipient %s, dropped", recipient)
		return nil
	}

	externalID := parsed.MessageID
	if externalID == "" {
		// No Message-ID: dedup on the stable object key instead.
		externalID = "s3:" + in.S3Key
	}
	receivedAt := parsed.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}

	return w.store.InsertMessage(ctx, HostedMessage{
		UserID:     userID,
		ExternalID: externalID,
		S3Key:      in.S3Key,
		FromAddr:   parsed.FromAddr,
		FromName:   parsed.FromName,
		Subject:    parsed.Subject,
		BodyText:   parsed.TextBody,
		BodyHTML:   parsed.HTMLBody,
		ReceivedAt: receivedAt,
	})
}

// pickDomainRecipient returns the first recipient on our mail domain, lowercased.
// A message may be addressed to several recipients (To/Cc); only the one on our
// domain identifies the target mailbox. The result is lowercased because
// addresses are always allocated lowercase (mailbox.Handle), while the SMTP
// envelope recipient carries whatever case the sender used — matching them
// case-sensitively would silently drop mail to e.g. "Ivan@..." vs "ivan@...".
func pickDomainRecipient(recipients []string, domain string) (string, bool) {
	domain = strings.ToLower(domain)
	for _, r := range recipients {
		addr := r
		if parsed, err := mail.ParseAddress(r); err == nil {
			addr = parsed.Address
		}
		addr = strings.ToLower(addr)
		if at := strings.LastIndexByte(addr, '@'); at >= 0 {
			if addr[at+1:] == domain {
				return addr, true
			}
		}
	}
	return "", false
}
