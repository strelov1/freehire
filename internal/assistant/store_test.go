package assistant

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// Fixed ids so a failure names a stable value rather than a fresh random one.
var (
	sessionID = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	otherID   = uuid.MustParse("22222222-2222-4222-8222-222222222222")
)

// fakeQueries is an in-memory stand-in for the generated queries, holding one
// session and its transcript. It records what it was asked for so the tests can
// assert the store scopes every read to the caller.
type fakeQueries struct {
	session  db.AssistantSession
	messages []db.AssistantMessage
	deleted  int64

	gotUserID int64
	labelSet  string
	touched   uuid.UUID
}

func (f *fakeQueries) CreateAssistantSession(_ context.Context, arg db.CreateAssistantSessionParams) (db.AssistantSession, error) {
	return db.AssistantSession{
		ID: sessionID, UserID: arg.UserID, Preset: arg.Preset, CvID: arg.CvID, JobID: arg.JobID,
	}, nil
}

func (f *fakeQueries) ListAssistantChatSessions(_ context.Context, userID int64) ([]db.AssistantSession, error) {
	f.gotUserID = userID
	if f.session.UserID != userID || f.session.Preset != PresetChat {
		return nil, nil
	}
	return []db.AssistantSession{{
		ID: f.session.ID, UserID: f.session.UserID, Preset: f.session.Preset,
		Label: f.session.Label, CvID: f.session.CvID, JobID: f.session.JobID,
	}}, nil
}

func (f *fakeQueries) GetAssistantSession(_ context.Context, arg db.GetAssistantSessionParams) (db.AssistantSession, error) {
	f.gotUserID = arg.UserID
	if f.session.ID != arg.ID || f.session.UserID != arg.UserID {
		return db.AssistantSession{}, pgx.ErrNoRows
	}
	return db.AssistantSession{
		ID: f.session.ID, UserID: f.session.UserID, Preset: f.session.Preset,
		Label: f.session.Label, CvID: f.session.CvID, JobID: f.session.JobID,
	}, nil
}

func (f *fakeQueries) DeleteAssistantSession(_ context.Context, _ db.DeleteAssistantSessionParams) (int64, error) {
	return f.deleted, nil
}

func (f *fakeQueries) TouchAssistantSession(_ context.Context, id uuid.UUID) error {
	f.touched = id
	return nil
}

func (f *fakeQueries) SetAssistantSessionLabel(_ context.Context, arg db.SetAssistantSessionLabelParams) error {
	f.labelSet = arg.Label.String
	return nil
}

func (f *fakeQueries) AppendAssistantMessage(_ context.Context, arg db.AppendAssistantMessageParams) (db.AssistantMessage, error) {
	seq := int32(len(f.messages) + 1)
	f.messages = append(f.messages, db.AssistantMessage{
		SessionID: arg.SessionID, Seq: seq, Role: arg.Role, Content: arg.Content,
	})
	return db.AssistantMessage{SessionID: arg.SessionID, Seq: seq, Role: arg.Role, Content: arg.Content}, nil
}

func (f *fakeQueries) ListAssistantMessages(_ context.Context, sessionID uuid.UUID) ([]db.AssistantMessage, error) {
	var out []db.AssistantMessage
	for _, m := range f.messages {
		if m.SessionID == sessionID {
			out = append(out, db.AssistantMessage{SessionID: m.SessionID, Seq: m.Seq, Role: m.Role, Content: m.Content})
		}
	}
	return out, nil
}

func TestGetSessionMapsNullableColumns(t *testing.T) {
	f := &fakeQueries{session: db.AssistantSession{
		ID: sessionID, UserID: 3, Preset: PresetTailor,
		Label: pgtype.Text{String: "Tailor for Acme", Valid: true},
		CvID:  ptr(int64(42)),
		JobID: nil, // never set
	}}
	s := NewStore(f)

	got, err := s.Session(context.Background(), sessionID, 3)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if got.Preset != PresetTailor || got.Label != "Tailor for Acme" {
		t.Errorf("session = %+v, want the stored preset and label", got)
	}
	if got.CVID == nil || *got.CVID != 42 {
		t.Errorf("CVID = %v, want 42", got.CVID)
	}
	if got.JobID != nil {
		t.Errorf("JobID = %v, want nil for an unset column", got.JobID)
	}
}

