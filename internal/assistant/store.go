package assistant

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgerr"
)

// Presets a session can run under. The preset selects the system prompt and the
// registered tools, and nothing else — one chat surface serves all of them. The
// vocabulary is also a CHECK on the column (0044, widened by 0048, 0049 and 0062), so
// a new preset is a migration and not only a constant.
const (
	PresetChat   = "chat"
	PresetTailor = "tailor"
	// PresetProfile is the experience interviewer: the same tools as a chat session, and a
	// prompt that goes looking for what the bank does not yet know.
	PresetProfile = "profile"
	// PresetBrowse is a conversation meant to be held from the browser extension's
	// side panel. It is the only preset whose agent can EVER see the page the
	// candidate is on — but only when the turn itself authenticated as the
	// extension's own Bearer session JWT; reached any other way (a stale rail entry,
	// an API key) it runs as a plain chat instead. See internal/handler's
	// effectivePreset and internal/assistant/AGENTS.md.
	PresetBrowse = "browse"
	// PresetInterview is the mock interview held against one application. Like a
	// tailoring session it is bound to a vacancy, but to no CV: it reads the CV and
	// the fit analysis and edits neither.
	PresetInterview = "interview"
	// PresetDebrief reviews an interview that has already happened, bound to the same
	// application a rehearsal is. It shares the rehearsal's tools and context; what
	// differs is the prompt, and one rule inside it that inverts — a rehearsal guards
	// against banking an improvisation, a debrief exists to bank a recollection.
	PresetDebrief = "debrief"
)

// ErrNotFound is returned for a session the caller does not own and for one that
// does not exist. The two are deliberately indistinguishable: telling them apart
// would let a caller probe for other users' session ids.
var ErrNotFound = errors.New("assistant: session not found")

// Session is one conversation, owner-scoped. Its id is a random UUID rather than a
// sequence: ownership already confines every read, but a countable id would still
// publish how many conversations exist and would make one forgotten owner check
// enumerable. CVID and JobID are set only for a tailoring session; nil pointers
// say "unbound" without leaking pgtype into callers.
type Session struct {
	ID     uuid.UUID
	UserID int64
	Preset string
	Label  string
	CVID   *uuid.UUID
	JobID  *int64
}

// Queries is the slice of the generated query surface the store needs.
// *db.Queries satisfies it; tests supply a fake.
type Queries interface {
	CreateAssistantSession(ctx context.Context, arg db.CreateAssistantSessionParams) (db.CreateAssistantSessionRow, error)
	ListAssistantChatSessions(ctx context.Context, userID int64) ([]db.ListAssistantChatSessionsRow, error)
	GetAssistantSession(ctx context.Context, arg db.GetAssistantSessionParams) (db.GetAssistantSessionRow, error)
	DeleteAssistantSession(ctx context.Context, arg db.DeleteAssistantSessionParams) (int64, error)
	TouchAssistantSession(ctx context.Context, arg db.TouchAssistantSessionParams) error
	SetAssistantSessionLabel(ctx context.Context, arg db.SetAssistantSessionLabelParams) error
	AppendAssistantMessage(ctx context.Context, arg db.AppendAssistantMessageParams) (db.AssistantMessage, error)
	ListAssistantMessages(ctx context.Context, sessionID uuid.UUID) ([]db.AssistantMessage, error)
	ListRecentAssistantMessages(ctx context.Context, arg db.ListRecentAssistantMessagesParams) ([]db.AssistantMessage, error)
}

// Store persists sessions and their transcripts. Every session read and write is
// scoped to an owner, so ownership needs no separate enforcement layer.
type Store struct{ q Queries }

// NewStore wraps the query surface.
func NewStore(q Queries) *Store { return &Store{q: q} }

// CreateSession starts a conversation. cvID/jobID bind a tailoring session to its
// CV and vacancy and are nil for a chat.
func (s *Store) CreateSession(ctx context.Context, userID int64, preset string, cvID *uuid.UUID, jobID *int64) (Session, error) {
	row, err := s.q.CreateAssistantSession(ctx, db.CreateAssistantSessionParams{
		UserID: userID,
		Preset: preset,
		CvID:   cvID,
		JobID:  jobID,
	})
	if err != nil {
		return Session{}, fmt.Errorf("assistant: create session: %w", err)
	}
	return session(row.ID, row.UserID, row.Preset, row.Label, row.CvID, row.JobID), nil
}

// ChatSessions lists the caller's general chats, most recently active first.
// Tailoring conversations are excluded: each belongs to a CV and is opened from
// the tailoring workspace, so it has no place in the chat rail.
func (s *Store) ChatSessions(ctx context.Context, userID int64) ([]Session, error) {
	rows, err := s.q.ListAssistantChatSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assistant: list sessions: %w", err)
	}
	out := make([]Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, session(r.ID, r.UserID, r.Preset, r.Label, r.CvID, r.JobID))
	}
	return out, nil
}

