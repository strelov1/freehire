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
// It is SetEmailClassification's sibling, deliberately: status, link, provenance
// and the classified stamp are written together, so a message is never left
// classified-but-unstamped or linked-but-unclassified — states the worker never
// produces and no reader expects.
//
// An out-of-vocabulary label is REFUSED here, where mailclassify.Sanitize would
// coerce it to `other`. The worker sanitizes because it feeds on raw model output
// derived from an attacker-controlled body and must never persist a raw string;
// this path carries a judgement the caller asked for, and silently rewriting it
// would record a verdict nobody chose.
func (s *Service) Triage(ctx context.Context, userID, id int64, v Verdict) (Message, error) {
	if !mailclassify.IsValidSignal(v.Signal) {
		return Message{}, invalid("signal", v.Signal, mailclassify.SignalValues)
	}

	// Resolve the slug before writing: an unknown one changes nothing.
	var jobID pgtype.Int8
	if v.Slug != "" {
		resolved, err := s.q.GetJobIDBySlug(ctx, v.Slug)
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

	if jobID.Valid {
		s.advanceStage(ctx, userID, jobID.Int64, mailclassify.StatusSignal(v.Signal))
	}
	msg, err := s.Get(ctx, userID, id)
	if err != nil {
		return Message{}, err
	}
	// Triage writes status, link and provenance in one update rather than going through
	// mutate, so it needs its own reconcile. It is the same call, deliberately: a triage
	// that set a signal on already-linked mail is an employer reply like any other.
	s.syncLedger(ctx, userID, id, msg.Source)
	return msg, nil
}

// Link attaches a message to the application named by slug.
func (s *Service) Link(ctx context.Context, userID, id int64, slug string) (Message, error) {
	if slug == "" {
		return Message{}, ErrSlugRequired
	}
	jobID, err := s.q.GetJobIDBySlug(ctx, slug)
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
// deterministic match links mail automatically, so everything the classifier
// proposes waits here — a suggestion nobody answers is a link that never happens.
func (s *Service) ResolveSuggestion(ctx context.Context, userID, id int64, accept bool) (Message, error) {
	return s.mutate(ctx, userID, id, func() (int64, error) {
		if accept {
			return s.q.ConfirmEmailLink(ctx, db.ConfirmEmailLinkParams{ID: id, UserID: userID})
		}
		return s.q.RejectEmailLink(ctx, db.RejectEmailLinkParams{ID: id, UserID: userID})
	})
}

// RecordApplication records a tracked application from one of the caller's
// messages and links the message to it in one call. It is the path for mail about
// an application that was never recorded: plain linking needs an application to
// point at, and there is none.
//
// The application is dated by the message, not by now(): it demonstrably existed
// by the time the employer wrote, so the message's timestamp is an honest upper
// bound. The error then leans toward under-reporting elapsed silence, which is the
// safe direction — a missed ghost is a non-event, a fabricated one tells a person
// they were ignored when they were not.
//
// Mail still carrying a pending suggestion is refused: the matcher has already
// proposed an answer, and overwriting it silently would leave the link's
// provenance ambiguous.
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
	jobID, err := s.q.GetJobIDBySlug(ctx, slug)
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
// Every inbox mutation funnels through mutate, so this one call covers confirming a
// suggestion, a manual (re)link, unlinking, and the external-harness triage — the rule
// lives in one place rather than in four.
//
// Best-effort: the mutation the caller asked for has already succeeded and is the
// primary result. A failure here is logged, never surfaced — the reconcile is idempotent
// and the next mutation, or the backfill, will carry it out.
func (s *Service) syncLedger(ctx context.Context, userID, id int64, mailSource string) {
	if err := ReconcileMailEvent(ctx, s.q, userID, id, mailSource); err != nil {
		log.Printf("inbox: ledger sync user=%d email=%d: %v", userID, id, err)
	}
}

// advanceStage moves a linked application forward when the verdict implies
// progress, by the same monotonic-forward rules the classification worker uses. It
// is best-effort: the verdict is already durable, and a failed advance must not
// fail the triage the caller successfully recorded.
func (s *Service) advanceStage(ctx context.Context, userID, jobID int64, sig mailclassify.StatusSignal) {
	current, err := s.q.GetUserJobStage(ctx, db.GetUserJobStageParams{UserID: userID, JobID: jobID})
	if err != nil {
		// No application row means the caller linked mail to a job they do not
		// track; there is simply no stage to advance.
		return
	}
	next, ok := mailclassify.AdvanceStage(current, sig)
	if !ok {
		return
	}
	err = s.q.AdvanceUserJobStage(ctx, db.AdvanceUserJobStageParams{
		UserID: userID, JobID: jobID, Stage: next,
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

// Overview is the mailbox's shape: enough to answer a broad question, and nothing
// a reader would have to pay to keep. Labels names EVERY label the vocabulary has,
// including the ones at zero — "no interview invitations" and "no such label" are
// different answers, and a reader that only saw the non-empty ones could not tell
// them apart.
//
// The labels do not sum to Total: mail nothing has judged yet carries none, and is
// counted by Unclassified.
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

// IsNotFound reports whether err means "the caller cannot address that" — either
// this package's sentinel or the store's empty result. The two are the same answer
// to every reader: a row that is not the caller's is reported as missing, never as
// forbidden. The HTTP handler already maps pgx.ErrNoRows to a 404 centrally; this
// is the same rule for a reader that renders no HTTP status.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows)
}
