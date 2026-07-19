package referral

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- fakes ---------------------------------------------------------------

// fakeRepo is an in-memory Repository for the service branch tests. Each field lets a test
// steer one method's outcome; the request/offer creators capture the last input.
type fakeRepo struct {
	eligible     bool
	approved     bool
	cvOwned      bool
	hasResume    bool
	countSince   int64
	recipients   []Recipient
	getRequest   Request
	getRequestOK bool
	createErr    error
	resolveErr   error

	createdReq *RequestInput
	createdOff *OfferInput
	decided    *decidedOffer
	resolved   *resolvedRequest
}

type decidedOffer struct {
	id, moderator int64
	status        string
}

type resolvedRequest struct {
	id, actor int64
	status    string
}

func (f *fakeRepo) CreateOffer(_ context.Context, in OfferInput) (Offer, error) {
	f.createdOff = &in
	if f.createErr != nil {
		return Offer{}, f.createErr
	}
	return Offer{ID: 1, UserID: in.UserID, CompanySlug: in.CompanySlug, Status: OfferPending}, nil
}

func (f *fakeRepo) DecideOffer(_ context.Context, offerID, moderatorID int64, status string) (Offer, error) {
	f.decided = &decidedOffer{offerID, moderatorID, status}
	if f.resolveErr != nil {
		return Offer{}, f.resolveErr
	}
	return Offer{ID: offerID, Status: status}, nil
}

func (f *fakeRepo) ListOffersByUser(context.Context, int64) ([]Offer, error) { return nil, nil }
func (f *fakeRepo) ListPendingOffers(context.Context) ([]Offer, error)       { return nil, nil }
func (f *fakeRepo) CompanyHasApprovedReferrer(context.Context, string) (bool, error) {
	return f.eligible, nil
}
func (f *fakeRepo) ReferrerApprovedForCompany(context.Context, int64, string) (bool, error) {
	return f.approved, nil
}
func (f *fakeRepo) ApprovedReferrerRecipients(context.Context, string) ([]Recipient, error) {
	return f.recipients, nil
}
func (f *fakeRepo) CVBelongsToUser(context.Context, int64, int64) (bool, error) {
	return f.cvOwned, nil
}
func (f *fakeRepo) UserHasResume(context.Context, int64) (bool, error) {
	return f.hasResume, nil
}
func (f *fakeRepo) GetOffer(context.Context, int64) (Offer, bool, error) {
	return Offer{}, false, nil
}

func (f *fakeRepo) CreateRequest(_ context.Context, in RequestInput) (Request, error) {
	f.createdReq = &in
	if f.createErr != nil {
		return Request{}, f.createErr
	}
	return Request{ID: 7, SeekerUserID: in.SeekerUserID, CompanySlug: in.CompanySlug, Status: RequestSent}, nil
}

func (f *fakeRepo) CountRequestsSince(_ context.Context, _ int64, _ time.Time) (int64, error) {
	return f.countSince, nil
}

func (f *fakeRepo) GetRequest(context.Context, int64) (Request, bool, error) {
	return f.getRequest, f.getRequestOK, nil
}

func (f *fakeRepo) ResolveRequest(_ context.Context, id, actorID int64, status string) (Request, error) {
	f.resolved = &resolvedRequest{id, actorID, status}
	if f.resolveErr != nil {
		return Request{}, f.resolveErr
	}
	return Request{ID: id, Status: status, ActedBy: &actorID}, nil
}

func (f *fakeRepo) ListRequestsBySeeker(context.Context, int64) ([]Request, error) { return nil, nil }
func (f *fakeRepo) ListIncomingRequests(context.Context, int64) ([]Request, error) { return nil, nil }

// fakePinger records who it was asked to ping and can be told to fail.
type fakePinger struct {
	pinged []int64
	err    error
}

func (p *fakePinger) PingReferrer(_ context.Context, r Recipient, _ string) error {
	p.pinged = append(p.pinged, r.UserID)
	return p.err
}

func newService(repo *fakeRepo, pinger *fakePinger) *Service {
	return New(repo, pinger, Config{DailyRequestCap: 3, CabinetURL: "https://freehire.dev/my/referrals"})
}

func cvID(n int64) *int64 { return &n }

// --- offer ---------------------------------------------------------------

