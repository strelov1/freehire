package messaging

import (
	"context"
	"math"
	"testing"
)

type paginationRepository struct {
	Repository
	limit int
}

func (*paginationRepository) ConversationFor(context.Context, int64, int64) (ConversationRow, bool, error) {
	return ConversationRow{}, true, nil
}

func (r *paginationRepository) Messages(_ context.Context, _, _ int64, limit int) ([]MessageRow, error) {
	r.limit = limit
	return nil, nil
}

// The repository converts the page limit to int32. Exercise the service boundary with
// values that would overflow that conversion if its clamp were removed.
func TestMessagesBoundsRepositoryPageLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		want  int
	}{
		{"default", 0, DefaultMessagesLimit},
		{"negative", math.MinInt, DefaultMessagesLimit},
		{"one", 1, 1},
		{"maximum", MaxMessagesLimit, MaxMessagesLimit},
		{"above maximum", MaxMessagesLimit + 1, MaxMessagesLimit},
		{"int32 maximum", math.MaxInt32, MaxMessagesLimit},
		{"int maximum", math.MaxInt, MaxMessagesLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &paginationRepository{}
			if _, err := New(repo).Messages(context.Background(), 1, 2, 0, tc.limit); err != nil {
				t.Fatal(err)
			}
			if repo.limit != tc.want {
				t.Fatalf("repository page limit = %d, want %d", repo.limit, tc.want)
			}
		})
	}
}
