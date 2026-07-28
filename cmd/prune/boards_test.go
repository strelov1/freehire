package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

func writeBoardFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// The company-scoped rules have no ingest counterpart, so a deletion under one is
// undone by the next hourly crawl unless the board is struck from the source files.
// The worker therefore has to know which companies are still listed — matched by the
// same slug normalization ingest writes, or the guard would compare different strings.
func TestListedCompaniesMatchesIngestSlugging(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", `
- company: "Acme Corp."
  board: acme
- company: "Beta & Co"
  board: beta
`)
	writeBoardFile(t, dir, "custom.yml", `
- company: "Gamma"
  provider: lever
  board: gamma
`)
	// Not a board file: must not be read as one.
	writeBoardFile(t, dir, "README.md", "not yaml")

	b, err := loadBoards(dir)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}

	for _, want := range []boardKey{
		{"greenhouse", "acme"}, {"greenhouse", "beta"}, {"lever", "gamma"},
	} {
		if !b.listed[want] {
			t.Errorf("%+v not listed; got %v", want, b.listed)
		}
	}
	if len(b.listed) != 3 {
		t.Errorf("listed %d entries, want 3 — README.md is not a board file", len(b.listed))
	}
	// A posting is matched to its board through the namespaced external_id.
	if !b.crawls("greenhouse", "acme:12345") {
		t.Error("a posting of a listed board must read as crawled")
	}
	// The same board id under another provider is a different board.
	if b.crawls("workday", "acme:12345") {
		t.Error("a board listed under greenhouse must not shield the same id under workday")
	}
}

// A slug the source files do not mention is retired, and only those may be deleted
// under a company-scoped rule.
func TestListedCompaniesOmitsUnlisted(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", "- company: Acme\n  board: acme\n")

	b, err := loadBoards(dir)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}
	if b.crawls("greenhouse", "retired-co:1") {
		t.Error("a board absent from every file must not read as crawled")
	}
	// A link-source import or a moderator row carries a real provider but no listed
	// board, and nothing re-crawls it — so it must never read as crawled.
	if b.crawls("greenhouse", "some-unlisted-board:99") {
		t.Error("an unlisted board must not read as crawled, whatever its provider")
	}
}

// A source directory that cannot be read must stop the run rather than yield an empty
// set: an empty set reads as "every board is retired", which would let the company
// rules delete the whole catalogue.
func TestListedCompaniesFailsClosed(t *testing.T) {
	if _, err := loadBoards(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("an unreadable source directory must be an error, not an empty listing — empty means every board is retired")
	}
}

// The guard runs against the real sources/ directory on every invocation, and that
// directory holds files that are not board lists — telegram.yml is the channel list for
// cmd/tg-ingest and does not parse as one. A fixture-only test cannot see that, and did
// not: --boards errored on every real run until this case existed.
func TestLoadBoardsReadsTheRealSourcesDirectory(t *testing.T) {
	b, err := loadBoards("../../sources")
	if err != nil {
		t.Fatalf("loadBoards on the real directory: %v", err)
	}
	if len(b.listed) < 100 {
		t.Errorf("listed %d entries, want the real catalogue's boards", len(b.listed))
	}
	if b.byProvider["greenhouse"] == nil {
		t.Error("greenhouse must have boards")
	}
	for _, notCrawled := range []string{"telegram", "manual", ""} {
		if b.byProvider[notCrawled] != nil {
			t.Errorf("%q is not a board provider and must have no boards", notCrawled)
		}
	}
}

// Retired boards live in sources/retired/, and the guard depends on not seeing them: a
// glob that descended into subdirectories would read them as live, and the rules that
// require a retired board would quietly stop firing.
func TestLoadBoardsIgnoresRetiredSubdirectory(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", "- company: Live Co\n  board: live\n")
	if err := os.MkdirAll(filepath.Join(dir, "retired"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeBoardFile(t, filepath.Join(dir, "retired"), "greenhouse.yml", "- company: Gone Co\n  board: gone\n")

	b, err := loadBoards(dir)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}
	if !b.crawls("greenhouse", "live:1") {
		t.Error("the live board must still read as crawled")
	}
	if b.crawls("greenhouse", "gone:1") {
		t.Error("a retired board must not read as crawled — moving it out of sources/ is what retires it")
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
	brd := boards{
		listed: map[boardKey]bool{
			{"greenhouse", "determined"}: true, {"greenhouse", "unknown"}: true,
			{"greenhouse", "technical"}: true, {"greenhouse", "skilled"}: true,
		},
		byProvider: map[string]map[string]bool{"greenhouse": {
			"determined": true, "unknown": true, "technical": true, "skilled": true,
		}},
	}
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
	brd := boards{
		listed:     map[boardKey]bool{{"greenhouse", "unknown"}: true},
		byProvider: map[string]map[string]bool{"greenhouse": {"unknown": true}},
	}
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
	brd := boards{
		listed: map[boardKey]bool{
			{"greenhouse", "staffing"}: true, {"greenhouse", "software"}: true,
		},
		byProvider: map[string]map[string]bool{"greenhouse": {
			"staffing": true, "software": true,
		}},
	}
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

// Retiring every board a provider has is a one-way door, and the report is where it
// has to be caught. Ingest takes one board file by path, so a provider with nothing
// left in sources/ is never crawled again — and the company-scoped rules refuse a job
// they cannot re-crawl, so its postings can never be pruned either. The dead weight
// becomes permanent. sources/retired/README.md states the order (prune the jobs first,
// move the last entry after), but stating it in prose leaves it to whoever reads the
// report at 2am.
//
// The boards are still listed — they are genuine candidates — but the report names the
// providers it would empty, so the entry that empties one is moved last, deliberately.
func TestReportBoardsNamesProvidersItWouldEmpty(t *testing.T) {
	brd := boards{
		listed: map[boardKey]bool{
			// Every board this provider has is about to be retired.
			{"tinyats", "one"}: true, {"tinyats", "two"}: true,
			// This one keeps a board, so it is not emptied.
			{"greenhouse", "dead"}: true, {"greenhouse", "alive"}: true,
		},
		byProvider: map[string]map[string]bool{
			"tinyats":    {"one": true, "two": true},
			"greenhouse": {"dead": true, "alive": true},
		},
	}
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
	if !strings.Contains(got, "every listed board") {
		t.Fatalf("the report must warn that a provider would be left with no boards; got:\n%s", got)
	}
	warning := got[strings.Index(got, "every listed board"):]
	if !strings.Contains(warning, "tinyats") {
		t.Errorf("the warning must name tinyats; got:\n%s", warning)
	}
	if strings.Contains(warning, "greenhouse") {
		t.Errorf("greenhouse keeps a board and must not be named as emptied; got:\n%s", warning)
	}
}
