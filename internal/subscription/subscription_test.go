package subscription

import (
	"context"
	"errors"
	"testing"
)

// createArgs captures the primitive params Create is handed, so a test can assert them
// without a db.* params struct.
type createArgs struct {
	UserID        int64
	SavedSearchID int64
	Channel       string
}

// fakeRepo records Create calls so a test can assert the channel that reached the
// repository (and whether it was reached at all).
type fakeRepo struct {
	created *createArgs
}

func (r *fakeRepo) List(context.Context, int64) ([]SubscriptionListItem, error) {
	return nil, nil
}

func (r *fakeRepo) Create(_ context.Context, userID, savedSearchID int64, channel string) (Subscription, error) {
	r.created = &createArgs{UserID: userID, SavedSearchID: savedSearchID, Channel: channel}
	return Subscription{Channel: channel}, nil
}

func (r *fakeRepo) SetActive(context.Context, int64, int64, bool) (Subscription, error) {
	return Subscription{}, nil
}

func (r *fakeRepo) Delete(context.Context, int64, int64) error { return nil }

func TestCreate_EmailChannelAccepted(t *testing.T) {
	repo := &fakeRepo{}
	sub, err := New(repo).Create(context.Background(), 1, 2, ChannelEmail)
	if err != nil {
		t.Fatalf("Create(email) error = %v, want nil", err)
	}
	if repo.created == nil || repo.created.Channel != ChannelEmail {
		t.Errorf("repo Create channel = %v, want %q reaching the repo", repo.created, ChannelEmail)
	}
	if sub.Channel != ChannelEmail {
		t.Errorf("returned channel = %q, want %q", sub.Channel, ChannelEmail)
	}
}

func TestCreate_UnknownChannelRejected(t *testing.T) {
	repo := &fakeRepo{}
	_, err := New(repo).Create(context.Background(), 1, 2, "carrier-pigeon")
	if !errors.Is(err, ErrInvalidChannel) {
		t.Errorf("Create(unknown) error = %v, want ErrInvalidChannel", err)
	}
	if repo.created != nil {
		t.Errorf("repo Create was called with %+v, want no call for an invalid channel", repo.created)
	}
}
