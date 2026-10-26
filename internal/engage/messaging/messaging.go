// Package messaging is the conversation core every people-to-people feature shares:
// conversations of a kind between accounts, the request-and-accept step, messages and read
// cursors, account-wide blocks, reports to moderators and the unread-notice worker.
//
// Every participant speaks as a PERSONA: one account has several (its real self, an
// anonymous buddy handle, later a mentor or an employer), and the other side of a
// conversation is drawn from the persona it speaks as there, never from the account. A
// feature plugs in twice: as a Persona, which draws and names the accounts speaking as it,
// and as a Kind, which words the notices of its conversations. Who may start a
// conversation is the feature's own rule (its service checks it, then calls Start); the
// core enforces only what holds everywhere: participation, blocks, the pair key, message
// length. The core never imports a feature. See AGENTS.md beside this file.
package messaging

import (
	"context"
	"errors"
	"time"
)

// Conversation statuses as stored.
const (
	StatusPending  = "pending"
	StatusOpen     = "open"
	StatusDeclined = "declined"
	StatusClosed   = "closed"
)

// What a conversation is to the participant looking at it. A pending one is incoming for
// its recipient and outgoing for its starter, and a declined one stays outgoing: the
// starter is never told it was refused.
const (
	ForMeIncoming = "incoming"
	ForMeOutgoing = "outgoing"
	ForMeOpen     = "open"
	ForMeClosed   = "closed"
)

const (
	// MaxMessageRunes bounds one message. A kind may bound its first message tighter.
	MaxMessageRunes = 4000
	// MaxMessagesLimit is the largest page Messages returns; DefaultMessagesLimit applies
	// when none is asked.
	MaxMessagesLimit     = 100
	DefaultMessagesLimit = 50
)

var (
	// ErrNotFound is a conversation that does not exist or that the caller does not take
	// part in; the two are deliberately the same answer (404).
	ErrNotFound = errors.New("messaging: conversation not found")
	// ErrInvalidMessage is a message that is empty or too long (400).
	ErrInvalidMessage = errors.New("messaging: message is empty or too long")
	// ErrUnavailable is a start refused because a block stands between the two accounts,
	// or because the starter named themselves. It never says which (403).
	ErrUnavailable = errors.New("messaging: conversation cannot be started")
	// ErrExists is a start on a pair that already has a conversation of the kind (409).
	ErrExists = errors.New("messaging: a conversation already exists")
	// ErrState is an answer to a conversation that is not pending or by its starter, or a
	// message into one that is not open (409).
	ErrState = errors.New("messaging: conversation cannot be answered or written to")
)

// Counterpart is how one participant is drawn for another. Persona is the persona that
// participant speaks as; Detail is that persona's own value (for buddy, the handle and
// card), nil when Reachable is false, and then nothing about the other side is served.
type Counterpart struct {
	Persona   string
	Reachable bool
	Detail    any
}

// Notice is the wording and destination of one unread notice. It carries nothing a
// participant wrote and nothing that names an account.
type Notice struct {
	Title string
	Line  string
	// Path is the site-relative link the notice opens.
	Path string
	// NotificationKind is the in-app notification center's kind for it.
	NotificationKind string
	// Footer tells the recipient why they are getting the mail.
	Footer string
}

// Persona draws and names the accounts that speak as it. Implementations are registered
// when the Service is built.
//
// Personas are never joined: nothing a Persona returns may identify the same account under
// another persona — no account id, no email, no other handle.
type Persona interface {
	// Name is the stored conversation_participants.persona.
	Name() string
	// Counterparts describes each of otherIDs, all speaking as this persona, as viewerID
	// may see them, keyed by account. An id missing from the answer is drawn as
	// unreachable.
	Counterparts(ctx context.Context, viewerID int64, otherIDs []int64) (map[int64]Counterpart, error)
	// Names is what a moderator reads each account as when it speaks as this persona: the
	// name the others saw. An id missing from the answer is shown with no name.
	Names(ctx context.Context, userIDs []int64) (map[int64]string, error)
}

// Kind is a feature that holds conversations, as far as the notices are concerned.
// Implementations are registered when the Notifier is built.
type Kind interface {
	// Name is the stored conversations.kind.
	Name() string
	// Notice is the unread notice for one conversation; pending is a request not yet
	// answered.
	Notice(conversationID int64, pending bool) Notice
}

// Participant is one account in a conversation and the persona it speaks as there.
type Participant struct {
	UserID  int64
	Persona string
}

// ConversationRow is a stored conversation.
type ConversationRow struct {
	ID         int64
	Kind       string
	StarterID  int64
	Status     string
	CreatedAt  time.Time
	AcceptedAt *time.Time
}

// Conversation is a conversation as one of its participants sees it. Unread and
// LastMessageAt are filled by List only.
type Conversation struct {
	ID          int64
	Kind        string
	Status      string
	StatusForMe string
	// Starter is true when the viewer started it.
	Starter       bool
	CreatedAt     time.Time
	AcceptedAt    *time.Time
	Counterpart   Counterpart
	Unread        int
	LastMessageAt time.Time
}

