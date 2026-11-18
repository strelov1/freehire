package inbox

import (
	"context"
	"errors"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/mailclassify"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Verdict is one caller's judgement of one message: what it is, and optionally
// which of their applications it belongs to.
type Verdict struct {
	Signal string
	// Slug names the application the message belongs to. Empty means "I am not
	// deciding the link" — never "clear it"; clearing is Unlink.
	Slug string
	// Confidence is the caller's own confidence, stored for display and debugging.
	// It gates nothing: the caller asked for this classification.
	Confidence *float32
}

// Triage records a verdict for one message and advances the linked application's
// stage by the same rules the classification worker uses.
//
// Status, link, provenance and the classified stamp are written together, like
// SetEmailClassification, so a message is never classified-but-unstamped or
// linked-but-unclassified.
//
// An out-of-vocabulary label is REFUSED here, where mailclassify.Sanitize would
// coerce it to `other`: the worker sanitizes untrusted model output, but this
// path carries a judgement the caller asked for and must not rewrite it.
func (s *Service) Triage(ctx context.Context, userID, id int64, v Verdict) (Message, error) {
	if !mailclassify.IsValidSignal(v.Signal) {
		return Message{}, invalid("signal", v.Signal, mailclassify.SignalValues)
	}

	// Resolve the slug before writing: an unknown one changes nothing.
	var jobID pgtype.Int8
	if v.Slug != "" {
		resolved, err := s.q.GetJobIDBySlug(ctx, db.GetJobIDBySlugParams{PublicSlug: v.Slug, CallerID: userID})
		if err != nil {
			return Message{}, err
		}
		jobID = pgtype.Int8{Int64: resolved, Valid: true}
	}

	var confidence pgtype.Float4
	if v.Confidence != nil {
		confidence = pgtype.Float4{Float32: *v.Confidence, Valid: true}
	}
	rows, err := s.q.AgentTriageEmail(ctx, db.AgentTriageEmailParams{
		ID: id, UserID: userID, StatusSignal: pgtype.Text{String: v.Signal, Valid: true},
		JobID: jobID, Confidence: confidence,
	})
	if err != nil {
		return Message{}, err
	}
	if rows == 0 {
		return Message{}, ErrNotFound
	}

	msg, err := s.Get(ctx, userID, id)
	if err != nil {
		return Message{}, err
	}
	// Runs after the read because the stage move is attributed by the message's own store.
	if jobID.Valid {
		s.advanceStage(ctx, userID, jobID.Int64, id, msg.Source, mailclassify.StatusSignal(v.Signal))
	}
	// Triage bypasses mutate, so it reconciles the ledger itself.
	s.syncLedger(ctx, userID, id, msg.Source)
	return msg, nil
}

// Link attaches a message to the application named by slug.
func (s *Service) Link(ctx context.Context, userID, id int64, slug string) (Message, error) {
	if slug == "" {
		return Message{}, ErrSlugRequired
	}
	jobID, err := s.q.GetJobIDBySlug(ctx, db.GetJobIDBySlugParams{PublicSlug: slug, CallerID: userID})
	if err != nil {
		return Message{}, err
	}
	return s.mutate(ctx, userID, id, func() (int64, error) {
		return s.q.LinkEmailToJob(ctx, db.LinkEmailToJobParams{
			ID: id, UserID: userID, JobID: pgtype.Int8{Int64: jobID, Valid: true},
		})
	})
}

// Unlink clears a message's application link, leaving its classification intact.
func (s *Service) Unlink(ctx context.Context, userID, id int64) (Message, error) {
	return s.mutate(ctx, userID, id, func() (int64, error) {
		return s.q.UnlinkEmail(ctx, db.UnlinkEmailParams{ID: id, UserID: userID})
	})
}

// ResolveSuggestion answers a matcher's pending proposal: accept promotes it to a
// confirmed link, reject dismisses it and leaves the message unlinked. Only a
// deterministic match links mail automatically; classifier proposals wait here.
func (s *Service) ResolveSuggestion(ctx context.Context, userID, id int64, accept bool) (Message, error) {
	return s.mutate(ctx, userID, id, func() (int64, error) {
		if accept {
			return s.q.ConfirmEmailLink(ctx, db.ConfirmEmailLinkParams{ID: id, UserID: userID})
		}
		return s.q.RejectEmailLink(ctx, db.RejectEmailLinkParams{ID: id, UserID: userID})
	})
}

// RecordApplication records a tracked application from one of the caller's
// messages and links the message to it in one call, for mail about an
// application that was never recorded.
//
// The application is dated by the message, not by now(): it existed by the time
// the employer wrote. That errs toward under-reporting silence, the safe
// direction: a fabricated ghost tells a person they were ignored when they were not.
//
// Mail still carrying a pending suggestion is refused, so the link's provenance
// is not silently overwritten.
func (s *Service) RecordApplication(ctx context.Context, userID, id int64, slug string) (Message, error) {
	if s.apps == nil {
		return Message{}, ErrUnavailable
	}
	if slug == "" {
		return Message{}, ErrSlugRequired
	}
	email, err := s.q.GetEmail(ctx, db.GetEmailParams{ID: id, UserID: userID})
	if err != nil {
		return Message{}, err
	}
	if email.SuggestedJobID.Valid && !email.JobID.Valid {
		return Message{}, ErrPendingSuggestion
	}
	jobID, err := s.q.GetJobIDBySlug(ctx, db.GetJobIDBySlugParams{PublicSlug: slug, CallerID: userID})
	if err != nil {
		return Message{}, err
	}
	source, err := appevent.SourceForMail(email.Source)
	if err != nil {
		return Message{}, err
	}
	if err := s.apps.MarkAppliedAt(ctx, userID, slug, email.ReceivedAt.Time, source); err != nil {
		return Message{}, err
	}
	return s.mutate(ctx, userID, id, func() (int64, error) {
		return s.q.LinkEmailToJob(ctx, db.LinkEmailToJobParams{
			ID: id, UserID: userID, JobID: pgtype.Int8{Int64: jobID, Valid: true},
		})
	})
}

// mutate runs one link mutation and returns the message it changed. A mutation
// matching no row is a message that is not the caller's — reported as missing,
// never as forbidden.
func (s *Service) mutate(ctx context.Context, userID, id int64, do func() (int64, error)) (Message, error) {
	rows, err := do()
	if err != nil {
		return Message{}, err
	}
	if rows == 0 {
		return Message{}, ErrNotFound
	}
	msg, err := s.Get(ctx, userID, id)
	if err != nil {
		return Message{}, err
	}
	s.syncLedger(ctx, userID, id, msg.Source)
	return msg, nil
}

// syncLedger reconciles the ledger with the message's current link and classification.
//
// Best-effort: the mutation already succeeded, so a failure is logged, not surfaced.
// The reconcile is idempotent; the next mutation or the backfill retries it.
func (s *Service) syncLedger(ctx context.Context, userID, id int64, mailSource string) {
	if err := ReconcileMailEvent(ctx, s.q, userID, id, mailSource); err != nil {
		log.Printf("inbox: ledger sync user=%d email=%d: %v", userID, id, err)
	}
}

// advanceStage moves a linked application forward when the verdict implies
// progress, by the same monotonic-forward rules the classification worker uses.
// Best-effort: the verdict is already durable, so a failed advance does not fail the triage.
//
// emailID and mailSource date and attribute the ledger event: the message is the evidence.
func (s *Service) advanceStage(ctx context.Context, userID, jobID, emailID int64, mailSource string, sig mailclassify.StatusSignal) {
	current, err := s.q.GetUserJobStage(ctx, db.GetUserJobStageParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		// The caller linked mail to a job they do not track: no stage to advance.
		return
	}
	if err != nil {
		// The database failed to answer; log it so the stage does not stall silently.
		log.Printf("inbox: read stage user=%d job=%d: %v", userID, jobID, err)
		return
	}
	next, ok := mailclassify.AdvanceStage(current, sig)
	if !ok {
		return
	}
	source, err := appevent.SourceForMail(mailSource)
	if err != nil {
		// An unrecognised store must not be stamped as an observed one (as in
		// ReconcileMailEvent); without provenance the move cannot be recorded, so it is not made.
		log.Printf("inbox: advance stage user=%d job=%d: %v", userID, jobID, err)
		return
	}
	err = s.q.AdvanceUserJobStage(ctx, db.AdvanceUserJobStageParams{
		UserID: userID, JobID: jobID, Stage: next, EmailID: emailID, EventSource: source,
	})
	if err != nil {
		log.Printf("inbox: advance stage user=%d job=%d: %v", userID, jobID, err)
	}
}

// LabelCount is one classification label and how much mail carries it.
type LabelCount struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Overview is the mailbox's shape, without any content. Labels names EVERY label
// in the vocabulary, including zero counts, so "no interview invitations" differs
// from "no such label".
//
// The labels do not sum to Total: unjudged mail carries none and is counted by Unclassified.
type Overview struct {
	Total        int64        `json:"total"`
	Unread       int64        `json:"unread"`
	Unclassified int64        `json:"unclassified"`
	Linked       int64        `json:"linked"`
	Suggested    int64        `json:"suggested"`
	Unlinked     int64        `json:"unlinked"`
	Labels       []LabelCount `json:"labels"`
}

// Overview reports the shape of the caller's live mail without any of its content.
func (s *Service) Overview(ctx context.Context, userID int64) (Overview, error) {
	rows, err := s.q.CountEmailsByState(ctx, userID)
	if err != nil {
		return Overview{}, err
	}

	counted := make(map[string]int64, len(rows))
	var out Overview
	for _, r := range rows {
		counted[r.Label] = r.N
		out.Total += r.N
		out.Unread += r.Unread
		out.Unclassified += r.Unclassified
		out.Linked += r.Linked
		out.Suggested += r.Suggested
	}
	out.Unlinked = out.Total - out.Linked - out.Suggested

	out.Labels = make([]LabelCount, 0, len(mailclassify.SignalValues))
	for _, label := range mailclassify.SignalValues {
		out.Labels = append(out.Labels, LabelCount{Label: label, Count: counted[label]})
	}
	return out, nil
}

// IsNotFound reports whether err means "the caller cannot address that": this
// package's sentinel or the store's empty result. A row that is not the caller's
// is reported as missing, never as forbidden.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows)
}
