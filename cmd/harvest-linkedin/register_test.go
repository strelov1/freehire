package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
)

func TestRegistrationPlanUsesTheWorklistSearchBudget(t *testing.T) {
	queries, err := loadQueries(writeTemp(t, `queries:
  - keywords: " software   engineer "
    location: " Germany "
    pages: 3
    jobage: 14
  - keywords: SOFTWARE engineer
    location: germany
    pages: 99
    jobage: 30
  - keywords: data engineer
    location: Poland
`))
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := registrationPlan(queries)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 {
		t.Fatalf("planned %d searches, want two unique slices", len(inputs))
	}
	if got := inputs[0]; got.Provider != "linkedin" || got.Board != "software engineer" || got.Region != "Germany" || got.Company != "LinkedIn Germany software engineer" || !reflect.DeepEqual(got.Tenants, map[string]string{"pages": "3", "jobage": "14"}) {
		t.Fatalf("first search = %+v", got)
	}
	if !reflect.DeepEqual(inputs[1].Tenants, map[string]string{"pages": "1", "jobage": "7"}) {
		t.Fatalf("default limits = %+v", inputs[1].Tenants)
	}
}

func TestLoadQueriesAcceptsTheMaximumBoundedSearch(t *testing.T) {
	queries, err := loadQueries(writeTemp(t, "queries:\n  - keywords: software engineer\n    location: Germany\n    pages: 100\n    jobage: 30\n"))
	if err != nil || len(queries) != 1 || queries[0].Pages != 100 || queries[0].JobAge != 30 {
		t.Fatalf("maximum bounded search=%+v, %v", queries, err)
	}
}

func TestLoadQueriesRejectsUnboundedOrMalformedSearches(t *testing.T) {
	for _, body := range []string{
		"queries:\n  - keywords: ' '\n    location: Germany\n",
		"queries:\n  - keywords: developer\n    location: ' '\n",
		"queries:\n  - keywords: developer\n    location: Germany\n    pages: -1\n",
		"queries:\n  - keywords: developer\n    location: Germany\n    pages: 101\n",
		"queries:\n  - keywords: developer\n    location: Germany\n    jobage: -1\n",
		"queries:\n  - keywords: developer\n    location: Germany\n    jobage: 31\n",
	} {
		if got, err := loadQueries(writeTemp(t, body)); err == nil {
			t.Errorf("accepted bad search %+v from %q", got, body)
		}
	}
}

func TestRegisterSearchesPreviewDoesNotRequireDatabaseOrNetwork(t *testing.T) {
	t.Setenv("DATABASE_URL", "not-a-database")
	t.Setenv("SCRAPLING_FETCH_URL", "not-a-service")
	path := writeTemp(t, "queries:\n  - keywords: software engineer\n    location: Germany\n")
	if exit := runArgs([]string{"--register-searches", path}); exit != 0 {
		t.Fatalf("preview exit=%d", exit)
	}
	if exit := runArgs([]string{"--apply", path}); exit != 2 {
		t.Fatalf("apply without registration exit=%d, want 2", exit)
	}
	if exit := runArgs([]string{"--register-searches", path, "company-slugs.txt"}); exit != 2 {
		t.Fatalf("registration with export catalogue exit=%d, want 2", exit)
	}
}

type registrationInserter struct {
	results []boardcatalog.Board
	errors  []error
	calls   []boardcatalog.InsertInput
}

func (s *registrationInserter) Insert(_ context.Context, in boardcatalog.InsertInput, status boardcatalog.Status) (boardcatalog.Board, error) {
	if status != boardcatalog.StatusPending {
		return boardcatalog.Board{}, errors.New("harvested searches must start pending")
	}
	i := len(s.calls)
	s.calls = append(s.calls, in)
	return s.results[i], s.errors[i]
}

func TestRegisterSearchesReportsDuplicatesAndStorageFailures(t *testing.T) {
	inputs := []boardcatalog.InsertInput{{Board: "software engineer", Region: "Germany"}, {Board: "data engineer", Region: "Poland"}}
	insert := &registrationInserter{results: []boardcatalog.Board{{}, {Status: boardcatalog.StatusPending}}, errors: []error{boardcatalog.ErrDuplicateBoard, nil}}
	got, err := registerSearches(context.Background(), insert, inputs)
	if err != nil || got.Planned != 2 || got.Added != 1 || got.AlreadyKnown != 1 || got.Rejected != 0 {
		t.Fatalf("duplicate outcome=%+v err=%v", got, err)
	}
	for _, test := range []struct {
		board boardcatalog.Board
		err   error
	}{
		{err: errors.New("storage unavailable")},
		{board: boardcatalog.Board{Status: boardcatalog.StatusRejected, RejectedReason: "bad provider"}},
	} {
		insert := &registrationInserter{results: []boardcatalog.Board{test.board}, errors: []error{test.err}}
		got, err := registerSearches(context.Background(), insert, inputs)
		if err == nil || len(insert.calls) != 1 || got.Added != 0 {
			t.Fatalf("failed insert outcome=%+v calls=%d err=%v", got, len(insert.calls), err)
		}
		if test.board.Status == boardcatalog.StatusRejected && got.Rejected != 1 {
			t.Fatalf("rejection not counted: %+v", got)
		}
	}
}
