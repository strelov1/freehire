package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestRepairedURL(t *testing.T) {
	const oldURL = "https://www.ess.gov.si/iskalci-zaposlitve/iskanje-zaposlitve/iskanje-dela/?idp=3493570"
	const fixedURL = "https://www.ess.gov.si/iskalci-zaposlitve/iskanje-zaposlitve/iskanje-dela/?idp=3493570/#/pdm/3493570"
	for _, tt := range []struct {
		name string
		job  db.Job
		want string
	}{
		{
			name: "legacy vacancy link",
			job:  db.Job{Source: "ess", ExternalID: ":3493570", URL: oldURL},
			want: fixedURL,
		},
		{
			name: "already repaired",
			job:  db.Job{Source: "ess", ExternalID: ":3493570", URL: fixedURL},
			want: fixedURL,
		},
		{
			name: "different source",
			job:  db.Job{Source: "other", ExternalID: ":3493570", URL: oldURL},
			want: oldURL,
		},
		{
			name: "private posting",
			job:  db.Job{Source: "ess", ExternalID: ":3493570", URL: oldURL, IsPrivate: true},
			want: oldURL,
		},
		{
			name: "different vacancy identity",
			job:  db.Job{Source: "ess", ExternalID: ":3475977", URL: oldURL},
			want: oldURL,
		},
		{
			name: "different destination",
			job:  db.Job{Source: "ess", ExternalID: ":3493570", URL: "https://employer.example/jobs/3493570"},
			want: "https://employer.example/jobs/3493570",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := repairedURL(tt.job); got != tt.want {
				t.Errorf("repairedURL = %q, want %q", got, tt.want)
			}
		})
	}
}

type fakeStore struct {
	jobs     []db.Job
	writeErr error
	conflict bool
}

func (s *fakeStore) ListJobsBySourceAfter(_ context.Context, p db.ListJobsBySourceAfterParams) ([]db.Job, error) {
	page := []db.Job{}
	for _, j := range s.jobs {
		if j.Source == p.Source && j.ID > p.AfterID {
			page = append(page, j)
		}
	}
	return page, nil
}

func (s *fakeStore) RepairESSJobURL(_ context.Context, p db.RepairESSJobURLParams) (db.RepairESSJobURLRow, error) {
	if s.writeErr != nil {
		return db.RepairESSJobURLRow{}, s.writeErr
	}
	if s.conflict {
		return db.RepairESSJobURLRow{}, nil
	}
	for i := range s.jobs {
		j := &s.jobs[i]
		if j.ID == p.ID && j.URL == p.OldUrl && j.ContentHash == p.OldContentHash {
			j.URL = p.URL
			j.ContentHash = p.ContentHash
			return db.RepairESSJobURLRow{UpdatedCount: 1}, nil
		}
	}
	return db.RepairESSJobURLRow{}, nil
}

func TestBackfillReportsThenRepairsAndResumes(t *testing.T) {
	const oldURL = "https://www.ess.gov.si/iskalci-zaposlitve/iskanje-zaposlitve/iskanje-dela/?idp=3493570"
	const fixedURL = "https://www.ess.gov.si/iskalci-zaposlitve/iskanje-zaposlitve/iskanje-dela/?idp=3493570/#/pdm/3493570"
	j := db.Job{
		ID: 1, Source: "ess", ExternalID: ":3493570", URL: oldURL,
		Title: "Developer", Description: "Build software", PublicSlug: "published-job-url",
	}
	j.ContentHash = pgtype.Text{String: jobhash.OfRow(j, j.Description), Valid: true}
	store := &fakeStore{jobs: []db.Job{j}}
	read, changed, written, err := backfillAll(context.Background(), store, false)
	if err != nil || read != 1 || changed != 1 || written != 0 {
		t.Fatalf("report = %d/%d/%d, %v; want 1/1/0", read, changed, written, err)
	}
	if store.jobs[0].URL != oldURL || store.jobs[0].ContentHash != j.ContentHash {
		t.Fatal("report mode changed the stored posting")
	}
	read, changed, written, err = backfillAll(context.Background(), store, true)
	if err != nil || read != 1 || changed != 1 || written != 1 {
		t.Fatalf("apply = %d/%d/%d, %v; want 1/1/1", read, changed, written, err)
	}
	fixed := store.jobs[0]
	if fixed.URL != fixedURL || fixed.PublicSlug != j.PublicSlug || fixed.Description != j.Description {
		t.Fatalf("repaired posting = %+v", fixed)
	}
	if fixed.ContentHash == j.ContentHash || fixed.ContentHash.String != jobhash.OfRow(fixed, fixed.Description) {
		t.Fatal("content hash must reflect the corrected URL and preserve the other indexed fields")
	}
	read, changed, written, err = backfillAll(context.Background(), store, true)
	if err != nil || read != 1 || changed != 0 || written != 0 {
		t.Fatalf("repeat = %d/%d/%d, %v; want 1/0/0", read, changed, written, err)
	}
}

func TestBackfillStopsOnWriteFailure(t *testing.T) {
	writeErr := errors.New("write failed")
	store := &fakeStore{
		jobs: []db.Job{{
			ID: 1, Source: "ess", ExternalID: ":3493570",
			URL: "https://www.ess.gov.si/iskalci-zaposlitve/iskanje-zaposlitve/iskanje-dela/?idp=3493570",
		}},
		writeErr: writeErr,
	}
	_, _, written, err := backfillAll(context.Background(), store, true)
	if !errors.Is(err, writeErr) || written != 0 {
		t.Fatalf("written=%d err=%v, want 0 and the write failure", written, err)
	}
}

func TestBackfillReportsConcurrentChange(t *testing.T) {
	store := &fakeStore{
		jobs: []db.Job{{
			ID: 1, Source: "ess", ExternalID: ":3493570",
			URL: "https://www.ess.gov.si/iskalci-zaposlitve/iskanje-zaposlitve/iskanje-dela/?idp=3493570",
		}},
		conflict: true,
	}
	_, _, written, err := backfillAll(context.Background(), store, true)
	if err == nil || written != 0 {
		t.Fatalf("written=%d err=%v, want an uncompleted repair after a concurrent change", written, err)
	}
}
