package resume

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/client_golang/prometheus"
)

type failingUploadBlobs struct{ *fakeBlobs }

func (f failingUploadBlobs) Put(context.Context, string, string, io.Reader, int64) error {
	return errors.New("object storage unavailable")
}

type failingUploadRepo struct{ *fakeRepo }

func (f failingUploadRepo) Set(context.Context, int64, string) error {
	return errors.New("database unavailable")
}

// WithUploadLock hands fn this wrapper, not the embedded fake, so the failing Set is the one
// the upload reaches.
func (f failingUploadRepo) WithUploadLock(_ context.Context, _ int64, fn func(Repository) error) error {
	return fn(f)
}

// racingUploadRepo lets another upload commit right after this one: once fn returns, the
// pointer's stamp moves on, as a concurrent upload's commit would move it.
type racingUploadRepo struct{ *fakeRepo }

func (r racingUploadRepo) WithUploadLock(ctx context.Context, userID int64, fn func(Repository) error) error {
	if err := fn(r.fakeRepo); err != nil {
		return err
	}
	r.uploadedAt[userID] = pgtype.Timestamptz{Time: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	return nil
}

// uploadCount reads the same process registry that the metrics listener exports.
func uploadCount(t *testing.T, outcome string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "freehire_cv_uploads_total" {
			continue
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "outcome" && label.GetValue() == outcome {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// The stamp an upload returns is the one it wrote, read under its lock — never a later
// upload's, which would let this upload's extraction pass the stamp guard as that one's.
func TestPutReturnsTheStampItWrote(t *testing.T) {
	repo := racingUploadRepo{newFakeRepo()}
	meta, err := New(newFakeBlobs(), repo).Put(context.Background(), 1, "text/plain", []byte("CV"))
	if err != nil {
		t.Fatal(err)
	}
	if meta.UploadedAt == nil || meta.UploadedAt.Year() == 2030 {
		t.Fatalf("UploadedAt = %v, want the stamp this upload wrote", meta.UploadedAt)
	}
}

// Disabled storage is a deliberate mode, not a failed persistence attempt.
func TestUploadMetricsExcludeDisabledStorage(t *testing.T) {
	before := uploadCount(t, "error")
	if _, err := New(nil, nil).Put(context.Background(), 1, "text/plain", nil); !errors.Is(err, ErrStorageDisabled) {
		t.Fatalf("disabled storage error = %v", err)
	}
	if got := uploadCount(t, "error") - before; got != 0 {
		t.Fatalf("storage attempts while disabled = %v, want 0", got)
	}
}

// An object without a saved pointer must not be reported as a successful upload.
func TestUploadMetricsRequireBothObjectAndPointer(t *testing.T) {
	for _, tt := range []struct {
		name      string
		store     *Store
		wantError bool
	}{
		{"stored", New(newFakeBlobs(), newFakeRepo()), false},
		{"object failure", New(failingUploadBlobs{newFakeBlobs()}, newFakeRepo()), true},
		{"pointer failure", New(newFakeBlobs(), failingUploadRepo{newFakeRepo()}), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			success, failure := uploadCount(t, "success"), uploadCount(t, "error")
			_, err := tt.store.Put(context.Background(), 1, "text/plain", []byte("CV"))
			if (err != nil) != tt.wantError {
				t.Fatalf("Put error = %v", err)
			}
			wantSuccess, wantFailure := success+1, failure
			if tt.wantError {
				wantSuccess, wantFailure = success, failure+1
			}
			if got := uploadCount(t, "success"); got != wantSuccess {
				t.Errorf("successful uploads = %v, want %v", got, wantSuccess)
			}
			if got := uploadCount(t, "error"); got != wantFailure {
				t.Errorf("failed uploads = %v, want %v", got, wantFailure)
			}
		})
	}
}
