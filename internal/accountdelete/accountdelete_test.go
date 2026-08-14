package accountdelete

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeRepo records the calls the service makes and can fail either of them.
type fakeRepo struct {
	keys       []string
	keysErr    error
	deleteErr  error
	calls      *[]string
	deleteSeen bool
}

func (r *fakeRepo) BlobKeys(_ context.Context, _ int64) ([]string, error) {
	*r.calls = append(*r.calls, "list-keys")
	return r.keys, r.keysErr
}

func (r *fakeRepo) DeleteUser(_ context.Context, _ int64) error {
	*r.calls = append(*r.calls, "delete-rows")
	r.deleteSeen = true
	return r.deleteErr
}

// fakeBlobs records deleted keys and can fail the delete.
type fakeBlobs struct {
	deleted []string
	err     error
	calls   *[]string
}

func (b *fakeBlobs) Put(context.Context, string, string, io.Reader, int64) error { return nil }
func (b *fakeBlobs) Get(context.Context, string) (io.ReadCloser, error)          { return nil, nil }

func (b *fakeBlobs) Delete(_ context.Context, key string) error {
	*b.calls = append(*b.calls, "delete-object:"+key)
	if b.err != nil {
		return b.err
	}
	b.deleted = append(b.deleted, key)
	return nil
}

// The order is the whole design: keys are only knowable while the rows exist, and
// objects must be gone before the rows are, or a failed delete strands PII in the
// bucket with no key left to find it by.
func TestDelete_ErasesObjectsBeforeRows(t *testing.T) {
	var calls []string
	repo := &fakeRepo{keys: []string{"resumes/7", "referral-proof/7/acme.pdf"}, calls: &calls}
	blobs := &fakeBlobs{calls: &calls}
	revoked := 0
	svc := New(repo, blobs, func(context.Context, int64) error { revoked++; return nil })

	if err := svc.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got := strings.Join(calls, " ")
	if !strings.HasPrefix(got, "list-keys") {
		t.Errorf("calls = %q, want the key collection first", got)
	}
	if calls[len(calls)-1] != "delete-rows" {
		t.Errorf("calls = %q, want the row delete last", got)
	}
	if len(blobs.deleted) != 2 {
		t.Errorf("deleted objects = %v, want both keys", blobs.deleted)
	}
	if revoked != 1 {
		t.Errorf("revoke called %d times, want 1", revoked)
	}
}

// A storage failure must abort before any row is deleted: rows-gone-objects-left is
// unrecoverable, objects-gone-rows-left is a retry.
func TestDelete_StorageFailureLeavesTheAccountIntact(t *testing.T) {
	var calls []string
	repo := &fakeRepo{keys: []string{"resumes/7"}, calls: &calls}
	blobs := &fakeBlobs{err: errors.New("connection reset"), calls: &calls}
	svc := New(repo, blobs, nil)

	err := svc.Delete(context.Background(), 7)
	if !errors.Is(err, ErrStorageUnavailable) {
		t.Errorf("err = %v, want ErrStorageUnavailable", err)
	}
	if repo.deleteSeen {
		t.Error("rows were deleted despite the storage failure")
	}
}

// The token is discarded either way, so this system cannot use the grant again;
// blocking deletion on Google's availability would trap the member instead.
func TestDelete_RevokeFailureDoesNotBlockDeletion(t *testing.T) {
	var calls []string
	repo := &fakeRepo{calls: &calls}
	blobs := &fakeBlobs{calls: &calls}
	svc := New(repo, blobs, func(context.Context, int64) error {
		return errors.New("google timeout")
	})

	if err := svc.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !repo.deleteSeen {
		t.Error("account was not deleted after a failed revoke")
	}
}

// The gateway credential names this account on a system we do not own, so it goes with
// the account — and it must go BEFORE the row, because the row is the only place its
// value is written down. Erasing the row first would leave a live credential nothing can
// name, spending under an account that no longer exists.
func TestDelete_BlocksTheGatewayCredentialBeforeTheRow(t *testing.T) {
	var calls []string
	repo := &fakeRepo{calls: &calls}
	svc := New(repo, nil, nil).WithGatewayKeys(func(context.Context, int64) error {
		calls = append(calls, "block-gateway-key")
		return nil
	})

	if err := svc.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	blocked, deleted := -1, -1
	for i, c := range calls {
		switch c {
		case "block-gateway-key":
			blocked = i
		case "delete-rows":
			deleted = i
		}
	}
	if blocked < 0 {
		t.Fatalf("calls = %v, want the gateway credential blocked", calls)
	}
	if deleted < 0 || blocked > deleted {
		t.Errorf("calls = %v, want the credential blocked before the row is erased", calls)
	}
}

