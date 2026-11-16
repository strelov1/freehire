//go:build integration

package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type profileStore struct {
	*dbStore
	mu     sync.Mutex
	counts map[string]int
	times  map[string]time.Duration
}

func (s *profileStore) record(name string, start time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[name]++
	s.times[name] += time.Since(start)
}
func (s *profileStore) StoredDescription(ctx context.Context, source, id string) (string, error) {
	defer s.record("body", time.Now())
	return s.dbStore.StoredDescription(ctx, source, id)
}
func (s *profileStore) StoredDescriptions(ctx context.Context, source string, ids []string) (map[string]string, error) {
	defer s.record("body-batch", time.Now())
	return s.dbStore.StoredDescriptions(ctx, source, ids)
}
func (s *profileStore) Touch(ctx context.Context, source, id string, d *time.Time) (string, error) {
	defer s.record("touch", time.Now())
	return s.dbStore.Touch(ctx, source, id, d)
}
func (s *profileStore) Save(ctx context.Context, j job.Job) error {
	defer s.record("save", time.Now())
	return s.dbStore.Save(ctx, j)
}
func (s *profileStore) CloseCovered(ctx context.Context, source string, ids []string) ([]string, error) {
	defer s.record("close", time.Now())
	return s.dbStore.CloseCovered(ctx, source, ids)
}
func (s *profileStore) CanonicalCompanySlugs(ctx context.Context, keys []string) (map[string]string, error) {
	defer s.record("aliases", time.Now())
	return s.dbStore.CanonicalCompanySlugs(ctx, keys)
}

type profileCoverage struct {
	inner pipeline.CoverageLookup
	s     *profileStore
}

func (c profileCoverage) NonAggregatorCompanies(ctx context.Context, a, b, d []string) (map[string]bool, error) {
	defer c.s.record("coverage", time.Now())
	return c.inner.NonAggregatorCompanies(ctx, a, b, d)
}

type profileSource struct{ jobs []sources.Job }

func (profileSource) Provider() string { return "spainjobs" }
func (s profileSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return s.jobs, nil
}
func (s profileSource) FetchNew(ctx context.Context, e sources.CompanyEntry, _ func(string) bool) ([]sources.Job, error) {
	return s.Fetch(ctx, e)
}
func TestBufferedWriteBatchesBodies(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	base := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy())
	s := &profileStore{dbStore: base, counts: map[string]int{}, times: map[string]time.Duration{}}
	seed := func(provider, id, company string) {
		j, e := job.New(job.Draft{Input: jobderive.Input{Source: provider, ExternalID: id, Title: "Software Engineer", Company: company, Description: "<p>Build services in Python and PostgreSQL.</p>"}, URL: "https://example.com/" + id})
		if e != nil {
			t.Fatal(e)
		}
		if e = base.Save(ctx, j); e != nil {
			t.Fatal(e)
		}
	}
	seed("lever", "profile-ats", "Profile Covered")
	seed("spainjobs", ":profile-missing-0", "Profile Covered")
	raw := make([]sources.Job, 0, 656)
	for i := 0; i < 512; i++ {
		raw = append(raw, sources.Job{ExternalID: fmt.Sprintf("profile-missing-%d", i), Title: "Software Engineer", Company: "Profile Covered", URL: "https://example.com/job"})
	}
	for i := 0; i < 128; i++ {
		id := fmt.Sprintf("profile-known-%d", i)
		seed("spainjobs", ":"+id, "Profile Known")
		raw = append(raw, sources.Job{ExternalID: id, Title: "Software Engineer", Company: "Profile Known", SeenRefresh: true})
	}
	for i := 0; i < 16; i++ {
		raw = append(raw, sources.Job{ExternalID: fmt.Sprintf("profile-new-%d", i), Title: "Software Engineer", Company: "Profile New", URL: "https://example.com/new", Description: "Build services in Python."})
	}
	r := pipeline.Runner{Registry: map[string]sources.Source{"spainjobs": profileSource{raw}}, Store: s, Aliases: s, Coverage: profileCoverage{newCoverage(pool), s}}
	start := time.Now()
	stats, err := r.Run(ctx, []sources.CompanyEntry{{Provider: "spainjobs", Company: "SpainJobs"}})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total().Ingested != 144 || stats.Total().Skipped != 0 || stats.Total().ATSCovered != 512 {
		t.Fatalf("stats=%+v", stats.Total())
	}
	for name, want := range map[string]int{"body": 0, "body-batch": 2, "touch": 128, "save": 16, "close": 1, "aliases": 1, "coverage": 1} {
		if s.counts[name] != want {
			t.Errorf("%s calls=%d want %d", name, s.counts[name], want)
		}
	}
	var closed, queued bool
	if err := pool.QueryRow(ctx, `SELECT closed_at IS NOT NULL, EXISTS(SELECT 1 FROM search_delete_outbox o WHERE o.job_id=j.id) FROM jobs j WHERE source='spainjobs' AND external_id=':profile-missing-0'`).Scan(&closed, &queued); err != nil {
		t.Fatal(err)
	}
	if !closed || !queued {
		t.Fatalf("covered stored row: closed=%t delete queued=%t", closed, queued)
	}
	t.Logf("elapsed=%s stats=%+v counts=%v times=%v", time.Since(start), stats.Total(), s.counts, s.times)
}