func TestSubmitOfferRequiresProof(t *testing.T) {
	repo := &fakeRepo{}
	s := newService(repo, &fakePinger{})
	if _, err := s.SubmitOffer(context.Background(), OfferInput{UserID: 1, CompanySlug: "acme", ProofKey: "  "}); !errors.Is(err, ErrProofRequired) {
		t.Fatalf("err = %v, want ErrProofRequired", err)
	}
	if repo.createdOff != nil {
		t.Error("repo.CreateOffer should not run when proof is missing")
	}
}

func TestDecideOfferMapsApproveReject(t *testing.T) {
	for _, tc := range []struct {
		approve bool
		want    string
	}{{true, OfferApproved}, {false, OfferRejected}} {
		repo := &fakeRepo{}
		s := newService(repo, &fakePinger{})
		if _, err := s.DecideOffer(context.Background(), 5, 9, tc.approve); err != nil {
			t.Fatalf("decide: %v", err)
		}
		if repo.decided == nil || repo.decided.status != tc.want || repo.decided.moderator != 9 {
			t.Errorf("decided = %+v, want status %q by 9", repo.decided, tc.want)
		}
	}
}

// --- request creation ----------------------------------------------------

func TestCreateRequestValidation(t *testing.T) {
	base := RequestInput{SeekerUserID: 1, CompanySlug: "acme", CVKind: CVOriginal, ContactEmail: "s@x.test"}

	tests := []struct {
		name string
		in   RequestInput
		want error
	}{
		{"no contact", RequestInput{SeekerUserID: 1, CompanySlug: "acme", CVKind: CVOriginal}, ErrNoContact},
		{"original with cv id", withCV(base, CVOriginal, cvID(4)), ErrInvalidCVChoice},
		{"built without cv id", withCV(base, CVBuilt, nil), ErrInvalidCVChoice},
		{"unknown kind", withCV(base, "weird", nil), ErrInvalidCVChoice},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{eligible: true}
			s := newService(repo, &fakePinger{})
			if _, err := s.CreateRequest(context.Background(), tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if repo.createdReq != nil {
				t.Error("repo.CreateRequest should not run when validation fails")
			}
		})
	}
}

func TestCreateRequestEligibilityAndCap(t *testing.T) {
	valid := RequestInput{SeekerUserID: 1, CompanySlug: "acme", CVKind: CVOriginal, ContactEmail: "s@x.test"}

	t.Run("company not eligible", func(t *testing.T) {
		repo := &fakeRepo{eligible: false, hasResume: true}
		s := newService(repo, &fakePinger{})
		if _, err := s.CreateRequest(context.Background(), valid); !errors.Is(err, ErrCompanyNotEligible) {
			t.Fatalf("err = %v, want ErrCompanyNotEligible", err)
		}
	})

	t.Run("daily cap reached", func(t *testing.T) {
		repo := &fakeRepo{eligible: true, hasResume: true, countSince: 3} // cap is 3
		s := newService(repo, &fakePinger{})
		if _, err := s.CreateRequest(context.Background(), valid); !errors.Is(err, ErrDailyCapReached) {
			t.Fatalf("err = %v, want ErrDailyCapReached", err)
		}
		if repo.createdReq != nil {
			t.Error("request should not be written once the cap is hit")
		}
	})

	t.Run("under cap writes and pings all referrers", func(t *testing.T) {
		repo := &fakeRepo{eligible: true, hasResume: true, countSince: 2, recipients: []Recipient{{UserID: 10}, {UserID: 11}}}
		pinger := &fakePinger{}
		s := newService(repo, pinger)
		req, err := s.CreateRequest(context.Background(), valid)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if req.Status != RequestSent {
			t.Errorf("status = %q, want sent", req.Status)
		}
		if len(pinger.pinged) != 2 {
			t.Errorf("pinged %v, want both referrers", pinger.pinged)
		}
	})

	t.Run("ping failure does not fail the request", func(t *testing.T) {
		repo := &fakeRepo{eligible: true, hasResume: true, recipients: []Recipient{{UserID: 10}}}
		s := newService(repo, &fakePinger{err: errors.New("smtp down")})
		if _, err := s.CreateRequest(context.Background(), valid); err != nil {
			t.Fatalf("create should swallow ping errors, got %v", err)
		}
	})
}