// The same rule the Google grant follows: a member leaving must not be held up by a third
// system's availability, and the credential is worthless once its account is gone.
func TestDelete_GatewayFailureDoesNotBlockDeletion(t *testing.T) {
	var calls []string
	repo := &fakeRepo{calls: &calls}
	svc := New(repo, nil, nil).WithGatewayKeys(func(context.Context, int64) error {
		return errors.New("gateway down")
	})

	if err := svc.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !repo.deleteSeen {
		t.Error("account was not deleted after a failed credential block")
	}
}

// Storage and Gmail are both optional deployments-wide. A nil dependency means
// "nothing to erase there", never an error — otherwise a self-hosted instance with
// no S3 could never delete an account.
func TestDelete_WorksWithoutStorageOrGmail(t *testing.T) {
	var calls []string
	repo := &fakeRepo{calls: &calls}
	svc := New(repo, nil, nil)

	if err := svc.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !repo.deleteSeen {
		t.Error("account was not deleted when storage and Gmail are unconfigured")
	}
}

// A failure to read the keys is also a hard stop: deleting the rows blind would
// strand whatever objects the account owns.
func TestDelete_KeyReadFailureAborts(t *testing.T) {
	var calls []string
	repo := &fakeRepo{keysErr: errors.New("query failed"), calls: &calls}
	blobs := &fakeBlobs{calls: &calls}
	svc := New(repo, blobs, nil)

	if err := svc.Delete(context.Background(), 7); err == nil {
		t.Fatal("Delete succeeded despite an unreadable key list")
	}
	if repo.deleteSeen {
		t.Error("rows were deleted without knowing the object keys")
	}
}

// Apple grants reference users with ON DELETE RESTRICT, so releasing them runs
// before the row delete, and — unlike the best-effort Gmail/gateway steps —
// its failure must abort deletion rather than let DeleteUser hit the FK.
func TestDelete_ReleasesAppleGrantsBeforeTheRow(t *testing.T) {
	var calls []string
	repo := &fakeRepo{calls: &calls}
	svc := New(repo, nil, nil).WithAppleGrants(func(context.Context, int64) error {
		calls = append(calls, "release-apple-grants")
		return nil
	})

	if err := svc.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	released, deleted := -1, -1
	for i, c := range calls {
		switch c {
		case "release-apple-grants":
			released = i
		case "delete-rows":
			deleted = i
		}
	}
	if released < 0 {
		t.Fatalf("calls = %v, want apple grants released", calls)
	}
	if deleted < 0 || released > deleted {
		t.Errorf("calls = %v, want apple grants released before the row is erased", calls)
	}
}

func TestDelete_AppleGrantsFailureBlocksDeletion(t *testing.T) {
	var calls []string
	repo := &fakeRepo{calls: &calls}
	svc := New(repo, nil, nil).WithAppleGrants(func(context.Context, int64) error {
		return errors.New("still has an active apple grant")
	})

	if err := svc.Delete(context.Background(), 7); err == nil {
		t.Fatal("Delete succeeded despite a failed apple grant release")
	}
	if repo.deleteSeen {
		t.Error("rows were deleted despite the FK still being blocked")
	}
}

// The blob objects are irrecoverable once deleted; a step that can still hard-fail
// (Apple grant release) must run before them, so a failure there leaves the objects —
// and the account — completely untouched rather than stranding dangling object keys
// on a row that survives because DeleteUser was never reached.
func TestDelete_AppleGrantsFailureLeavesObjectsUntouched(t *testing.T) {
	var calls []string
	repo := &fakeRepo{keys: []string{"resumes/7"}, calls: &calls}
	blobs := &fakeBlobs{calls: &calls}
	svc := New(repo, blobs, nil).WithAppleGrants(func(context.Context, int64) error {
		return errors.New("still has an active apple grant")
	})

	if err := svc.Delete(context.Background(), 7); err == nil {
		t.Fatal("Delete succeeded despite a failed apple grant release")
	}
	if len(calls) != 0 {
		t.Errorf("calls = %v, want no object-storage or row calls before the apple grant release fails", calls)
	}
	if len(blobs.deleted) != 0 {
		t.Errorf("deleted objects = %v, want none deleted", blobs.deleted)
	}
	if repo.deleteSeen {
		t.Error("rows were deleted despite the FK still being blocked")
	}
}
