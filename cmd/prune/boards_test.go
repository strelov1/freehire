package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeCatalog is a boardLister/boardRetirer over an in-memory board set, so the guard's
// properties are tested without a database.
type fakeCatalog struct {
	rows    []db.ListLiveBoardsRow
	listErr error
	// retired records every (provider, board, region) the run retired, in order.
	retired []db.RetireBoardParams
}

func (f *fakeCatalog) ListLiveBoards(context.Context) ([]db.ListLiveBoardsRow, error) {
	return f.rows, f.listErr
}

func (f *fakeCatalog) RetireBoard(_ context.Context, arg db.RetireBoardParams) (int64, error) {
	f.retired = append(f.retired, arg)
	return 1, nil
}

func liveBoard(provider, board string, region ...string) db.ListLiveBoardsRow {
	r := db.ListLiveBoardsRow{Provider: provider, Board: board}
	if len(region) > 0 {
		r.Region = region[0]
	}
	return r
}

// catalogOf builds a boards set from "provider/board" pairs, each region-less — the
// shape every report fixture below wants.
func catalogOf(t *testing.T, pairs ...string) boards {
	t.Helper()
	rows := make([]db.ListLiveBoardsRow, len(pairs))
	for i, p := range pairs {
		provider, board, ok := strings.Cut(p, "/")
		if !ok {
			t.Fatalf("catalogOf: %q is not provider/board", p)
		}
		rows[i] = liveBoard(provider, board)
	}
	b, err := loadBoards(context.Background(), &fakeCatalog{rows: rows})
	if err != nil {
		t.Fatalf("catalogOf: %v", err)
	}
	return b
}

// The company-scoped rules have no ingest counterpart, so a deletion under one is undone
// by the next hourly crawl unless the board leaves the catalog. The worker therefore has
// to know which boards are still live, and match a posting to its board the same way the
// write path namespaced it.
func TestLoadBoardsMatchesPostingsToTheirBoard(t *testing.T) {
	b, err := loadBoards(context.Background(), &fakeCatalog{rows: []db.ListLiveBoardsRow{
		liveBoard("greenhouse", "acme"),
		liveBoard("greenhouse", "beta"),
		liveBoard("lever", "gamma"),
	}})
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}

	for _, want := range []boardKey{
		{"greenhouse", "acme"}, {"greenhouse", "beta"}, {"lever", "gamma"},
	} {
		if len(b.regionsOf(want)) != 1 {
			t.Errorf("%+v not listed; got %v", want, b.byProvider)
		}
	}
	// A posting is matched to its board through the namespaced external_id.
	if !b.crawls("greenhouse", "acme:12345") {
		t.Error("a posting of a live board must read as crawled")
	}
	// The same board id under another provider is a different board.
	if b.crawls("workday", "acme:12345") {
		t.Error("a board live under greenhouse must not shield the same id under workday")
	}
}

// A board the catalog does not list is retired, and only its postings may be deleted
// under a company-scoped rule.
func TestLoadBoardsOmitsUnlisted(t *testing.T) {
	b, err := loadBoards(context.Background(), &fakeCatalog{rows: []db.ListLiveBoardsRow{
		liveBoard("greenhouse", "acme"),
	}})
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}
	if b.crawls("greenhouse", "retired-co:1") {
		t.Error("a board absent from the catalog must not read as crawled")
	}
	// A link-source import or a moderator row carries a real provider but no live
	// board, and nothing re-crawls it — so it must never read as crawled.
	if b.crawls("greenhouse", "some-unlisted-board:99") {
		t.Error("an unlisted board must not read as crawled, whatever its provider")
	}
}

// A catalog read that fails, or one that comes back empty, must stop the run rather than
// yield an empty set: an empty set reads as "every board is retired", which would let the
// company rules delete the whole catalogue.
func TestLoadBoardsFailsClosed(t *testing.T) {
	if _, err := loadBoards(context.Background(), &fakeCatalog{listErr: errors.New("connection refused")}); err == nil {
		t.Error("a failed catalog read must be an error, not an empty listing")
	}
	if _, err := loadBoards(context.Background(), &fakeCatalog{}); err == nil {
		t.Error("an empty catalog must be an error — empty means every board is retired")
	}
}

