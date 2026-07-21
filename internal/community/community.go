// Package community is the anonymous discussion-thread use case: a signed-in user
// starts a topic attached to a subject (a company or a vacancy) and any signed-in
// user replies. Authors are shown only through a stable pseudonymous persona — the
// real user_id is the private key the handle hides behind, used for moderation and
// rate limiting but never sent to a client.
//
// The subject is polymorphic: (SubjectType, subject_ref) where subject_ref is the
// subject's public slug. The primitive is deliberately decoupled from what it
// attaches to, so a future subject type plugs in without reshaping this package.
// Persistence is a Repository; subject existence is a SubjectChecker.
package community

import (
	"context"
	"errors"
	"time"
)

// Supported subject types. subject_ref is the subject's public slug: companies.slug
// for a company, jobs.public_slug for a job.
const (
	SubjectCompany = "company"
	SubjectJob     = "job"
)

// Thread status vocabulary. A thread is open until a moderator closes it; a closed
// thread leaves the subject listing and rejects new replies.
const (
	StatusOpen   = "open"
	StatusClosed = "closed"
)

// Per-user rate-limit defaults over a rolling window. Threads and replies are free
// but spam-resistant; tune as usage shows.
const (
	DefaultThreadWindow = 24 * time.Hour
	DefaultThreadCap    = 10
	DefaultReplyWindow  = time.Hour
	DefaultReplyCap     = 30
)

// Sentinel errors, mapped to HTTP statuses by the handler.
var (
	// ErrUnsupportedSubject is a subject_type other than company or job (400).
	ErrUnsupportedSubject = errors.New("community: unsupported subject type")
	// ErrSubjectNotFound is a subject slug that names no existing company or job (404).
	ErrSubjectNotFound = errors.New("community: subject not found")
	// ErrThreadNotFound is a read/reply against a thread id that does not exist (404).
	ErrThreadNotFound = errors.New("community: thread not found")
	// ErrThreadClosed is a reply to a thread a moderator has closed (409).
	ErrThreadClosed = errors.New("community: thread is closed")
	// ErrEmptyBody is a thread or reply submitted with no body text (422).
	ErrEmptyBody = errors.New("community: body is required")
	// ErrEmptyTitle is a thread submitted with no title (422).
	ErrEmptyTitle = errors.New("community: title is required")
	// ErrRateLimited is a user over their thread or reply window cap (429).
	ErrRateLimited = errors.New("community: rate limit exceeded")
	// ErrPersonaNotFound is returned by Repository.GetPersona when a user has no
	// persona yet — the service mints one and retries.
	ErrPersonaNotFound = errors.New("community: persona not found")
	// ErrHandleTaken is a handle-unique violation on InsertPersona — the service
	// generates another candidate and retries.
	ErrHandleTaken = errors.New("community: handle already taken")
)

// Persona is a user's stable pseudonymous identity. Only Handle is ever exposed.
type Persona struct {
	UserID    int64
	Handle    string
	CreatedAt time.Time
}

// Thread is a topic attached to a subject. AuthorHandle is the persona of the
// opener; the author's user_id is never carried on this read view.
type Thread struct {
	ID           int64
	SubjectType  string
	SubjectRef   string
	AnchorPath   string // nullable seam, empty at MVP
	Title        string
	Body         string // the opening post
	AuthorHandle string
	ReplyCount   int32
	Status       string
	CreatedAt    time.Time
}

// Reply is one reply in a thread, optionally nested. ParentID is 0 for a top-level
// reply, or another reply's id when nested under it. AuthorHandle is the persona;
// IsAI marks a (future) system-authored reply, which has no persona.
type Reply struct {
	ID           int64
	ThreadID     int64
	ParentID     int64
	AuthorHandle string
	IsAI         bool
	Body         string
	CreatedAt    time.Time
}

// Cursor is a keyset page cursor over (CreatedAt, ID). A zero Cursor means "first
// page". Listings order by CreatedAt DESC, ID DESC and return rows strictly before
// the cursor, so deep pages never scan skipped rows.
type Cursor struct {
	CreatedAt time.Time
	ID        int64
}

// IsZero reports whether the cursor is the first-page sentinel.
func (c Cursor) IsZero() bool { return c.ID == 0 && c.CreatedAt.IsZero() }

// CreateThreadInput is a request to open a thread. SubjectSlug is the subject's
// public slug; the service validates it against SubjectType.
type CreateThreadInput struct {
	UserID      int64
	SubjectType string
	SubjectSlug string
	Title       string
	Body        string
}

// Repository is the persistence port. Read methods return handles (joined from
// personas), never author user ids.
type Repository interface {
	GetPersona(ctx context.Context, userID int64) (Persona, error)
	InsertPersona(ctx context.Context, userID int64, handle string) (Persona, error)

	InsertThread(ctx context.Context, subjectType, subjectRef, title, body string, authorUserID int64) (Thread, error)
	GetThread(ctx context.Context, id int64) (Thread, error)
	ListOpenThreads(ctx context.Context, subjectType, subjectRef string, cur Cursor, limit int32) ([]Thread, error)
	CountOpenThreads(ctx context.Context, subjectType, subjectRef string) (int64, error)
	CloseThread(ctx context.Context, id int64) error

	InsertReply(ctx context.Context, threadID, parentReplyID, authorUserID int64, body string) (Reply, error)
	IncrementReplyCount(ctx context.Context, threadID int64) error
	ListReplies(ctx context.Context, threadID int64, cur Cursor, limit int32) ([]Reply, error)

	CountRecentThreads(ctx context.Context, userID int64, since time.Time) (int64, error)
	CountRecentReplies(ctx context.Context, userID int64, since time.Time) (int64, error)
}

// SubjectChecker reports whether a subject slug names an existing company or job.
type SubjectChecker interface {
	SubjectExists(ctx context.Context, subjectType, slug string) (bool, error)
}
