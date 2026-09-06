package maillink

import (
	"errors"

	"github.com/strelov1/freehire/internal/application/mailclassify"
)

// messageAtFault reports whether the message itself caused err, which decides how the
// failed entry is bounded: the message's own fault spends the attempt budget, anything
// else waits out the grace window (see FailEmailClassification).
//
// It enumerates what is genuinely about THIS message, and the default is deliberately on
// the other side. Almost everything that fails here is shared by every message in the
// queue — Postgres (Applications, ThreadLinks, CurrentStage, Save) and the model gateway
// (Classify) — and an attempt counter does not measure an outage: the lease makes a
// claimed entry re-claimable minutes later, so a fifteen-minute gateway failure spends
// all three attempts on everything then in the queue. Nothing in the repository clears
// failed_at, and EnqueuePendingEmailClassification is ON CONFLICT DO NOTHING, so those
// entries stay buried: mail unclassified, unlinked, stages unmoved, recoverable only by
// hand. That is the same shape enrich.postingAtFault was written for after two LiteLLM
// outages permanently dead-lettered 172,875 postings, and the same shape as the 2726
// messages this queue lost to an unset Claimed.Source.
//
// Two failures that look like candidates are deliberately absent, and both for the same
// reason — they are OURS, identical for every message, so blaming the message would bury
// the whole queue on one bug:
//
//   - a schema build failure (mailclassify's llmschema.Of), which is a pure function of
//     our own types and either always works or never does;
//   - an unrecognised emails.source (appevent.SourceForMail). The one time this fired in
//     production the column was fine and the store's own mapping had dropped the field,
//     which is exactly the case the attempt ceiling must not punish the mail for. The age
//     window gives two weeks to notice and fix it instead.
func messageAtFault(err error) bool {
	if err == nil {
		return false
	}
	// The model could not produce JSON for this message's content. A retry draws a fresh
	// sample against the same content and generally reproduces it.
	return errors.Is(err, mailclassify.ErrUnparseableResponse)
}