func TestCreateRequestBuiltCVOwnership(t *testing.T) {
	built := RequestInput{SeekerUserID: 1, CompanySlug: "acme", CVKind: CVBuilt, CVID: cvID(42), ContactEmail: "s@x.test"}

	t.Run("foreign cv is rejected as an invalid choice", func(t *testing.T) {
		repo := &fakeRepo{eligible: true, cvOwned: false}
		s := newService(repo, &fakePinger{})
		if _, err := s.CreateRequest(context.Background(), built); !errors.Is(err, ErrInvalidCVChoice) {
			t.Fatalf("err = %v, want ErrInvalidCVChoice", err)
		}
		if repo.createdReq != nil {
			t.Error("must not write a request attaching a CV the seeker does not own")
		}
	})

	t.Run("owned cv is accepted", func(t *testing.T) {
		repo := &fakeRepo{eligible: true, cvOwned: true}
		s := newService(repo, &fakePinger{})
		if _, err := s.CreateRequest(context.Background(), built); err != nil {
			t.Fatalf("create with owned CV: %v", err)
		}
		if repo.createdReq == nil {
			t.Error("owned built CV should produce a request")
		}
	})
}

func TestCreateRequestOriginalNeedsResume(t *testing.T) {
	original := RequestInput{SeekerUserID: 1, CompanySlug: "acme", CVKind: CVOriginal, ContactEmail: "s@x.test"}
	repo := &fakeRepo{eligible: true, hasResume: false}
	s := newService(repo, &fakePinger{})
	if _, err := s.CreateRequest(context.Background(), original); !errors.Is(err, ErrNoResume) {
		t.Fatalf("err = %v, want ErrNoResume", err)
	}
	if repo.createdReq != nil {
		t.Error("must not write an original request when the seeker has no résumé")
	}
}

// --- resolve + cv access -------------------------------------------------

func TestResolveRequestAuthorization(t *testing.T) {
	t.Run("missing request", func(t *testing.T) {
		repo := &fakeRepo{getRequestOK: false}
		s := newService(repo, &fakePinger{})
		if _, err := s.ResolveRequest(context.Background(), 7, 9, true); !errors.Is(err, ErrRequestNotFound) {
			t.Fatalf("err = %v, want ErrRequestNotFound", err)
		}
	})

	t.Run("not an approved referrer", func(t *testing.T) {
		repo := &fakeRepo{getRequestOK: true, getRequest: Request{ID: 7, CompanySlug: "acme"}, approved: false}
		s := newService(repo, &fakePinger{})
		if _, err := s.ResolveRequest(context.Background(), 7, 9, true); !errors.Is(err, ErrNotAuthorized) {
			t.Fatalf("err = %v, want ErrNotAuthorized", err)
		}
		if repo.resolved != nil {
			t.Error("must not resolve when unauthorized")
		}
	})

	t.Run("authorized decline maps status", func(t *testing.T) {
		repo := &fakeRepo{getRequestOK: true, getRequest: Request{ID: 7, CompanySlug: "acme"}, approved: true}
		s := newService(repo, &fakePinger{})
		if _, err := s.ResolveRequest(context.Background(), 7, 9, false); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if repo.resolved == nil || repo.resolved.status != RequestDeclined || repo.resolved.actor != 9 {
			t.Errorf("resolved = %+v, want declined by 9", repo.resolved)
		}
	})
}

func TestAuthorizeCVAccess(t *testing.T) {
	repo := &fakeRepo{getRequestOK: true, getRequest: Request{ID: 7, CompanySlug: "acme", CVKind: CVOriginal}, approved: true}
	s := newService(repo, &fakePinger{})
	got, err := s.AuthorizeCVAccess(context.Background(), 7, 9)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if got.ID != 7 {
		t.Errorf("request id = %d, want 7", got.ID)
	}

	repo.approved = false
	if _, err := s.AuthorizeCVAccess(context.Background(), 7, 9); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("err = %v, want ErrNotAuthorized", err)
	}
}

// withCV returns a copy of in with the CV choice replaced.
func withCV(in RequestInput, kind string, id *int64) RequestInput {
	in.CVKind = kind
	in.CVID = id
	return in
}
