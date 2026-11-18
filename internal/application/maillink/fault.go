package maillink

import (
	"errors"

	"github.com/strelov1/freehire/internal/application/mailclassify"
)

// messageAtFault reports whether the message itself caused err, which decides how the
// failed entry is bounded: the message's own fault spends the attempt budget, anything
// else waits out the grace window (see FailEmailClassification).
//
// It lists only what is about THIS message; the default is "not at fault". Nearly
// every failure here is shared by the whole queue (Postgres, the model gateway), and
// an attempt counter does not measure an outage: a short gateway failure would spend
// all attempts on everything queued. Nothing clears failed_at and the enqueue is ON
// CONFLICT DO NOTHING, so such entries would stay buried. Same shape as
// enrich.postingAtFault.
//
// Two failures are deliberately absent because they are OURS and identical for every
// message, so blaming the message would bury the queue on one bug:
//
//   - a schema build failure (mailclassify's llmschema.Of), a pure function of our types;
//   - an unrecognised emails.source (appevent.SourceForMail). The age window gives two
//     weeks to notice and fix it instead.
func messageAtFault(err error) bool {
	if err == nil {
		return false
	}
	// The model could not produce JSON for this message's content. A retry draws a fresh
	// sample against the same content and generally reproduces it.
	return errors.Is(err, mailclassify.ErrUnparseableResponse)
}
