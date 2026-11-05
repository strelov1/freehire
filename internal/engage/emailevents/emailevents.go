// Package emailevents keeps the ledger of how campaign and digest mail fared: SES publishes
// opens, clicks and delivery outcomes through SNS to an SQS queue, and this package drains
// the queue into email_events and, after 90 days, reduces the rows to daily counts.
package emailevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Retention is how long an event keeps its message id before only daily counts remain.
const Retention = 90 * 24 * time.Hour

// rollUpBatch bounds one retention statement, so a large first pass holds no long lock.
const rollUpBatch = 1000

// maxDrainPerRun bounds one run. A cron run that always catches up on the next tick has no
// need to chase a queue that a large campaign is still filling.
const maxDrainPerRun = 20000

// Received is one queue message.
type Received struct {
	Body      string
	AckHandle string
}

// Source is the queue.
type Source interface {
	// Receive returns the next batch, empty when the queue is drained.
	Receive(ctx context.Context) ([]Received, error)
	Ack(ctx context.Context, handle string) error
}

// Store is the slice of the database the ledger needs.
type Store interface {
	RecordEmailEvent(ctx context.Context, arg db.RecordEmailEventParams) error
	RollUpExpiredEmailEvents(ctx context.Context, arg db.RollUpExpiredEmailEventsParams) (int64, error)
}

// sesEvent is the subset of an SES event record the ledger keeps. mail.destination and
// mail.commonHeaders are deliberately not decoded: a field that is not there cannot end up
// next to what a person opened.
type sesEvent struct {
	EventType string `json:"eventType"`
	Mail      struct {
		MessageID string              `json:"messageId"`
		Timestamp time.Time           `json:"timestamp"`
		Tags      map[string][]string `json:"tags"`
	} `json:"mail"`
	Delivery  occurrence `json:"delivery"`
	Open      occurrence `json:"open"`
	Click     occurrence `json:"click"`
	Bounce    occurrence `json:"bounce"`
	Complaint occurrence `json:"complaint"`
}

type occurrence struct {
	Timestamp time.Time `json:"timestamp"`
}

// errIgnored marks a well-formed record the ledger has no use for.
var errIgnored = errors.New("emailevents: not a tracked event")

// decode unwraps the SNS envelope, when there is one, and reads the SES event in it.
func decode(body string) (db.RecordEmailEventParams, error) {
	var envelope struct {
		Message string `json:"Message"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return db.RecordEmailEventParams{}, fmt.Errorf("emailevents: envelope: %w", err)
	}
	payload := body
	if envelope.Message != "" {
		payload = envelope.Message
	}
	var e sesEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		return db.RecordEmailEventParams{}, fmt.Errorf("emailevents: event: %w", err)
	}
	if e.Mail.MessageID == "" {
		return db.RecordEmailEventParams{}, errors.New("emailevents: event names no message")
	}
	campaign := first(e.Mail.Tags["campaign"])
	if campaign == "" {
		return db.RecordEmailEventParams{}, errIgnored
	}

	var at time.Time
	eventType := strings.ToLower(e.EventType)
	switch eventType {
	case "send":
		at = e.Mail.Timestamp
	case "delivery":
		at = e.Delivery.Timestamp
	case "open":
		at = e.Open.Timestamp
	case "click":
		at = e.Click.Timestamp
	case "bounce":
		at = e.Bounce.Timestamp
	case "complaint":
		at = e.Complaint.Timestamp
	default:
		return db.RecordEmailEventParams{}, errIgnored
	}
	if at.IsZero() {
		at = e.Mail.Timestamp
	}
	return db.RecordEmailEventParams{
		MessageID:  e.Mail.MessageID,
		Type:       eventType,
		Campaign:   campaign,
		Variant:    first(e.Mail.Tags["variant"]),
		OccurredAt: pgtype.Timestamptz{Time: at, Valid: true},
	}, nil
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Stats is what one drain did.
type Stats struct {
	Recorded int
	Ignored  int
	Failed   int
}

// Drain moves the queue into the ledger until it is empty or the per-run bound is reached.
//
// A record that cannot be decoded is acked: it will read the same on every redelivery. A
// failed insert is not, so SQS redelivers it, and it ends the run, because the next write is
// going to the same unavailable database.
func Drain(ctx context.Context, src Source, store Store) (Stats, error) {
	var stats Stats
	for stats.Recorded+stats.Ignored+stats.Failed < maxDrainPerRun {
		batch, err := src.Receive(ctx)
		if err != nil {
			return stats, err
		}
		if len(batch) == 0 {
			return stats, nil
		}
		for _, m := range batch {
			event, err := decode(m.Body)
			switch {
			case errors.Is(err, errIgnored):
				stats.Ignored++
			case err != nil:
				log.Printf("emailevents: dropping a record: %v", err)
				stats.Failed++
			default:
				if err := store.RecordEmailEvent(ctx, event); err != nil {
					return stats, fmt.Errorf("emailevents: recording %s: %w", event.Type, err)
				}
				stats.Recorded++
			}
			if err := src.Ack(ctx, m.AckHandle); err != nil {
				return stats, err
			}
		}
	}
	return stats, nil
}

// RollUp reduces every event past retention to daily counts, one bounded batch at a time.
func RollUp(ctx context.Context, store Store, now time.Time) error {
	for {
		rows, err := store.RollUpExpiredEmailEvents(ctx, db.RollUpExpiredEmailEventsParams{
			Before:  pgtype.Timestamptz{Time: now.Add(-Retention), Valid: true},
			MaxRows: rollUpBatch,
		})
		if err != nil {
			return fmt.Errorf("emailevents: rolling up: %w", err)
		}
		if rows == 0 {
			return nil
		}
	}
}
