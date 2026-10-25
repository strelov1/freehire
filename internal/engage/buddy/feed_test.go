package buddy

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFeedCursorRoundTrip(t *testing.T) {
	in := FeedCursor{At: time.UnixMicro(1_700_000_000_123_456).UTC(), ID: 42}
	got, err := decodeFeedCursor(encodeFeedCursor(in))
	if err != nil || got == nil || !got.At.Equal(in.At) || got.ID != in.ID {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
	if c, err := decodeFeedCursor(""); c != nil || err != nil {
		t.Fatalf("empty cursor = %+v, %v", c, err)
	}
	for _, bad := range []string{"!!", "bm9wZQ", "YV9i"} {
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
	if _, err := New(stubRepo{}).Feed(context.Background(), 1, "bogus", "", 10); !errors.Is(err, ErrBadFeedRequest) {
		t.Errorf("bad mode: %v", err)
	}
	if _, err := New(stubRepo{}).Feed(context.Background(), 1, FeedAll, "", 10); !errors.Is(err, ErrNotMember) {
		t.Errorf("non-member: %v", err)
	}
	if err := New(stubRepo{}).HideApplication(context.Background(), 1, 2); !errors.Is(err, ErrNotMember) {
		t.Errorf("hide by non-member: %v", err)
	}
}
