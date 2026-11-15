package assistant

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Presets a session can run under. The preset selects the system prompt and the registered
// tools, nothing else. The vocabulary is also a CHECK on the column, so a new preset is a
// migration and not only a constant.
const (
	PresetChat   = "chat"
	PresetTailor = "tailor"
	// PresetProfile is the experience interviewer: the same tools as a chat session, and a
	// prompt that goes looking for what the bank does not yet know.
	PresetProfile = "profile"
	// PresetBrowse is a conversation held from the browser extension's side panel. Its agent
	// can see the page only when the turn authenticated as the extension's own Bearer session
	// JWT; reached any other way it runs as a plain chat. See internal/api/handler's
	// effectivePreset and internal/ai/assistant/AGENTS.md.
	PresetBrowse = "browse"
	// PresetInterview is the mock interview held against one application. Bound to a vacancy
	// but no CV: it reads the CV and fit analysis and edits neither.
	PresetInterview = "interview"
	// PresetDebrief reviews an interview that has already happened, bound to the same
	// application as a rehearsal and sharing its tools and context. Its prompt inverts one rule:
	// a rehearsal guards against banking an improvisation, a debrief banks a recollection.
	PresetDebrief = "debrief"
	// PresetApply is the conversation held against one automatic application: it narrates the
	// attempt's progress and takes the candidate's answers in free text. Bound to a vacancy and
	// no CV, like a rehearsal.
	PresetApply = "apply"
	// PresetBase is the conversation held against one BASE CV (the general CV tailored copies
	// start from): bound to a CV and no vacancy. It runs the tailoring workspace's CV tools
	// under a prompt that edits the general CV, and registers none of the vacancy tools.
	PresetBase = "base"
	// PresetSupport is first-line support: no CV, no vacancy, answers from a fixed FAQ, reads
	// the caller's plan through `my_account` and hands off via `escalate_to_human`. Reached only
	// through the dedicated /support routes, never the generic session ones.
	PresetSupport = "support"
)

// ErrNotFound is returned for a session the caller does not own and for one that does not
// exist. They are indistinguishable so a caller cannot probe for other users' session ids.
var ErrNotFound = errors.New("assistant: session not found")

// SearchContext keeps a fixed search and its local selection preferences outside model history.
type SearchContext struct {
	Query string `json:"query"`
	Brief string `json:"brief"`
}

// Session is one conversation, owner-scoped. Its id is a random UUID so ids do not publish
// how many conversations exist. CVID and JobID are set only for a bound session; nil
// pointers mean unbound.
type Session struct {
	SearchContext *SearchContext
	ID            uuid.UUID
	UserID        int64
	Preset        string
	Label         string
	CVID          *uuid.UUID
	JobID         *int64
}

// Queries is the slice of the generated query surface the store needs.
// *db.Queries satisfies it; tests supply a fake.
type Queries interface {
	CreateAssistantSession(ctx context.Context, arg db.CreateAssistantSessionParams) (db.CreateAssistantSessionRow, error)
	ListAssistantChatSessions(ctx context.Context, userID int64) ([]db.ListAssistantChatSessionsRow, error)
	GetAssistantSession(ctx context.Context, arg db.GetAssistantSessionParams) (db.GetAssistantSessionRow, error)
	GetAssistantSessionForJob(ctx context.Context, arg db.GetAssistantSessionForJobParams) (db.GetAssistantSessionForJobRow, error)
	GetEmptyAssistantSession(ctx context.Context, arg db.GetEmptyAssistantSessionParams) (db.GetEmptyAssistantSessionRow, error)
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
	return session(row.ID, row.UserID, row.Preset, row.Label, row.CvID, row.JobID, row.SearchQuery, row.SelectionBrief), nil
}

