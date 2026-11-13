package handler

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

func TestRunAgentSearch(t *testing.T) {
	fs := &fakeSearcher{res: search.SearchResult{Total: 3}}
	res, outcome, err := runAgentSearch(context.Background(), fs, url.Values{"q": {"go"}}, 5, 10, true)
	if err != nil || res.Total != 3 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if fs.got.Query != "go" || fs.got.Limit != 5 || fs.got.Offset != 10 {
		t.Errorf("params = %+v", fs.got)
	}
	if outcome != semanticUnavailable {
		t.Errorf("outcome = %v, want semanticUnavailable", outcome)
	}

	_, outcome, _ = runAgentSearch(context.Background(), fs, url.Values{}, 5, 0, true)
	if got := outcome.noted(nil); len(got) != 1 || got[0] != "semantic" {
		t.Errorf("noted = %v, want [semantic]", got)
	}

	boom := errors.New("boom")
	if _, _, err := runAgentSearch(context.Background(), &fakeSearcher{err: boom}, url.Values{}, 5, 0, false); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}

func TestLoadAgentJob_MissIsErrNoRows(t *testing.T) {
	load := func(context.Context, string) (db.Job, error) { return db.Job{}, pgx.ErrNoRows }
	if _, _, err := loadAgentJob(context.Background(), load, nil, "x"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("err = %v, want pgx.ErrNoRows", err)
	}
}