// A retired board's row stays in the table, and the guard depends on not seeing it: the
// query filters to pending/active, so what it returns is exactly what a crawl visits.
func TestLoadBoardsCountsLiveRowsPerProvider(t *testing.T) {
	b, err := loadBoards(context.Background(), &fakeCatalog{rows: []db.ListLiveBoardsRow{
		liveBoard("whatjobs", "developer", "us"),
		liveBoard("whatjobs", "developer", "gb"),
		liveBoard("greenhouse", "acme"),
	}})
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}
	// One board under two regions is two catalog rows but one crawl target by
	// external_id, which is what the retire path has to know to name both.
	if got := b.regionsOf(boardKey{"whatjobs", "developer"}); len(got) != 2 {
		t.Errorf("regionsOf = %v, want both regional rows", got)
	}
	if b.liveRows("whatjobs") != 2 || b.liveRows("greenhouse") != 1 {
		t.Errorf("liveRows = whatjobs %d, greenhouse %d; want 2 and 1",
			b.liveRows("whatjobs"), b.liveRows("greenhouse"))
	}
}

// boardRow builds one candidate on a board, carrying the tri-state is_tech the
// derivation writes: a nil isTech is the "unknown" the dictionaries leave behind.
func boardRow(id int64, source, externalID string, isTech *bool, skills []string) db.PruneCandidatesRow {
	r := db.PruneCandidatesRow{ID: id, Source: source, ExternalID: externalID, Skills: skills}
	if isTech != nil {
		r.IsTech = pgtype.Bool{Bool: *isTech, Valid: true}
	}
	return r
}

func boolp(v bool) *bool { return &v }

// The report's premise is "this board has never posted anything technical", and that
// only follows where something was actually classified. is_tech is tri-state by design
// — jobderive leaves it NULL rather than coercing, "so the unclassified mass stays
// measurable" — but the report collapsed NULL into false, so a board nobody had
// classified read exactly like one determined to be non-technical.
//
// Measured on prod this was not an edge case: 11023 of the 17841 listed boards, 62% of
// the report, had no verdict on a single posting, against 10.6% among the boards the
// same report kept. Absence of a signal was doing the work of evidence.
func TestReportBoardsWithholdsBoardsWithNoVerdict(t *testing.T) {
	brd := catalogOf(t, "greenhouse/determined", "greenhouse/unknown",
		"greenhouse/technical", "greenhouse/skilled")
	q := &fakeCandidates{rows: []db.PruneCandidatesRow{
		// Classified, and the verdict was "not technical": the report may act on it.
		boardRow(1, "greenhouse", "determined:1", boolp(false), nil),
		// Never classified either way — the 62% case. Nothing is known about it.
		boardRow(2, "greenhouse", "unknown:1", nil, nil),
		// A verdict of "technical" keeps a board off the list, as before.
		boardRow(3, "greenhouse", "technical:1", boolp(true), nil),
		// So does a tagged skill, even with no verdict on the row.
		boardRow(4, "greenhouse", "skilled:1", nil, []string{"python"}),
	}}

	var out strings.Builder
	if err := reportBoards(context.Background(), &out, q, brd); err != nil {
		t.Fatalf("reportBoards: %v", err)
	}
	got := out.String()

	if !strings.Contains(got, "determined") {
		t.Errorf("a board whose postings were classified non-technical must be listed; got:\n%s", got)
	}
	for _, board := range []string{"unknown", "technical", "skilled"} {
		if strings.Contains(got, board+"\n") {
			t.Errorf("board %q must not be listed for retirement; got:\n%s", board, got)
		}
	}
}

// A guard that silently shrinks the report is worse than one that does not exist: the
// operator reads a short list as "this is everything", and the withheld boards never
// come back into view. The scan already reports what its source gate turned down, and
// this gate owes the same accounting.
func TestReportBoardsCountsWhatItWithheld(t *testing.T) {
	brd := catalogOf(t, "greenhouse/unknown")
	q := &fakeCandidates{rows: []db.PruneCandidatesRow{
		boardRow(1, "greenhouse", "unknown:1", nil, nil),
	}}

	var out strings.Builder
	if err := reportBoards(context.Background(), &out, q, brd); err != nil {
		t.Fatalf("reportBoards: %v", err)
	}
	if !strings.Contains(out.String(), "1") || !strings.Contains(out.String(), "classified") {
		t.Errorf("the report must say how many boards it withheld for want of a verdict; got:\n%s", out.String())
	}
}