// ChatSessions lists the caller's general chats, most recently active first. Bound
// conversations are excluded; they open from their own workspace.
func (s *Store) ChatSessions(ctx context.Context, userID int64) ([]Session, error) {
	rows, err := s.q.ListAssistantChatSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assistant: list sessions: %w", err)
	}
	out := make([]Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, session(r.ID, r.UserID, r.Preset, r.Label, r.CvID, r.JobID, r.SearchQuery, r.SelectionBrief))
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
	return session(row.ID, row.UserID, row.Preset, row.Label, row.CvID, row.JobID, row.SearchQuery, row.SelectionBrief), nil
}

// SessionForJob returns the caller's most recent session of preset bound to jobID, and
// false when there is none, so reopening an application resumes instead of starting another.
func (s *Store) SessionForJob(ctx context.Context, userID int64, preset string, jobID int64) (Session, bool, error) {
	row, err := s.q.GetAssistantSessionForJob(ctx, db.GetAssistantSessionForJobParams{UserID: userID, Preset: preset, JobID: &jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("assistant: get session for job: %w", err)
	}
	return session(row.ID, row.UserID, row.Preset, row.Label, row.CvID, row.JobID, row.SearchQuery, row.SelectionBrief), true, nil
}

// EmptySession returns the caller's newest session of preset with no user message, and
// false when there is none, so "new conversation" can reuse the blank one.
func (s *Store) EmptySession(ctx context.Context, userID int64, preset string) (Session, bool, error) {
	row, err := s.q.GetEmptyAssistantSession(ctx, db.GetEmptyAssistantSessionParams{UserID: userID, Preset: preset})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("assistant: get empty session: %w", err)
	}
	return session(row.ID, row.UserID, row.Preset, row.Label, row.CvID, row.JobID, row.SearchQuery, row.SelectionBrief), true, nil
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

// Touch marks a session as the most recently active. Owner-scoped: userID must be the
// caller who already proved ownership of id.
func (s *Store) Touch(ctx context.Context, id uuid.UUID, userID int64) error {
	if err := s.q.TouchAssistantSession(ctx, db.TouchAssistantSessionParams{ID: id, UserID: userID}); err != nil {
		return fmt.Errorf("assistant: touch session: %w", err)
	}
	return nil
}

// LabelSession names a session from its first user message. The query applies it only
// while the label is unset, so it is safe on every turn. Owner-scoped like Touch.
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

// appendAttempts bounds the seq-collision retry below.
const appendAttempts = 3

// Append writes one message to a session's transcript and returns it with the sequence
// number the database assigned.
//
// The next seq is computed inside the INSERT, so two turns racing in one session (chat open
// in two tabs) can pick the same one and the primary key rejects the loser. Retrying
// re-reads max(seq); giving up would drop a message, and a vanished tool result leaves a
// call nothing answers, which providers reject.
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

// Transcript reads a session's WHOLE messages in order, for a client replaying the
// conversation. The model's history uses RecentTranscript instead.
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

// RecentTranscript reads a session's most recent limit messages, oldest first: the bounded
// counterpart of Transcript for rebuilding the model's history. limit must be positive.
func (s *Store) RecentTranscript(ctx context.Context, sessionID uuid.UUID, limit int) ([]Message, error) {
	rows, err := s.q.ListRecentAssistantMessages(ctx, db.ListRecentAssistantMessagesParams{
		SessionID: sessionID,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: read recent transcript: %w", err)
	}
	// The query orders newest first so LIMIT keeps the tail; reverse to ascending seq.
	out := make([]Message, len(rows))
	for i, r := range rows {
		out[len(rows)-1-i] = Message{Seq: r.Seq, Role: r.Role, Content: r.Content}
	}
	return out, nil
}

// session builds the domain shape from the columns every session query selects. sqlc
// emits a distinct row type per query, so the mapping lives here once.
func session(id uuid.UUID, userID int64, preset string, label pgtype.Text, cvID *uuid.UUID, jobID *int64, query pgtype.Text, brief string) Session {
	result := Session{ID: id, UserID: userID, Preset: preset, Label: label.String, CVID: cvID, JobID: jobID}
	if query.Valid {
		result.SearchContext = &SearchContext{Query: query.String, Brief: brief}
	}
	return result
}