func TestStoredDescriptionsIsolatesPrivateAndOtherSources(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	store := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy())
	for _, id := range []string{"body-batch-public", "body-batch-private"} {
		if err := store.Save(ctx, cheapPosting(id, "Software Engineer")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET is_private=true WHERE source='lever' AND external_id='body-batch-private'`); err != nil {
		t.Fatal(err)
	}
	ids := []string{"body-batch-public", "body-batch-private", "body-batch-absent"}
	got, err := store.StoredDescriptions(ctx, "lever", ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[ids[0]] == "" {
		t.Fatalf("body keys not isolated: %v", got)
	}
	other, err := store.StoredDescriptions(ctx, "greenhouse", ids)
	if err != nil || len(other) != 0 {
		t.Fatalf("other source: count=%d err=%v", len(other), err)
	}
}

type duplicateBodyStore struct {
	*dbStore
	saved chan struct{}
}

func (s *duplicateBodyStore) Save(ctx context.Context, j job.Job) error {
	f := j.Fields()
	if f.Description == "" {
		select {
		case <-s.saved:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := s.dbStore.Save(ctx, j); err != nil {
		return err
	}
	if f.Description != "" && !f.UpsertParams().BodyFromCatalogue {
		close(s.saved)
	}
	return nil
}
func (s *duplicateBodyStore) StoredDescription(ctx context.Context, source, id string) (string, error) {
	select {
	case <-s.saved:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return s.dbStore.StoredDescription(ctx, source, id)
}
func TestBufferedDuplicateKeepsBodyAndFacetsInStep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := testdb.Pool(t)
	s := &duplicateBodyStore{dbStore: newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy()), saved: make(chan struct{})}
	raw := []sources.Job{
		{ExternalID: "duplicate-body", Title: "Software Engineer", Company: "Duplicate Body", Description: "Build services in Python.", URL: "https://example.com/duplicate-body"},
		{ExternalID: "duplicate-body", Title: "Software Engineer", Company: "Duplicate Body", URL: "https://example.com/duplicate-body"},
	}
	r := pipeline.Runner{Registry: map[string]sources.Source{"spainjobs": profileSource{raw}}, Store: s}
	stats, err := r.Run(ctx, []sources.CompanyEntry{{Provider: "spainjobs"}})
	if err != nil || stats.Total().Skipped != 0 {
		t.Fatalf("stats=%+v err=%v", stats.Total(), err)
	}
	var body string
	var skills []string
	if err := pool.QueryRow(ctx, `SELECT description,skills FROM jobs WHERE source='spainjobs' AND external_id=':duplicate-body'`).Scan(&body, &skills); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Python") || !slices.Contains(skills, "python") {
		t.Fatalf("body and facets disagree: body=%q skills=%v", body, skills)
	}
}

type insertAfterPrefetchStore struct {
	*dbStore
	posting job.Job
}

func (s *insertAfterPrefetchStore) StoredDescriptions(ctx context.Context, source string, ids []string) (map[string]string, error) {
	bodies, err := s.dbStore.StoredDescriptions(ctx, source, ids)
	if err != nil {
		return nil, err
	}
	if err = s.Save(ctx, s.posting); err != nil {
		return nil, err
	}
	return bodies, nil
}
func TestBufferedCachedAbsenceFencesAConcurrentInsert(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	posting, err := job.New(job.Draft{Input: jobderive.Input{Source: "spainjobs", ExternalID: ":prefetch-insert", Title: "Software Engineer", Company: "Prefetch Insert", Description: "Build services in Python."}, URL: "https://example.com/prefetch-insert"})
	if err != nil {
		t.Fatal(err)
	}
	s := &insertAfterPrefetchStore{dbStore: newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy()), posting: posting}
	raw := []sources.Job{{ExternalID: "prefetch-insert", Title: "Software Engineer", Company: "Prefetch Insert", URL: "https://example.com/prefetch-insert"}}
	r := pipeline.Runner{Registry: map[string]sources.Source{"spainjobs": profileSource{raw}}, Store: s}
	stats, _ := r.Run(ctx, []sources.CompanyEntry{{Provider: "spainjobs"}})
	if stats.Total().Skipped != 1 || stats.Total().Ingested != 0 {
		t.Fatalf("stale absence was not fenced: %+v", stats.Total())
	}
	var body string
	var skills []string
	if err := pool.QueryRow(ctx, `SELECT description,skills FROM jobs WHERE source='spainjobs' AND external_id=':prefetch-insert'`).Scan(&body, &skills); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Python") || !slices.Contains(skills, "python") {
		t.Fatalf("body and facets disagree: body=%q skills=%v", body, skills)
	}
}