func TestSessionOfAnotherUserIsNotFound(t *testing.T) {
	f := &fakeQueries{session: db.AssistantSession{ID: sessionID, UserID: 3, Preset: PresetChat}}
	s := NewStore(f)

	_, err := s.Session(context.Background(), sessionID, 99)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for a session owned by someone else", err)
	}
}

func TestDeleteSessionThatAffectedNoRowIsNotFound(t *testing.T) {
	s := NewStore(&fakeQueries{deleted: 0})
	if err := s.DeleteSession(context.Background(), sessionID, 3); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound when the delete matched nothing", err)
	}
}

func TestDeleteOwnedSessionSucceeds(t *testing.T) {
	s := NewStore(&fakeQueries{deleted: 1})
	if err := s.DeleteSession(context.Background(), sessionID, 3); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
}

func TestAppendAndReadTranscript(t *testing.T) {
	f := &fakeQueries{}
	s := NewStore(f)
	ctx := context.Background()

	user, _ := EncodeUser("hi")
	if _, err := s.Append(ctx, sessionID, user); err != nil {
		t.Fatalf("Append: %v", err)
	}
	answer, _ := EncodeAssistant("hello", nil)
	if _, err := s.Append(ctx, sessionID, answer); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := s.Transcript(ctx, sessionID)
	if err != nil {
		t.Fatalf("Transcript: %v", err)
	}
	if len(got) != 2 || got[0].Role != RoleUser || got[1].Role != RoleAssistant {
		t.Fatalf("transcript = %+v, want the two messages in order", got)
	}
	if got[0].Seq != 1 || got[1].Seq != 2 {
		t.Errorf("sequence numbers = %d,%d, want 1,2", got[0].Seq, got[1].Seq)
	}

	history, err := Conversation(got)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("history has %d messages, want 2", len(history))
	}
}

func TestLabelSessionUsesTheFirstMessage(t *testing.T) {
	f := &fakeQueries{}
	s := NewStore(f)
	if err := s.LabelSession(context.Background(), sessionID, "find go jobs"); err != nil {
		t.Fatalf("LabelSession: %v", err)
	}
	if f.labelSet != "find go jobs" {
		t.Errorf("label = %q, want the first user message", f.labelSet)
	}
}

func TestCreateSessionPassesTheBinding(t *testing.T) {
	f := &fakeQueries{}
	s := NewStore(f)
	cv := int64(42)
	job := int64(9)

	got, err := s.CreateSession(context.Background(), 3, PresetTailor, &cv, &job)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if got.Preset != PresetTailor || got.CVID == nil || *got.CVID != 42 || got.JobID == nil || *got.JobID != 9 {
		t.Errorf("session = %+v, want a tailoring session bound to cv 42 / job 9", got)
	}
}

func TestTranscriptReturnsTheStoredBytesUnchanged(t *testing.T) {
	f := &fakeQueries{}
	s := NewStore(f)
	ctx := context.Background()
	stored, _ := EncodeToolResult("c1", "facets", `{"skills":["go"]}`)
	if _, err := s.Append(ctx, sessionID, stored); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := s.Transcript(ctx, sessionID)
	if err != nil {
		t.Fatalf("Transcript: %v", err)
	}
	if string(got[0].Content) != string(stored.Content) {
		t.Errorf("content = %s, want the bytes as appended", got[0].Content)
	}
}

func TestChatSessionsExcludesTailoringConversations(t *testing.T) {
	// A tailoring conversation belongs to a CV and is opened from the tailoring
	// workspace; listing it in the chat rail would offer a chat that leads nowhere.
	f := &fakeQueries{session: db.AssistantSession{ID: sessionID, UserID: 3, Preset: PresetTailor}}
	got, err := NewStore(f).ChatSessions(context.Background(), 3)
	if err != nil {
		t.Fatalf("ChatSessions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("rail lists %d sessions, want none — the only one is a tailoring chat", len(got))
	}
}

func TestChatSessionsListsChats(t *testing.T) {
	f := &fakeQueries{session: db.AssistantSession{ID: sessionID, UserID: 3, Preset: PresetChat}}
	got, err := NewStore(f).ChatSessions(context.Background(), 3)
	if err != nil {
		t.Fatalf("ChatSessions: %v", err)
	}
	if len(got) != 1 || got[0].ID != sessionID {
		t.Errorf("rail = %+v, want the one chat", got)
	}
}

// ptr is the shorthand for the nullable columns the queries model as pointers.
func ptr[T any](v T) *T { return &v }
