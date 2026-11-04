package resume

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/strelov1/freehire/internal/platform/db"
)

type failingUploadBlobs struct{ *fakeBlobs }

func (f failingUploadBlobs) Put(context.Context, string, string, io.Reader, int64) error {
	return errors.New("object storage unavailable")
}

type failingUploadRepo struct{ *fakeRepo }

func (f failingUploadRepo) Set(context.Context, int64, string) error {
	return errors.New("database unavailable")
}

type failingReadbackRepo struct{ *fakeRepo }

func (f failingReadbackRepo) Get(context.Context, int64) (db.GetUserResumeRow, error) {
	return db.GetUserResumeRow{}, errors.New("readback failed after saving")
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

// A metadata read failure cannot undo an upload whose object and pointer were saved.
func TestUploadMetricsCountPersistenceEvenWhenReadbackFails(t *testing.T) {
	before := uploadCount(t, "success")
	store := New(newFakeBlobs(), failingReadbackRepo{newFakeRepo()})
	if _, err := store.Put(context.Background(), 1, "text/plain", []byte("CV")); err == nil {
		t.Fatal("expected metadata readback failure")
	}
	if got := uploadCount(t, "success") - before; got != 1 {
		t.Fatalf("persisted uploads = %v, want 1", got)
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
