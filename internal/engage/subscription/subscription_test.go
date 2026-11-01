package subscription

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/search/search"
)

func TestCreateRefusesAnExistingSearchThatCannotRun(t *testing.T) {
	query := url.Values{"q": {"go OR rust OR java OR python OR ruby OR php"}}.Encode()
	repo := &fakeRepo{savedSearchQuery: query}
	_, err := New(repo).Create(context.Background(), 1, 2, ChannelEmail)
	if !errors.Is(err, search.ErrTooManyAlternatives) {
		t.Fatalf("error = %v, want ErrTooManyAlternatives", err)
	}
	if repo.created != nil {
		t.Fatal("subscribed to an existing search that the matcher cannot execute")
	}
}

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
	// savedSearchQuery is what the saved search being subscribed to carries, returned
	// verbatim — an empty one is a real value here, not "unset", since that is the case
	// the guard exists for.
	savedSearchQuery string
	savedSearchErr   error
	// hasPushDevice is whether the account has a registered device.
	hasPushDevice bool
}

func (r *fakeRepo) HasPushDevice(context.Context, int64) (bool, error) {
	return r.hasPushDevice, nil
}

func (r *fakeRepo) SavedSearchQuery(context.Context, int64, int64) (string, error) {
	return r.savedSearchQuery, r.savedSearchErr
}

// filteredRepo is a fake whose saved search carries a real filter — what every case
// that is not about the guard wants.
func filteredRepo() *fakeRepo { return &fakeRepo{savedSearchQuery: "seniority=senior"} }

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
	repo := filteredRepo()
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
	repo := filteredRepo()
	_, err := New(repo).Create(context.Background(), 1, 2, "carrier-pigeon")
	if !errors.Is(err, ErrInvalidChannel) {
		t.Errorf("Create(unknown) error = %v, want ErrInvalidChannel", err)
	}
	if repo.created != nil {
		t.Errorf("repo Create was called with %+v, want no call for an invalid channel", repo.created)
	}
}

// Saving an unfiltered board is legitimate — it is the "show all" view. SUBSCRIBING to
// one asks to be notified about every posting in the catalogue, which is never what
// anyone means. On prod 2026-09-04 one such subscription held 248k undelivered matches
// and, through a claim ordered by subscription id, starved every subscription created
// after it.
//
// A blank query is only the obvious half of it. The queries below are all non-empty
// strings that the delivery worker reads as no filter at all — a facet that has been
// retired, a param mistyped in the singular, a saved sort with nothing to sort — so a
// guard on emptiness lets each of them through and mails the same 248k.
func TestCreateRefusesAnUnfilteredSavedSearch(t *testing.T) {
	for _, query := range []string{
		"",
		"   ",
		"remote=remote_unspecified", // a retired facet: no filter reads `remote`
		"country=it",                // the facet is `countries`; this narrows nothing
		"sort=created_at&limit=20",  // transport params only
	} {
		repo := &fakeRepo{savedSearchQuery: query}
		_, err := New(repo).Create(context.Background(), 1, 2, "telegram")
		if !errors.Is(err, ErrUnfilteredSearch) {
			t.Errorf("query %q: err = %v, want ErrUnfilteredSearch", query, err)
		}
		if repo.created != nil {
			t.Errorf("query %q: nothing must be stored", query)
		}
	}
}

// The other side of the same gate: a query that DOES narrow must still be subscribable,
// including the one that carries no facet at all. Free text is what the matcher searches
// on, so a saved search of `q=golang` is as narrow as any facet and refusing it would
// take away the most ordinary subscription there is.
func TestCreateAcceptsEverySavedSearchThatNarrows(t *testing.T) {
	for _, query := range []string{
		"q=golang",                           // free text only
		"seniority=senior",                   // one facet
		"countries=de&sort=created_at",       // a facet beside a transport param
		"posted_within_days=7",               // a scalar filter, not a facet
		"skills_exclude=php",                 // an exclusion is a filter too
		"q=golang&remote=remote_unspecified", // the retired facet does not spoil the text
	} {
		repo := &fakeRepo{savedSearchQuery: query}
		if _, err := New(repo).Create(context.Background(), 1, 2, "telegram"); err != nil {
			t.Errorf("query %q: err = %v, want nil", query, err)
		}
		if repo.created == nil {
			t.Errorf("query %q: want the subscription stored", query)
		}
	}
}

// The guard reads the saved search, so a missing or non-owned one must still surface as
// not-found rather than as "unfiltered".
func TestCreateSurfacesAMissingSavedSearch(t *testing.T) {
	repo := &fakeRepo{savedSearchErr: ErrSavedSearchNotFound}
	if _, err := New(repo).Create(context.Background(), 1, 2, "telegram"); !errors.Is(err, ErrSavedSearchNotFound) {
		t.Errorf("err = %v, want ErrSavedSearchNotFound", err)
	}
}

// Push delivers only to a registered device, so subscribing on it without one
// would queue matches that can never leave. 49 accounts held such a
// subscription on prod (2026-10-21) and none had a device: 820k matches piled up.
func TestCreate_PushNeedsARegisteredDevice(t *testing.T) {
	repo := filteredRepo()
	if _, err := New(repo).Create(context.Background(), 1, 2, notify.ChannelPush); !errors.Is(err, ErrNoPushDevice) {
		t.Fatalf("Create(push) without a device: err = %v, want ErrNoPushDevice", err)
	}
	if repo.created != nil {
		t.Error("Create(push) without a device must not reach the repository")
	}

	repo = filteredRepo()
	repo.hasPushDevice = true
	if _, err := New(repo).Create(context.Background(), 1, 2, notify.ChannelPush); err != nil {
		t.Fatalf("Create(push) with a device: err = %v, want nil", err)
	}
}
