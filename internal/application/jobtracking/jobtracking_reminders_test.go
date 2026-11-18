package jobtracking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/jobtracking"
)

// fakeReminders records the reminder side effects the service asks for. It is a
// two-method port, so a handwritten fake stays this small — which is the point of
// declaring the interface here rather than reaching for engage's own service.
type fakeReminders struct {
	scheduled []int64
	cancelled []int64
	err       error
}

func (f *fakeReminders) ScheduleOnSave(_ context.Context, _, jobID int64) error {
	f.scheduled = append(f.scheduled, jobID)
	return f.err
}

func (f *fakeReminders) Cancel(_ context.Context, _, jobID int64) error {
	f.cancelled = append(f.cancelled, jobID)
	return f.err
}

// TestSaveJob_SchedulesTheReminder is the regression this side effect was moved
// down for: it used to live in the Fiber handler, so a job saved through the in-app
// assistant — which calls this service directly and issues no HTTP request — never
// got a reminder row. The test drives the service, exactly as that caller does.
func TestSaveJob_SchedulesTheReminder(t *testing.T) {
	repo := newRepo()
	repo.saveResult = jobtracking.Interaction{JobID: jobID, SavedAt: tPtr(time.Now())}
	rem := &fakeReminders{}
	svc := jobtracking.New(repo, jobtracking.WithReminders(rem))

	if _, err := svc.SaveJob(ctx(), userID, slug); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rem.scheduled) != 1 || rem.scheduled[0] != jobID {
		t.Errorf("scheduled = %v, want exactly [%d]", rem.scheduled, jobID)
	}
	if len(rem.cancelled) != 0 {
		t.Errorf("cancelled = %v, want none", rem.cancelled)
	}
}

// TestEndingTheIntentCancelsTheReminder covers every way an application stops being
// pending. MarkAppliedAt is the mail-reconstruction path, which never cancelled at
// all while the rule lived at the HTTP door.
func TestEndingTheIntentCancelsTheReminder(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  func(*jobtracking.Service) error
	}{
		{"apply", func(s *jobtracking.Service) error {
			_, err := s.MarkApplied(ctx(), userID, slug, appevent.SourceAssistant)
			return err
		}},
		{"apply from mail", func(s *jobtracking.Service) error {
			_, err := s.MarkAppliedAt(ctx(), userID, slug, time.Now().Add(-time.Hour), appevent.SourceMailGmail)
			return err
		}},
		{"apply on a stated day", func(s *jobtracking.Service) error {
			day := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
			now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
			_, err := s.MarkAppliedOn(ctx(), userID, slug, day, now, appevent.SourceUser)
			return err
		}},
		{"unsave", func(s *jobtracking.Service) error {
			_, err := s.Unsave(ctx(), userID, slug)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepo()
			rem := &fakeReminders{}
			svc := jobtracking.New(repo, jobtracking.WithReminders(rem))

			if err := tc.act(svc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(rem.cancelled) != 1 || rem.cancelled[0] != jobID {
				t.Errorf("cancelled = %v, want exactly [%d]", rem.cancelled, jobID)
			}
			if len(rem.scheduled) != 0 {
				t.Errorf("scheduled = %v, want none", rem.scheduled)
			}
		})
	}
}

// TestUnsave_CancelsEvenWithNothingToClear pins the idempotent case: unsaving a job
// that carries no interaction row still withdraws the intent, because the caller
// asked for it and the reminder may exist regardless.
func TestUnsave_CancelsEvenWithNothingToClear(t *testing.T) {
	repo := newRepo()
	repo.unsaveErr = jobtracking.ErrNoInteraction
	rem := &fakeReminders{}
	svc := jobtracking.New(repo, jobtracking.WithReminders(rem))

	got, err := svc.Unsave(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JobID != jobID {
		t.Errorf("JobID = %d, want %d", got.JobID, jobID)
	}
	if len(rem.cancelled) != 1 {
		t.Errorf("cancelled = %v, want exactly one", rem.cancelled)
	}
}

// TestReminderIsBestEffort pins both halves of "best effort": a failing reminder
// never fails the tracking write, and a service built without the port does the
// write and nothing else.
func TestReminderIsBestEffort(t *testing.T) {
	t.Run("a reminder failure does not fail the save", func(t *testing.T) {
		repo := newRepo()
		repo.saveResult = jobtracking.Interaction{JobID: jobID, SavedAt: tPtr(time.Now())}
		rem := &fakeReminders{err: errors.New("reminder store is down")}
		svc := jobtracking.New(repo, jobtracking.WithReminders(rem))

		got, err := svc.SaveJob(ctx(), userID, slug)
		if err != nil {
			t.Fatalf("save should succeed despite the reminder: %v", err)
		}
		if got.SavedAt == nil {
			t.Error("SavedAt should be set")
		}
	})

	t.Run("no port configured is silent", func(t *testing.T) {
		repo := newRepo()
		repo.saveResult = jobtracking.Interaction{JobID: jobID, SavedAt: tPtr(time.Now())}
		svc := jobtracking.New(repo)

		if _, err := svc.SaveJob(ctx(), userID, slug); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestFailedWriteSchedulesNothing keeps the side effect downstream of the write it
// belongs to: a save that did not happen must not leave a reminder behind.
func TestFailedWriteSchedulesNothing(t *testing.T) {
	repo := newRepo()
	repo.saveErr = errors.New("write failed")
	rem := &fakeReminders{}
	svc := jobtracking.New(repo, jobtracking.WithReminders(rem))

	if _, err := svc.SaveJob(ctx(), userID, slug); err == nil {
		t.Fatal("expected the save to fail")
	}
	if len(rem.scheduled) != 0 {
		t.Errorf("scheduled = %v, want none", rem.scheduled)
	}
}
