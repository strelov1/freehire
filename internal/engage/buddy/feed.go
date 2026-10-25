package buddy

import (
	"context"
	"errors"
	"strconv"
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

// FeedRow is a Repository's raw answer for one event.
type FeedRow struct {
	ID       int64
	Day      string
	Kind     string
	Signal   string
	Company  Company
	JobTitle *string
	Author   SimilarRow
}

// Feed reads one page of the viewer's buddy feed, newest first. limit is clamped to
// 1..MaxFeedLimit (zero means the default). cursor is empty for the first page.
func (s *Service) Feed(ctx context.Context, viewerID int64, mode FeedMode, cursor string, limit int) (FeedPage, error) {
	if mode != FeedAll && mode != FeedSimilar {
		return FeedPage{}, ErrBadFeedRequest
	}
	after, err := decodeFeedCursor(cursor)
	if err != nil {
		return FeedPage{}, err
	}
	if err := s.requireActive(ctx, viewerID); err != nil {
		return FeedPage{}, err
	}

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
	return s.feedPage(ctx, viewerID, authors, mode == FeedSimilar, after, clampFeedLimit(limit))
}

// MemberFeed reads one page of a single member's events, for that member's page. It is
// the same query as Feed narrowed to one author, so every visibility rule still applies;
// a handle the viewer may not see is ErrNotFound, exactly as Profile answers it.
func (s *Service) MemberFeed(ctx context.Context, viewerID int64, handle, cursor string, limit int) (FeedPage, error) {
	after, err := decodeFeedCursor(cursor)
	if err != nil {
		return FeedPage{}, err
	}
	target, err := s.visibleMember(ctx, viewerID, handle)
	if err != nil {
		return FeedPage{}, err
	}
	return s.feedPage(ctx, viewerID, []string{target.Handle}, true, after, clampFeedLimit(limit))
}

// ActivityDays is how far back a member's activity grid reaches, today included.
const ActivityDays = 365

// ActivityDay is how many feed events a member has on one UTC calendar date (YYYY-MM-DD).
type ActivityDay struct {
	Day   string
	Count int
}

// MemberActivity counts one member's feed events per UTC day over the last ActivityDays
// days, for the year grid on their page. It counts exactly what MemberFeed lists and
// nothing else — never the tracker's own activity, which also counts follow-ups and every
// stage move. A handle the viewer may not see is ErrNotFound, as Profile answers it.
func (s *Service) MemberActivity(ctx context.Context, viewerID int64, handle string) ([]ActivityDay, error) {
	target, err := s.visibleMember(ctx, viewerID, handle)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return s.repo.Activity(ctx, viewerID, target.UserID, today.AddDate(0, 0, -(ActivityDays-1)))
}

// feedPage reads the page after the event `after` (0 for the first page), restricted to
// authors when authorsOnly is set.
func (s *Service) feedPage(ctx context.Context, viewerID int64, authors []string, authorsOnly bool, after int64, limit int) (FeedPage, error) {
	// One extra row says whether another page exists. A continuation also reads the
	// cursor's own event, which the repository returns first.
	want := limit + 1
	if after != 0 {
		want++
	}
	rows, err := s.repo.Feed(ctx, viewerID, authors, authorsOnly, after, want)
	if err != nil {
		return FeedPage{}, err
	}
	if after != 0 {
		// The cursor must name an event this viewer can see in this feed: otherwise it
		// is forged, or the event has since been hidden, and its position is not ours
		// to reveal.
		if len(rows) == 0 || rows[0].ID != after {
			return FeedPage{}, ErrBadFeedRequest
		}
		rows = rows[1:]
	}
	var page FeedPage
	if len(rows) > limit {
		rows = rows[:limit]
		page.NextCursor = encodeFeedCursor(rows[limit-1].ID)
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

// The cursor is the id of the last entry of the page and nothing else. The server
// resolves the keyset position from that event itself, so no time ever leaves it.
func encodeFeedCursor(id int64) string { return strconv.FormatInt(id, 10) }

// decodeFeedCursor returns 0 for the first page.
func decodeFeedCursor(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrBadFeedRequest
	}
	return id, nil
}
