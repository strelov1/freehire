package main

import (
	"context"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/job/qualityaudit"
	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeSuccessorQueries struct {
	chronic []db.ListChronicBoardsRow
	urls    map[string]string // provider/board pattern -> latest URL
	catalog map[string]db.Board
}

func (f fakeSuccessorQueries) ListChronicBoards(context.Context, db.ListChronicBoardsParams) ([]db.ListChronicBoardsRow, error) {
	return f.chronic, nil
}

func (f fakeSuccessorQueries) SuccessorAuditBoardJobs(_ context.Context, p db.SuccessorAuditBoardJobsParams) (db.SuccessorAuditBoardJobsRow, error) {
	return db.SuccessorAuditBoardJobsRow{OpenJobs: 3, LatestUrl: f.urls[p.Source+"/"+p.BoardPattern]}, nil
}

func (f fakeSuccessorQueries) GetBoardForResolve(_ context.Context, p db.GetBoardForResolveParams) (db.Board, error) {
	b, ok := f.catalog[p.Provider+"/"+p.Board]
	if !ok {
		return db.Board{}, pgx.ErrNoRows
	}
	return b, nil
}

// fakeProber moves every board it is handed to jibe/<board>.example, and records what it got.
type fakeProber struct{ got []qualityaudit.ChronicBoard }

func (p *fakeProber) Run(_ context.Context, boards []qualityaudit.ChronicBoard) []qualityaudit.Move {
	p.got = boards
	moves := make([]qualityaudit.Move, 0, len(boards))
	for _, b := range boards {
		moves = append(moves, qualityaudit.Move{Board: b, SuccessorProvider: "jibe", SuccessorBoard: b.Board + ".example"})
	}
	return moves
}

func TestFindSuccessorsResolvesCatalogue(t *testing.T) {
	q := fakeSuccessorQueries{
		chronic: []db.ListChronicBoardsRow{
			{Provider: "icims", Board: "acme", Total: 4},
			{Provider: "icims", Board: "beta", Total: 4},
			{Provider: "icims", Board: "nourl", Total: 4},
			{Provider: "adzuna", Board: "", Total: 4},
			{Provider: "jibe", Board: "Beta.example", Total: 4},
		},
		urls: map[string]string{
			"icims/acme:%": "https://careers-acme.icims.com/jobs/1/x/job",
			"icims/beta:%": "https://careers-beta.icims.com/jobs/2/y/job",
		},
		catalog: map[string]db.Board{
			"icims/acme":        {Company: "Acme", Status: "active"},
			"icims/beta":        {Company: "Beta", Status: "active"},
			"jibe/beta.example": {Company: "Beta", Status: "active"},
		},
	}
	prober := &fakeProber{}
	report, err := findSuccessors(context.Background(), q, prober, 7, 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.Chronic != 4 || report.Probed != 2 || len(prober.got) != 2 {
		t.Fatalf("chronic=%d probed=%d handed=%d, want 4/2/2", report.Chronic, report.Probed, len(prober.got))
	}
	for _, m := range report.Moves {
		if m.SuccessorChronic != (m.Board == "beta") {
			t.Fatalf("%s: successor_chronic = %v", m.Board, m.SuccessorChronic)
		}
	}
	status := map[string]string{}
	for _, m := range report.Moves {
		status[m.Board] = m.SuccessorStatus
		if m.OpenJobs != 3 || m.Company == "" {
			t.Fatalf("move %+v lost the old board's details", m)
		}
	}
	if status["acme"] != "absent" || status["beta"] != "active" {
		t.Fatalf("successor statuses = %v, want acme absent, beta active", status)
	}
}

func TestNameBoard(t *testing.T) {
	jibe, _ := url.Parse("https://talentconnect.example.org/acme/jobs")
	if p, b, ok := nameBoard(jibe, `<link href="/jibeapply/assets/app.css">`); !ok || p != "jibe" || b != "talentconnect.example.org" {
		t.Fatalf("self-hosted jibe = %s/%s %v", p, b, ok)
	}
	if _, _, ok := nameBoard(jibe, ""); ok {
		t.Fatal("an empty body must not name a self-hosted site")
	}
	lever, _ := url.Parse("https://jobs.lever.co/acme")
	if p, b, ok := nameBoard(lever, ""); !ok || p != "lever" || b != "acme" {
		t.Fatalf("lever = %s/%s %v", p, b, ok)
	}
}

type fakeRetirer struct{ retired []string }

func (f *fakeRetirer) Retire(_ context.Context, provider, board, _ string) (boardcatalog.Retirement, error) {
	f.retired = append(f.retired, provider+"/"+board)
	return boardcatalog.Retirement{Retired: true, Closed: 4}, nil
}

func TestApplyMovesRetiresOnlyRenamedTenants(t *testing.T) {
	report := successorReport{Moves: []successorMove{
		{Provider: "trakstar", Board: "old", SuccessorProvider: "trakstar", SuccessorBoard: "new", SuccessorStatus: "active"},
		{Provider: "trakstar", Board: "sick", SuccessorProvider: "trakstar", SuccessorBoard: "alsosick", SuccessorStatus: "active", SuccessorChronic: true},
		{Provider: "icims", Board: "acme", SuccessorProvider: "jibe", SuccessorBoard: "talent.acme.com", SuccessorStatus: "active"},
		{Provider: "pandape", Board: "a", SuccessorProvider: "pandape", SuccessorBoard: "b", SuccessorStatus: "absent"},
		{Provider: "inhire", Board: "x", SuccessorProvider: "inhire", SuccessorBoard: "y", SuccessorStatus: "active"},
	}}
	repo := &fakeRetirer{}
	if err := applyMoves(context.Background(), repo, &report, 1, true); err != nil {
		t.Fatal(err)
	}
	if len(repo.retired) != 1 || repo.retired[0] != "trakstar/old" {
		t.Fatalf("retired %v, want only trakstar/old (cap 1)", repo.retired)
	}
	if !report.Moves[0].Retired || report.Moves[0].ClosedJobs != 4 || report.Moves[4].Retired {
		t.Fatalf("moves = %+v", report.Moves)
	}

	dry := &fakeRetirer{}
	if err := applyMoves(context.Background(), dry, &report, 10, false); err != nil || len(dry.retired) != 0 {
		t.Fatalf("dry run retired %v (err %v)", dry.retired, err)
	}
}