// Message is one message as a participant sees it.
type Message struct {
	ID        int64
	Mine      bool
	Body      string
	CreatedAt time.Time
}

// MessageRow is a Repository's raw answer for one message.
type MessageRow struct {
	ID        int64
	SenderID  int64
	Body      string
	CreatedAt time.Time
}

// ListRow is a Repository's raw answer for one entry of a participant's list.
type ListRow struct {
	ConversationRow
	Other         Participant
	Unread        int
	LastMessageAt time.Time
}

// StartRequest opens a conversation of a kind between two accounts, each speaking as a
// persona. PairKey is the kind's uniqueness rule (empty for none): a second start on the
// same key reopens a conversation closed without a block and is refused otherwise.
type StartRequest struct {
	Kind             string
	StarterID        int64
	StarterPersona   string
	RecipientID      int64
	RecipientPersona string
	PairKey          string
	Body             string
}

// Repository is the storage the Service needs.
type Repository interface {
	// Start inserts a pending conversation, its participants and its first message
	// atomically, reopening a closed one on the same pair key. It returns ErrUnavailable
	// when a block stands between the two and ErrExists when the pair key is taken by a
	// conversation that is not closed.
	Start(ctx context.Context, r StartRequest) (ConversationRow, error)
	// ConversationByPairKey reads the kind's conversation for a pair key, any status.
	ConversationByPairKey(ctx context.Context, kind, pairKey string) (ConversationRow, bool, error)
	// ConversationFor reads one conversation only when userID takes part in it.
	ConversationFor(ctx context.Context, userID, conversationID int64) (ConversationRow, bool, error)
	// Participants lists a conversation's accounts and their personas.
	Participants(ctx context.Context, conversationID int64) ([]Participant, error)
	// StartedSince is how many conversations of the kind userID started after since, and
	// when the oldest of them was started (the zero time when there are none).
	StartedSince(ctx context.Context, kind string, userID int64, since time.Time) (int, time.Time, error)
	// Blocked reports a block between the two accounts in either direction.
	Blocked(ctx context.Context, a, b int64) (bool, error)
	// Answer moves a pending conversation to status (open or declined) on behalf of a
	// participant who did not start it. It returns ErrNotFound when the caller does not
	// take part and ErrState when it is not pending or the caller started it.
	Answer(ctx context.Context, userID, conversationID int64, status string) (ConversationRow, error)
	// InsertMessage stores a message from userID into an open conversation they take part
	// in and moves their read cursor onto it. It returns ErrNotFound when userID does not
	// take part and ErrState when the conversation is not open.
	InsertMessage(ctx context.Context, userID, conversationID int64, body string) (MessageRow, error)
	// Messages is up to limit messages with an id above after, oldest first.
	Messages(ctx context.Context, conversationID, after int64, limit int) ([]MessageRow, error)
	// MarkRead advances userID's read cursor to upTo, never back and never past the newest
	// message. It returns ErrNotFound when userID does not take part.
	MarkRead(ctx context.Context, userID, conversationID, upTo int64) error
	// Conversations lists every conversation userID may see, newest message first.
	Conversations(ctx context.Context, userID int64) ([]ListRow, error)
	// Block records that blocker blocked the other participant and closes every
	// conversation between them. It returns ErrNotFound when blocker does not take part.
	Block(ctx context.Context, blocker, conversationID int64) error
	// Report stores a report. It returns ErrNotFound when reporter does not take part and
	// ErrAlreadyReported when they already filed one.
	Report(ctx context.Context, reporter, conversationID int64, reason, note string) error
	// Reports is one page of reports, newest first.
	Reports(ctx context.Context, limit, offset int, includeResolved bool) ([]ReportRow, error)
	// ReportMessages is every message of the given conversations, oldest first.
	ReportMessages(ctx context.Context, conversationIDs []int64) ([]ReportMessageRow, error)
	// ResolveReport stamps a report resolved, keeping an earlier resolution. It returns
	// ErrNotFound for an unknown id.
	ResolveReport(ctx context.Context, moderatorID, reportID int64) error
}

// Service is the conversation use cases.
type Service struct {
	repo     Repository
	personas map[string]Persona
}

// New builds a Service over the personas it can draw. A participant speaking as a persona
// it was not built with is drawn as unreachable.
func New(repo Repository, personas ...Persona) *Service {
	s := &Service{repo: repo, personas: make(map[string]Persona, len(personas))}
	for _, p := range personas {
		s.personas[p.Name()] = p
	}
	return s
}

func statusForMe(status string, starter bool) string {
	switch {
	case status == StatusOpen:
		return ForMeOpen
	case status == StatusClosed:
		return ForMeClosed
	case starter:
		return ForMeOutgoing
	}
	return ForMeIncoming
}

func view(userID int64, row ConversationRow) Conversation {
	starter := row.StarterID == userID
	return Conversation{
		ID: row.ID, Kind: row.Kind, Status: row.Status, StatusForMe: statusForMe(row.Status, starter),
		Starter: starter, CreatedAt: row.CreatedAt, AcceptedAt: row.AcceptedAt,
	}
}
