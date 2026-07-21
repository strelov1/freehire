package community

import (
	"context"
	"errors"
	"strings"
	"time"
)

// maxHandleAttempts bounds handle-collision retries when minting a persona. The base
// space is large, so a couple of retries is astronomically sufficient.
const maxHandleAttempts = 8

// Config tunes rate limits and page size. Zero values fall back to the Default*
// constants in New.
type Config struct {
	ThreadWindow time.Duration
	ThreadCap    int
	ReplyWindow  time.Duration
	ReplyCap     int
	PageSize     int32
}

// Service is the community use case: it validates input, enforces anonymity by
// minting personas, rate-limits per user, and orchestrates persistence.
type Service struct {
	repo     Repository
	subjects SubjectChecker
	cfg      Config
	now      func() time.Time
}

// New constructs a Service, filling any zero Config field with its default.
func New(repo Repository, subjects SubjectChecker, cfg Config) *Service {
	if cfg.ThreadWindow == 0 {
		cfg.ThreadWindow = DefaultThreadWindow
	}
	if cfg.ThreadCap == 0 {
		cfg.ThreadCap = DefaultThreadCap
	}
	if cfg.ReplyWindow == 0 {
		cfg.ReplyWindow = DefaultReplyWindow
	}
	if cfg.ReplyCap == 0 {
		cfg.ReplyCap = DefaultReplyCap
	}
	if cfg.PageSize == 0 {
		cfg.PageSize = 30
	}
	return &Service{repo: repo, subjects: subjects, cfg: cfg, now: time.Now}
}

// validSubjectType reports whether t is a supported subject discriminator.
func validSubjectType(t string) bool {
	return t == SubjectCompany || t == SubjectJob
}

// CreateThread opens a thread on a subject: validate the subject and body, enforce
// the per-user rate limit, mint the author's persona, and insert. The returned
// Thread carries the persona handle, never the author's user id.
func (s *Service) CreateThread(ctx context.Context, in CreateThreadInput) (Thread, error) {
	if !validSubjectType(in.SubjectType) {
		return Thread{}, ErrUnsupportedSubject
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Thread{}, ErrEmptyTitle
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return Thread{}, ErrEmptyBody
	}

	exists, err := s.subjects.SubjectExists(ctx, in.SubjectType, in.SubjectSlug)
	if err != nil {
		return Thread{}, err
	}
	if !exists {
		return Thread{}, ErrSubjectNotFound
	}

	if err := s.checkRate(ctx, in.UserID, s.repo.CountRecentThreads, s.cfg.ThreadWindow, s.cfg.ThreadCap); err != nil {
		return Thread{}, err
	}

	persona, err := s.persona(ctx, in.UserID)
	if err != nil {
		return Thread{}, err
	}

	thread, err := s.repo.InsertThread(ctx, in.SubjectType, in.SubjectSlug, title, body, in.UserID)
	if err != nil {
		return Thread{}, err
	}
	thread.AuthorHandle = persona.Handle
	return thread, nil
}

// ListThreads returns a subject's open threads, newest first, keyset-paged.
func (s *Service) ListThreads(ctx context.Context, subjectType, subjectSlug string, cur Cursor) ([]Thread, error) {
	if !validSubjectType(subjectType) {
		return nil, ErrUnsupportedSubject
	}
	return s.repo.ListOpenThreads(ctx, subjectType, subjectSlug, cur, s.cfg.PageSize)
}

// CountThreads returns how many open threads a subject has — the detail-page badge.
func (s *Service) CountThreads(ctx context.Context, subjectType, subjectSlug string) (int64, error) {
	if !validSubjectType(subjectType) {
		return 0, ErrUnsupportedSubject
	}
	return s.repo.CountOpenThreads(ctx, subjectType, subjectSlug)
}

// GetThread returns a single thread by id.
func (s *Service) GetThread(ctx context.Context, id int64) (Thread, error) {
	return s.repo.GetThread(ctx, id)
}

// ListReplies returns a thread's replies, oldest first, keyset-paged.
func (s *Service) ListReplies(ctx context.Context, threadID int64, cur Cursor) ([]Reply, error) {
	return s.repo.ListReplies(ctx, threadID, cur, s.cfg.PageSize)
}

// Reply posts a reply to an open thread: validate the body and thread, enforce the
// per-user rate limit, mint the persona, insert, and bump the denormalized count.
// parentReplyID is 0 for a top-level reply, or another reply's id to nest under it.
func (s *Service) Reply(ctx context.Context, threadID, parentReplyID, userID int64, body string) (Reply, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Reply{}, ErrEmptyBody
	}

	thread, err := s.repo.GetThread(ctx, threadID)
	if err != nil {
		return Reply{}, err
	}
	if thread.Status == StatusClosed {
		return Reply{}, ErrThreadClosed
	}

	if err := s.checkRate(ctx, userID, s.repo.CountRecentReplies, s.cfg.ReplyWindow, s.cfg.ReplyCap); err != nil {
		return Reply{}, err
	}

	persona, err := s.persona(ctx, userID)
	if err != nil {
		return Reply{}, err
	}

	reply, err := s.repo.InsertReply(ctx, threadID, parentReplyID, userID, body)
	if err != nil {
		return Reply{}, err
	}
	// Best-effort denormalized count; a rare drift is cosmetic, not correctness.
	if err := s.repo.IncrementReplyCount(ctx, threadID); err != nil {
		return Reply{}, err
	}
	reply.AuthorHandle = persona.Handle
	return reply, nil
}

// Close marks a thread closed (moderator action): it leaves the subject listing and
// rejects new replies.
func (s *Service) Close(ctx context.Context, threadID int64) error {
	return s.repo.CloseThread(ctx, threadID)
}

// persona returns the user's stable handle, minting one on first use. It retries on
// a handle collision with a fresh candidate; the user_id primary key makes a
// concurrent same-user mint resolve to the existing persona in the repository.
func (s *Service) persona(ctx context.Context, userID int64) (Persona, error) {
	p, err := s.repo.GetPersona(ctx, userID)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, ErrPersonaNotFound) {
		return Persona{}, err
	}
	for attempt := 0; attempt < maxHandleAttempts; attempt++ {
		p, err = s.repo.InsertPersona(ctx, userID, GenerateHandle())
		if err == nil {
			return p, nil
		}
		if errors.Is(err, ErrHandleTaken) {
			continue
		}
		return Persona{}, err
	}
	return Persona{}, ErrHandleTaken
}

// checkRate rejects the action when the user is at or over the cap within the window.
func (s *Service) checkRate(ctx context.Context, userID int64, count func(context.Context, int64, time.Time) (int64, error), window time.Duration, limit int) error {
	n, err := count(ctx, userID, s.now().Add(-window))
	if err != nil {
		return err
	}
	if n >= int64(limit) {
		return ErrRateLimited
	}
	return nil
}