// "Carries a tagged skill" is not the claim the report needs. The skills dictionary
// deliberately covers the non-engineering roles a technical company hires for —
// recruiting, HR, finance, legal, operations, customer success — because the facet
// describes every posting. Reading any tag as technical evidence therefore let a
// recruiting coordinator tagged {stakeholder-management, candidate-experience} vouch
// for a whole board, and a road-maintenance description vouch for its own.
//
// Measured on a 0.5% prod sample: of the postings the classifier calls non-technical
// yet that still carry skills, 37% carry nothing but non-engineering ones.
func TestReportBoardsIgnoresNonEngineeringSkills(t *testing.T) {
	brd := catalogOf(t, "greenhouse/staffing", "greenhouse/software")
	q := &fakeCandidates{rows: []db.PruneCandidatesRow{
		// Classified non-technical, and every tag on it names a non-engineering craft.
		boardRow(1, "greenhouse", "staffing:1", boolp(false),
			[]string{"stakeholder-management", "candidate-experience"}),
		// One engineering tag is evidence, exactly as before.
		boardRow(2, "greenhouse", "software:1", boolp(false),
			[]string{"talent-sourcing", "kubernetes"}),
	}}

	var out strings.Builder
	if err := reportBoards(context.Background(), &out, q, brd); err != nil {
		t.Fatalf("reportBoards: %v", err)
	}
	got := out.String()

	if !strings.Contains(got, "staffing") {
		t.Errorf("a board whose only tags name non-engineering craft must be listed; got:\n%s", got)
	}
	if strings.Contains(got, "software\n") {
		t.Errorf("a board with an engineering tag must stay off the list; got:\n%s", got)
	}
}

// Retiring every board a provider has is a one-way door, and the report is where it has
// to be caught. Ingest crawls only live catalog rows, so a provider with none is never
// crawled again — and the company-scoped rules refuse a job they cannot re-crawl, so its
// postings can never be pruned either. The dead weight becomes permanent.
//
// retireBoards enforces the order too (it holds such a provider back), but a refusal
// discovered after the run reads as "there was nothing to do". The report is what the
// operator has in front of them beforehand.
//
// The boards are still listed — they are genuine candidates — but the report names the
// providers it would empty, so the one that empties a provider is retired last,
// deliberately, after its jobs are gone.
func TestReportBoardsNamesProvidersItWouldEmpty(t *testing.T) {
	// Every board tinyats has is about to be retired; greenhouse keeps one.
	brd := catalogOf(t, "tinyats/one", "tinyats/two", "greenhouse/dead", "greenhouse/alive")
	q := &fakeCandidates{rows: []db.PruneCandidatesRow{
		boardRow(1, "tinyats", "one:1", boolp(false), nil),
		boardRow(2, "tinyats", "two:1", boolp(false), nil),
		boardRow(3, "greenhouse", "dead:1", boolp(false), nil),
		boardRow(4, "greenhouse", "alive:1", boolp(true), nil),
	}}

	var out strings.Builder
	if err := reportBoards(context.Background(), &out, q, brd); err != nil {
		t.Fatalf("reportBoards: %v", err)
	}
	got := out.String()

	if !strings.Contains(got, "tinyats") {
		t.Fatalf("the provider's boards must still be listed; got:\n%s", got)
	}
	// The warning has to name the provider AND say what the hazard is, or it reads as
	// decoration and gets moved with everything else.
	if !strings.Contains(got, "every live board") {
		t.Fatalf("the report must warn that a provider would be left with no boards; got:\n%s", got)
	}
	warning := got[strings.Index(got, "every live board"):]
	if !strings.Contains(warning, "tinyats") {
		t.Errorf("the warning must name tinyats; got:\n%s", warning)
	}
	if strings.Contains(warning, "greenhouse") {
		t.Errorf("greenhouse keeps a board and must not be named as emptied; got:\n%s", warning)
	}
}
