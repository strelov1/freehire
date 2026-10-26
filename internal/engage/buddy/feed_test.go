package buddy

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
)

func TestFeedCursorIsTheBareEventID(t *testing.T) {
	if got := encodeFeedCursor(42); got != strconv.Itoa(42) {
		t.Fatalf("cursor = %q", got)
	}
	if id, err := decodeFeedCursor("42"); id != 42 || err != nil {
		t.Fatalf("decode = %d, %v", id, err)
	}
	if id, err := decodeFeedCursor(""); id != 0 || err != nil {
		t.Fatalf("empty cursor = %d, %v", id, err)
	}
	for _, bad := range []string{"!!", "0", "-5", "1_2", "1700000000123456_42", "4x"} {
		if _, err := decodeFeedCursor(bad); !errors.Is(err, ErrBadFeedRequest) {
			t.Errorf("cursor %q: err = %v", bad, err)
		}
	}
}

func TestFeedType(t *testing.T) {
	if feedType("applied", "") != FeedApplied || feedType("stage_set", "interview") != FeedInterview || feedType("stage_set", "offer") != FeedOffer {
		t.Fatal("feedType mapping")
	}
}

func TestFeedRefusesBadModeAndNonMember(t *testing.T) {
	if _, err := New(stubRepo{}, nil).Feed(context.Background(), 1, "bogus", "", 10); !errors.Is(err, ErrBadFeedRequest) {
		t.Errorf("bad mode: %v", err)
	}
	if _, err := New(stubRepo{}, nil).Feed(context.Background(), 1, FeedAll, "", 10); !errors.Is(err, ErrNotMember) {
		t.Errorf("non-member: %v", err)
	}
	if err := New(stubRepo{}, nil).HideApplication(context.Background(), 1, 2); !errors.Is(err, ErrNotMember) {
		t.Errorf("hide by non-member: %v", err)
	}
}

// feedRepo answers Similar with a fixed top set and records what Feed was asked for.
type feedRepo struct {
	Repository
	top     []SimilarRow
	rows    []FeedRow
	authors []string
	similar bool
}

func (f *feedRepo) Member(context.Context, int64) (Member, bool, error) {
	return Member{Active: true}, true, nil
}

func (f *feedRepo) Similar(_ context.Context, _ int64, limit, _ int) ([]SimilarRow, error) {
	if limit != SimilarFeedMembers {
		return nil, errors.New("similar feed must ask for the top SimilarFeedMembers")
	}
	return f.top, nil
}

func (f *feedRepo) Feed(_ context.Context, _ int64, authors []string, similarOnly bool, _ int64, _ int) ([]FeedRow, error) {
	f.authors, f.similar = authors, similarOnly
	return f.rows, nil
}

func TestFeedSimilarModeHandsOnlyTheTopMembersToTheQuery(t *testing.T) {
	r := &feedRepo{top: []SimilarRow{{Handle: "a"}, {Handle: "b"}}}
	if _, err := New(r, nil).Feed(context.Background(), 1, FeedSimilar, "", 10); err != nil {
		t.Fatal(err)
	}
	if !r.similar || !slices.Equal(r.authors, []string{"a", "b"}) {
		t.Fatalf("query got similar=%v authors=%v", r.similar, r.authors)
	}
	// No similar members at all is an empty feed, not a fall back to everyone.
	empty := &feedRepo{}
	page, err := New(empty, nil).Feed(context.Background(), 1, FeedSimilar, "", 10)
	if err != nil || len(page.Entries) != 0 || empty.authors != nil {
		t.Fatalf("empty top: %+v, %v, authors %v", page, err, empty.authors)
	}
}

func TestFeedContinuationMustStartAtTheCursorEvent(t *testing.T) {
	// The cursor event is missing from the answer (not visible to this viewer).
	r := &feedRepo{rows: []FeedRow{{ID: 7}, {ID: 6}}}
	if _, err := New(r, nil).Feed(context.Background(), 1, FeedAll, "9", 10); !errors.Is(err, ErrBadFeedRequest) {
		t.Fatalf("foreign cursor: %v", err)
	}
	r = &feedRepo{rows: []FeedRow{{ID: 9}, {ID: 7}, {ID: 6}}}
	page, err := New(r, nil).Feed(context.Background(), 1, FeedAll, "9", 10)
	if err != nil || len(page.Entries) != 2 || page.Entries[0].ID != 7 || page.NextCursor != "" {
		t.Fatalf("page = %+v, %v", page, err)
	}
}
