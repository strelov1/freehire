package subscription

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// fakeRepo records Create calls so a test can assert the channel that reached the
// repository (and whether it was reached at all).
type fakeRepo struct {
	created *db.CreateSubscriptionParams
}

func (r *fakeRepo) List(context.Context, int64) ([]db.ListSubscriptionsRow, error) {
	return nil, nil
}

func (r *fakeRepo) Create(_ context.Context, p db.CreateSubscriptionParams) (db.Subscription, error) {
	r.created = &p
	return db.Subscription{Channel: p.Channel}, nil
}

func (r *fakeRepo) SetActive(context.Context, db.SetSubscriptionActiveParams) (db.Subscription, error) {
	return db.Subscription{}, nil
}

func (r *fakeRepo) Delete(context.Context, db.DeleteSubscriptionParams) error { return nil }

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
