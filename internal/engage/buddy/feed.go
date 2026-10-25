package buddy

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/application/appevent"
)

// FeedMode selects whose events the feed reads.
type FeedMode string

const (
	// FeedAll reads every active member's events.
	FeedAll FeedMode = "all"
	// FeedSimilar reads the events of the viewer's SimilarFeedMembers most similar members.
	FeedSimilar FeedMode = "similar"
)

const (
	// SimilarFeedMembers is how many of the most similar members the similar feed reads.
	SimilarFeedMembers = 200
	// MaxFeedLimit is the largest feed page; DefaultFeedLimit applies when none is asked.
	MaxFeedLimit     = 50
	DefaultFeedLimit = 20
)

// Feed event types. Nothing else ever reaches the feed.
const (
	FeedApplied   = "applied"
	FeedInterview = "interview"
	FeedOffer     = "offer"
)

// ErrBadFeedRequest is returned for an unknown mode or a cursor the feed did not issue.
var ErrBadFeedRequest = errors.New("buddy: invalid feed request")

// FeedEntry is one event in the feed. Day is the UTC calendar date (YYYY-MM-DD): the
// time of day is deliberately not carried, since it would help identify the author.
type FeedEntry struct {
	ID       int64
	Handle   string
	Card     CardSummary
	Company  Company
	JobTitle *string
	Type     string
	Day      string
}

// FeedPage is one page and the cursor of the next, empty when this is the last.
type FeedPage struct {
	Entries    []FeedEntry
	NextCursor string
}

// FeedCursor is the keyset position: the last entry a page returned.
type FeedCursor struct {
	At time.Time
	ID int64
}

// FeedRow is a Repository's raw answer for one event. OccurredAt only feeds the cursor.
type FeedRow struct {
	ID         int64
	OccurredAt time.Time
	Day        string
	Kind       string
	Signal     string
	Company    Company
	JobTitle   *string
	Author     SimilarRow
}

// Feed reads one page of the viewer's buddy feed, newest first. limit is clamped to
// 1..MaxFeedLimit (zero means the default). cursor is empty for the first page.
func (s *Service) Feed(ctx context.Context, viewerID int64, mode FeedMode, cursor string, limit int) (FeedPage, error) {
	if mode != FeedAll && mode != FeedSimilar {
		return FeedPage{}, ErrBadFeedRequest
	}
	pos, err := decodeFeedCursor(cursor)
	if err != nil {
		return FeedPage{}, err
	}
	if err := s.requireActive(ctx, viewerID); err != nil {
		return FeedPage{}, err
	}
	limit = clampFeedLimit(limit)

	var authors []string
	if mode == FeedSimilar {
		// The ranking is the similar list's, asked for once: its SQL is the only copy of
		// the scoring formula.
		top, err := s.repo.Similar(ctx, viewerID, SimilarFeedMembers, 0)
		if err != nil {
			return FeedPage{}, err
		}
		for _, r := range top {
			authors = append(authors, r.Handle)
		}
		if len(authors) == 0 {
			return FeedPage{}, nil
		}
	}
	// One extra row says whether another page exists.
	rows, err := s.repo.Feed(ctx, viewerID, authors, mode == FeedSimilar, pos, limit+1)
	if err != nil {
		return FeedPage{}, err
	}
	var page FeedPage
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		page.NextCursor = encodeFeedCursor(FeedCursor{At: last.OccurredAt, ID: last.ID})
	}
	page.Entries = make([]FeedEntry, len(rows))
	for i, r := range rows {
		page.Entries[i] = FeedEntry{
			ID: r.ID, Handle: r.Author.Handle, Card: cardSummary(r.Author),
			Company: r.Company, JobTitle: r.JobTitle, Type: feedType(r.Kind, r.Signal), Day: r.Day,
		}
	}
	return page, nil
}

// HideApplication keeps one of the caller's applications out of the buddy feed and the
// shared-company count. An application that is not the caller's is ErrNotFound.
func (s *Service) HideApplication(ctx context.Context, userID, applicationID int64) error {
	if err := s.requireActive(ctx, userID); err != nil {
		return err
	}
	ok, err := s.repo.HideApplication(ctx, userID, applicationID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// ShowApplication lifts a hide. Showing an application that was not hidden is a no-op.
func (s *Service) ShowApplication(ctx context.Context, userID, applicationID int64) error {
	if err := s.requireActive(ctx, userID); err != nil {
		return err
	}
	return s.repo.ShowApplication(ctx, userID, applicationID)
}

// HiddenApplications lists the caller's hidden application ids for the tracker.
func (s *Service) HiddenApplications(ctx context.Context, userID int64) ([]int64, error) {
	if err := s.requireActive(ctx, userID); err != nil {
		return nil, err
	}
	return s.repo.HiddenApplications(ctx, userID)
}

func (s *Service) requireActive(ctx context.Context, userID int64) error {
	m, ok, err := s.repo.Member(ctx, userID)
	if err != nil {
		return err
	}
	if !ok || !m.Active {
		return ErrNotMember
	}
	return nil
}

func clampFeedLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultFeedLimit
	case limit > MaxFeedLimit:
		return MaxFeedLimit
	}
	return limit
}

// feedType names a ledger event for the feed. The query admits only the three cases.
func feedType(kind, signal string) string {
	if kind == appevent.KindApplied {
		return FeedApplied
	}
	return signal
}

// The cursor is opaque to clients: "<unix micros>_<id>", base64url. Microseconds are
// the precision Postgres stores, so a round trip loses nothing.
func encodeFeedCursor(c FeedCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d_%d", c.At.UnixMicro(), c.ID)))
}

func decodeFeedCursor(s string) (*FeedCursor, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, ErrBadFeedRequest
	}
	micros, id, ok := strings.Cut(string(raw), "_")
	if !ok {
		return nil, ErrBadFeedRequest
	}
	us, err1 := strconv.ParseInt(micros, 10, 64)
	n, err2 := strconv.ParseInt(id, 10, 64)
	if err1 != nil || err2 != nil {
		return nil, ErrBadFeedRequest
	}
	return &FeedCursor{At: time.UnixMicro(us).UTC(), ID: n}, nil
}
