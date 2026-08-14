package referral

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/blobstore"
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
	deleteErr    error
	getOffer     Offer
	getOfferOK   bool
	getOfferErr  error

	createdReq *RequestInput
	createdOff *OfferInput
	decided    *decidedOffer
	deleted    *deletedOffer
	resolved   *resolvedRequest
}

type decidedOffer struct {
	id        uuid.UUID
	moderator int64
	status    string
}

type deletedOffer struct {
	id   uuid.UUID
	user int64
}

type resolvedRequest struct {
	id     uuid.UUID
	actor  int64
	status string
}

func (f *fakeRepo) CreateOffer(_ context.Context, in OfferInput) (Offer, error) {
	f.createdOff = &in
	if f.createErr != nil {
		return Offer{}, f.createErr
	}
	return Offer{ID: testOfferID, UserID: in.UserID, CompanySlug: in.CompanySlug, Status: OfferPending}, nil
}

func (f *fakeRepo) DecideOffer(_ context.Context, offerID uuid.UUID, moderatorID int64, status string) (Offer, error) {
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

// The ids these cases address. Fixed so a failure names a stable value.
var (
	testOfferID   = uuid.MustParse("aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa")
	testRequestID = uuid.MustParse("bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb")
)

// testCVID is the attached CV these cases address.
var testCVID = uuid.MustParse("77777777-7777-4777-8777-777777777777")

func (f *fakeRepo) CVBelongsToUser(context.Context, uuid.UUID, int64) (bool, error) {
	return f.cvOwned, nil
}
func (f *fakeRepo) UserHasResume(context.Context, int64) (bool, error) {
	return f.hasResume, nil
}
func (f *fakeRepo) GetOffer(context.Context, uuid.UUID) (Offer, bool, error) {
	return f.getOffer, f.getOfferOK, f.getOfferErr
}
func (f *fakeRepo) DeleteOffer(_ context.Context, offerID uuid.UUID, userID int64) error {
	f.deleted = &deletedOffer{offerID, userID}
	return f.deleteErr
}

func (f *fakeRepo) CreateRequest(_ context.Context, in RequestInput) (Request, error) {
	f.createdReq = &in
	if f.createErr != nil {
		return Request{}, f.createErr
	}
	return Request{ID: testRequestID, SeekerUserID: in.SeekerUserID, CompanySlug: in.CompanySlug, Status: RequestSent}, nil
}

func (f *fakeRepo) CountRequestsSince(_ context.Context, _ int64, _ time.Time) (int64, error) {
	return f.countSince, nil
}

func (f *fakeRepo) GetRequest(context.Context, uuid.UUID) (Request, bool, error) {
	return f.getRequest, f.getRequestOK, nil
}

func (f *fakeRepo) ResolveRequest(_ context.Context, id uuid.UUID, actorID int64, status string) (Request, error) {
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

// fakeBlobs is an in-memory blobstore.Store recording the keys it was asked to delete. Only
// Delete is exercised here — the service never reads or writes an object.
type fakeBlobs struct {
	deleted []string
	err     error
}

func (f *fakeBlobs) Put(context.Context, string, string, io.Reader, int64) error { return nil }
func (f *fakeBlobs) Get(context.Context, string) (io.ReadCloser, error)          { return nil, nil }
func (f *fakeBlobs) Delete(_ context.Context, key string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, key)
	return nil
}

func newService(repo *fakeRepo, pinger *fakePinger) *Service {
	return newServiceWithBlobs(repo, pinger, nil)
}

func newServiceWithBlobs(repo *fakeRepo, pinger *fakePinger, blobs blobstore.Store) *Service {
	return New(repo, pinger, blobs, Config{DailyRequestCap: 3, CabinetURL: "https://freehire.me/my/referrals"})
}

// ownedOffer is a stored offer belonging to userID, carrying a proof object.
func ownedOffer(userID int64, proofKey string) Offer {
	return Offer{ID: testOfferID, UserID: userID, CompanySlug: "acme", ProofKey: proofKey, Status: OfferApproved}
}

func cvID(n uuid.UUID) *uuid.UUID { return &n }

// linkedInURL is a valid profile URL for fixtures that must clear the required-LinkedIn gate.
const linkedInURL = "https://www.linkedin.com/in/jane-doe"

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

func TestSubmitOfferRequiresLinkedIn(t *testing.T) {
	repo := &fakeRepo{}
	s := newService(repo, &fakePinger{})
	if _, err := s.SubmitOffer(context.Background(), OfferInput{UserID: 1, CompanySlug: "acme", ProofKey: "k", LinkedInURL: "https://twitter.com/jane"}); !errors.Is(err, ErrInvalidLinkedIn) {
		t.Fatalf("err = %v, want ErrInvalidLinkedIn", err)
	}
	if repo.createdOff != nil {
		t.Error("repo.CreateOffer should not run when the LinkedIn URL is invalid")
	}
	if _, err := s.SubmitOffer(context.Background(), OfferInput{UserID: 1, CompanySlug: "acme", ProofKey: "k", LinkedInURL: linkedInURL}); err != nil {
		t.Fatalf("valid offer: %v", err)
	}
	if repo.createdOff == nil || repo.createdOff.LinkedInURL != linkedInURL {
		t.Errorf("offer LinkedIn not persisted: %+v", repo.createdOff)
	}
}

func TestValidLinkedInURL(t *testing.T) {
	valid := []string{
		"https://www.linkedin.com/in/jane-doe",
		"https://linkedin.com/in/jane-doe/",
		"http://de.linkedin.com/in/hans",
		"https://www.linkedin.com/in/jane-doe?trk=x",
	}
	for _, s := range valid {
		if !validLinkedInURL(s) {
			t.Errorf("validLinkedInURL(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"", "   ", "linkedin.com/in/jane", "https://linkedin.com/", "https://linkedin.com/company/acme",
		"https://notlinkedin.com/in/jane", "https://linkedin.com.evil.com/in/jane", "ftp://linkedin.com/in/jane",
		// Otherwise well-formed, but past maxLinkedInURLLen — an unbounded payload
		// riding along inside a field with no explicit length check before this.
		"https://www.linkedin.com/in/" + strings.Repeat("j", maxLinkedInURLLen),
	}
	for _, s := range invalid {
		if validLinkedInURL(s) {
			t.Errorf("validLinkedInURL(%q) = true, want false", s)
		}
	}
}

func TestDecideOfferMapsApproveReject(t *testing.T) {
	for _, tc := range []struct {
		approve bool
		want    string
	}{{true, OfferApproved}, {false, OfferRejected}} {
		repo := &fakeRepo{}
		s := newService(repo, &fakePinger{})
		if _, err := s.DecideOffer(context.Background(), testOfferID, 9, tc.approve); err != nil {
			t.Fatalf("decide: %v", err)
		}
		if repo.decided == nil || repo.decided.status != tc.want || repo.decided.moderator != 9 {
			t.Errorf("decided = %+v, want status %q by 9", repo.decided, tc.want)
		}
	}
}

func TestWithdrawOfferIsOwnerScoped(t *testing.T) {
	repo := &fakeRepo{getOffer: ownedOffer(42, ""), getOfferOK: true}
	s := newService(repo, &fakePinger{})
	if err := s.WithdrawOffer(context.Background(), testOfferID, 42); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if repo.deleted == nil || repo.deleted.id != testOfferID || repo.deleted.user != 42 {
		t.Errorf("deleted = %+v, want offer 5 by user 42", repo.deleted)
	}
}

func TestWithdrawOfferPropagatesNotFound(t *testing.T) {
	repo := &fakeRepo{getOffer: ownedOffer(42, ""), getOfferOK: true, deleteErr: ErrOfferNotFound}
	s := newService(repo, &fakePinger{})
	if err := s.WithdrawOffer(context.Background(), testOfferID, 42); !errors.Is(err, ErrOfferNotFound) {
		t.Errorf("err = %v, want ErrOfferNotFound", err)
	}
}

// Withdrawing hard-deletes the offer row, and that row is the ONLY thing that names the
// proof object: accountdelete finds a member's objects by reading these very rows
// (ListUserBlobKeys), so a row deleted without its object leaves a CV in the bucket that
// nothing can ever reach again — including the member's own account deletion.
func TestWithdrawOfferDeletesTheProofObject(t *testing.T) {
	repo := &fakeRepo{getOffer: ownedOffer(42, "referral-proofs/42/proof.pdf"), getOfferOK: true}
	blobs := &fakeBlobs{}
	s := newServiceWithBlobs(repo, &fakePinger{}, blobs)

	if err := s.WithdrawOffer(context.Background(), testOfferID, 42); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if len(blobs.deleted) != 1 || blobs.deleted[0] != "referral-proofs/42/proof.pdf" {
		t.Errorf("deleted objects = %v, want the offer's proof key", blobs.deleted)
	}
	if repo.deleted == nil {
		t.Error("the offer row was not deleted")
	}
}

// Objects go before rows, the same order accountdelete uses and for the same reason: if the
// object cannot be erased, leaving the row means the member can simply retry, whereas
// deleting it first would strand the object with no key left to find it by.
func TestWithdrawOfferKeepsTheOfferWhenTheProofCannotBeDeleted(t *testing.T) {
	repo := &fakeRepo{getOffer: ownedOffer(42, "referral-proofs/42/proof.pdf"), getOfferOK: true}
	blobs := &fakeBlobs{err: errors.New("s3 down")}
	s := newServiceWithBlobs(repo, &fakePinger{}, blobs)

	err := s.WithdrawOffer(context.Background(), testOfferID, 42)
	if !errors.Is(err, ErrProofStorageUnavailable) {
		t.Fatalf("err = %v, want ErrProofStorageUnavailable", err)
	}
	if repo.deleted != nil {
		t.Error("the offer row must survive a storage failure so the withdrawal can be retried")
	}
}

func TestWithdrawOfferRefusesAnotherMembersOffer(t *testing.T) {
	repo := &fakeRepo{getOffer: ownedOffer(7, "referral-proofs/7/proof.pdf"), getOfferOK: true}
	blobs := &fakeBlobs{}
	s := newServiceWithBlobs(repo, &fakePinger{}, blobs)

	if err := s.WithdrawOffer(context.Background(), testOfferID, 42); !errors.Is(err, ErrOfferNotFound) {
		t.Errorf("err = %v, want ErrOfferNotFound", err)
	}
	if len(blobs.deleted) != 0 {
		t.Errorf("deleted %v — a non-owner must not reach another member's proof", blobs.deleted)
	}
	if repo.deleted != nil {
		t.Error("a non-owner must not delete the row")
	}
}

// --- request creation ----------------------------------------------------

func TestCreateRequestValidation(t *testing.T) {
	base := RequestInput{SeekerUserID: 1, CompanySlug: "acme", CVKind: CVOriginal, ContactEmail: "s@x.test", LinkedInURL: linkedInURL}

	tests := []struct {
		name string
		in   RequestInput
		want error
	}{
		{"no contact", RequestInput{SeekerUserID: 1, CompanySlug: "acme", CVKind: CVOriginal}, ErrNoContact},
		{"original with cv id", withCV(base, CVOriginal, cvID(testCVID)), ErrInvalidCVChoice},
		{"built without cv id", withCV(base, CVBuilt, nil), ErrInvalidCVChoice},
		{"unknown kind", withCV(base, "weird", nil), ErrInvalidCVChoice},
		{"note too long", withNote(base, strings.Repeat("n", maxNoteLen+1)), ErrNoteTooLong},
		{"telegram contact too long", withTelegram(base, strings.Repeat("t", maxContactLen+1)), ErrContactTooLong},
		{"email contact too long", withEmail(base, strings.Repeat("e", maxContactLen+1)+"@x.test"), ErrContactTooLong},
		{"linkedin url too long", withLinkedIn(base, "https://www.linkedin.com/in/"+strings.Repeat("j", maxLinkedInURLLen)), ErrInvalidLinkedIn},
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
	valid := RequestInput{SeekerUserID: 1, CompanySlug: "acme", CVKind: CVOriginal, ContactEmail: "s@x.test", LinkedInURL: linkedInURL}

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
	built := RequestInput{SeekerUserID: 1, CompanySlug: "acme", CVKind: CVBuilt, CVID: cvID(testCVID), ContactEmail: "s@x.test", LinkedInURL: linkedInURL}

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
	original := RequestInput{SeekerUserID: 1, CompanySlug: "acme", CVKind: CVOriginal, ContactEmail: "s@x.test", LinkedInURL: linkedInURL}
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
		if _, err := s.ResolveRequest(context.Background(), testRequestID, 9, true); !errors.Is(err, ErrRequestNotFound) {
			t.Fatalf("err = %v, want ErrRequestNotFound", err)
		}
	})

	t.Run("not an approved referrer", func(t *testing.T) {
		repo := &fakeRepo{getRequestOK: true, getRequest: Request{ID: testRequestID, CompanySlug: "acme"}, approved: false}
		s := newService(repo, &fakePinger{})
		if _, err := s.ResolveRequest(context.Background(), testRequestID, 9, true); !errors.Is(err, ErrNotAuthorized) {
			t.Fatalf("err = %v, want ErrNotAuthorized", err)
		}
		if repo.resolved != nil {
			t.Error("must not resolve when unauthorized")
		}
	})

	t.Run("authorized decline maps status", func(t *testing.T) {
		repo := &fakeRepo{getRequestOK: true, getRequest: Request{ID: testRequestID, CompanySlug: "acme"}, approved: true}
		s := newService(repo, &fakePinger{})
		if _, err := s.ResolveRequest(context.Background(), testRequestID, 9, false); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if repo.resolved == nil || repo.resolved.status != RequestDeclined || repo.resolved.actor != 9 {
			t.Errorf("resolved = %+v, want declined by 9", repo.resolved)
		}
	})
}

func TestAuthorizeCVAccess(t *testing.T) {
	repo := &fakeRepo{getRequestOK: true, getRequest: Request{ID: testRequestID, CompanySlug: "acme", CVKind: CVOriginal}, approved: true}
	s := newService(repo, &fakePinger{})
	got, err := s.AuthorizeCVAccess(context.Background(), testRequestID, 9)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if got.ID != testRequestID {
		t.Errorf("request id = %s, want %s", got.ID, testRequestID)
	}

	repo.approved = false
	if _, err := s.AuthorizeCVAccess(context.Background(), testRequestID, 9); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("err = %v, want ErrNotAuthorized", err)
	}
}

// withCV returns a copy of in with the CV choice replaced.
func withCV(in RequestInput, kind string, id *uuid.UUID) RequestInput {
	in.CVKind = kind
	in.CVID = id
	return in
}

// withNote returns a copy of in with Note replaced.
func withNote(in RequestInput, note string) RequestInput {
	in.Note = note
	return in
}

// withTelegram returns a copy of in with ContactTelegram replaced.
func withTelegram(in RequestInput, telegram string) RequestInput {
	in.ContactTelegram = telegram
	return in
}

// withEmail returns a copy of in with ContactEmail replaced.
func withEmail(in RequestInput, email string) RequestInput {
	in.ContactEmail = email
	return in
}

// withLinkedIn returns a copy of in with LinkedInURL replaced.
func withLinkedIn(in RequestInput, url string) RequestInput {
	in.LinkedInURL = url
	return in
}