// Session reads one conversation the caller owns.
func (s *Store) Session(ctx context.Context, id uuid.UUID, userID int64) (Session, error) {
	row, err := s.q.GetAssistantSession(ctx, db.GetAssistantSessionParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("assistant: get session: %w", err)
	}
	return session(row.ID, row.UserID, row.Preset, row.Label, row.CvID, row.JobID), nil
}

// DeleteSession removes an owned conversation and its transcript.
func (s *Store) DeleteSession(ctx context.Context, id uuid.UUID, userID int64) error {
	n, err := s.q.DeleteAssistantSession(ctx, db.DeleteAssistantSessionParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("assistant: delete session: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Touch marks a session as the most recently active, so the rail follows real use.
// Owner-scoped like every other session write: userID must be the caller who already
// proved ownership of id (the Session it just read, created, or continued).
func (s *Store) Touch(ctx context.Context, id uuid.UUID, userID int64) error {
	if err := s.q.TouchAssistantSession(ctx, db.TouchAssistantSessionParams{ID: id, UserID: userID}); err != nil {
		return fmt.Errorf("assistant: touch session: %w", err)
	}
	return nil
}

// LabelSession names a session from its first user message. The query applies it
// only while the label is unset, so this is safe to call on every turn. Owner-scoped
// like Touch.
func (s *Store) LabelSession(ctx context.Context, id uuid.UUID, userID int64, label string) error {
	err := s.q.SetAssistantSessionLabel(ctx, db.SetAssistantSessionLabelParams{
		ID:     id,
		UserID: userID,
		Label:  pgtype.Text{String: label, Valid: label != ""},
	})
	if err != nil {
		return fmt.Errorf("assistant: label session: %w", err)
	}
	return nil
}

// appendAttempts bounds the retry below. A collision needs one more read of
// max(seq) to clear; more than a couple in a row means something else is wrong.
const appendAttempts = 3

// Append writes one message to a session's transcript and returns it with the
// sequence number the database assigned.
//
// The next seq is computed inside the INSERT, so two turns racing in the same
// session — the chat open in two tabs — can pick the same one and the primary key
// rejects the loser. Retrying re-reads max(seq), which is what the loser needed;
// giving up instead would drop a message out of the model's history, and a tool
// result that vanishes leaves a call nothing answers, which providers reject.
func (s *Store) Append(ctx context.Context, sessionID uuid.UUID, m Message) (Message, error) {
	var err error
	for attempt := 0; attempt < appendAttempts; attempt++ {
		var row db.AssistantMessage
		row, err = s.q.AppendAssistantMessage(ctx, db.AppendAssistantMessageParams{
			SessionID: sessionID,
			Role:      m.Role,
			Content:   m.Content,
		})
		if err == nil {
			return Message{Seq: row.Seq, Role: row.Role, Content: row.Content}, nil
		}
		if !pgerr.IsUniqueViolation(err) {
			break
		}
	}
	return Message{}, fmt.Errorf("assistant: append message: %w", err)
}

// Transcript reads a session's WHOLE messages in order. For a client replaying the
// conversation; the model's own history is rebuilt from RecentTranscript instead, which
// does not pay for rows that would only be trimmed away.
func (s *Store) Transcript(ctx context.Context, sessionID uuid.UUID) ([]Message, error) {
	rows, err := s.q.ListAssistantMessages(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("assistant: read transcript: %w", err)
	}
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, Message{Seq: r.Seq, Role: r.Role, Content: r.Content})
	}
	return out, nil
}

// RecentTranscript reads a session's most recent limit messages, oldest first — the
// bounded counterpart of Transcript, for rebuilding the model's own history every turn
// without fetching and JSON-decoding rows that Runner.trim() would only discard. limit
// must be positive.
func (s *Store) RecentTranscript(ctx context.Context, sessionID uuid.UUID, limit int) ([]Message, error) {
	rows, err := s.q.ListRecentAssistantMessages(ctx, db.ListRecentAssistantMessagesParams{
		SessionID: sessionID,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: read recent transcript: %w", err)
	}
	// The query orders newest first so LIMIT keeps the tail; reverse back to ascending
	// seq order, the shape every caller (trim, Conversation) expects.
	out := make([]Message, len(rows))
	for i, r := range rows {
		out[len(rows)-1-i] = Message{Seq: r.Seq, Role: r.Role, Content: r.Content}
	}
	return out, nil
}

// session builds the domain shape from the columns every session query selects.
// sqlc emits a distinct row type per query — same columns, three names — so the
// mapping lives here once and each caller adapts its row into it.
func session(id uuid.UUID, userID int64, preset string, label pgtype.Text, cvID *uuid.UUID, jobID *int64) Session {
	return Session{ID: id, UserID: userID, Preset: preset, Label: label.String, CVID: cvID, JobID: jobID}
}
