package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/userjob"
)

// stubTrackingRepo is a DB-free jobtracking.Repository for the handler unit
// tests. The empty/bad-stage cases under test are rejected by the Service before
// it touches the repository, so these methods only need to exist (they panic if
// the validation order ever regresses and a request unexpectedly reaches them).
type stubTrackingRepo struct{}

func (stubTrackingRepo) JobIDBySlug(context.Context, string) (int64, error) { return 1, nil }
func (stubTrackingRepo) RecordView(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: 1}, nil
}
func (stubTrackingRepo) MarkApplied(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: 1}, nil
}
func (stubTrackingRepo) SaveJob(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: 1}, nil
}
func (stubTrackingRepo) UnsaveJob(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: 1}, nil
}
func (stubTrackingRepo) DismissJob(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: 1}, nil
}
func (stubTrackingRepo) UndismissJob(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: 1}, nil
}
func (stubTrackingRepo) TrackJob(context.Context, int64, int64, *string, *string) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: 1}, nil
}
func (stubTrackingRepo) ClearJobProgress(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: 1}, nil
}
func (stubTrackingRepo) UntrackJob(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: 1}, nil
}
func (stubTrackingRepo) ListInteractions(context.Context, int64, jobtracking.Filter, int32, int32) ([]jobtracking.TrackedJob, error) {
	return nil, nil
}
func (stubTrackingRepo) CountInteractions(context.Context, int64) (jobtracking.Counts, error) {
	return jobtracking.Counts{}, nil
}
func (stubTrackingRepo) ViewedSlugs(context.Context, int64) ([]string, error)    { return nil, nil }
func (stubTrackingRepo) SavedSlugs(context.Context, int64) ([]string, error)     { return nil, nil }
func (stubTrackingRepo) DismissedSlugs(context.Context, int64) ([]string, error) { return nil, nil }
func (stubTrackingRepo) ExcludedJobIDs(context.Context, int64, int32) ([]int64, error) {
	return nil, nil
}
func (stubTrackingRepo) PipelineCounts(context.Context, int64) ([]userjob.StageCount, error) {
	return nil, nil
}

// trackApp mounts the track route on a handler whose tracking service is backed
// by a stub repository (no DB). The auth gate and the service's body validation
// (empty / unknown stage) reject before any repository call runs. The DB-backed
// path is covered by the user_jobs integration tests.
func trackApp() (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{issuer: iss, tracking: jobtracking.New(stubTrackingRepo{})}
	app := fiber.New()
	app.Patch("/jobs/:slug/track", auth.RequireAuth(iss), h.TrackJob)
	return app, iss
}

func TestTrackJob_RejectsEmptyAndUnknownStage(t *testing.T) {
	app, iss := trackApp()
	token, _ := iss.Issue(7)
	cases := []struct {
		name, body string
	}{
		{"empty body", `{}`},
		{"unknown stage", `{"stage":"banana"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(fiber.MethodPatch, "/jobs/go-dev/track", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			if resp.StatusCode != fiber.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestTrackJob_RequiresAuth(t *testing.T) {
	app, _ := trackApp()
	req := httptest.NewRequest(fiber.MethodPatch, "/jobs/go-dev/track", strings.NewReader(`{"stage":"interview"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestIsValidStage(t *testing.T) {
	for _, s := range userjob.Stages {
		if !userjob.ValidStage(s) {
			t.Errorf("%q should be a valid stage", s)
		}
	}
	for _, s := range []string{"banana", "", "Applied", "interviewing"} {
		if userjob.ValidStage(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

// The interaction shape now carries stage and notes alongside the timestamps.
func TestInteractionResponse_HasStageAndNotes(t *testing.T) {
	fields := marshalToFields(t, interactionResponse{JobID: 7})
	for _, want := range []string{"job_id", "viewed_at", "saved_at", "applied_at", "stage", "notes"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("interactionResponse missing %q", want)
		}
	}
}
