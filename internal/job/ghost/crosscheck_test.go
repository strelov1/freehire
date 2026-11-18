package ghost

import (
	"slices"
	"testing"
)

func posting(id int64, company, title string, stamped bool) Posting {
	return Posting{ID: id, CompanySlug: company, Title: title, Stamped: stamped}
}

// onBoard is a board large enough to pass minBoardTitles, carrying the given titles.
func onBoard(titles ...string) Board {
	b := Board{Titles: titles}
	for len(b.Titles) < minBoardTitles {
		b.Titles = append(b.Titles, "Office Manager")
	}
	return b
}

// carried returns postings the board above carries, enough of them to corroborate a
// batch and keep a single absent posting below maxAbsentShare.
func carried(company string) []Posting {
	return []Posting{
		posting(101, company, "Office Manager", false),
		posting(102, company, "Office Manager", false),
		posting(103, company, "Office Manager", false),
	}
}

// Absence is evidence only where we looked. Without the gate the signal reports our
// own board-coverage blind spots as the employer's fault — and the previous attempt
// at this feature died precisely by measuring our data rather than the world.
func TestCrosscheck_NoCompanyBoardMeansNoVerdict(t *testing.T) {
	got := Crosscheck([]Posting{posting(1, "acme", "Go Developer", false)}, Board{})

	if len(got.Stamp)+len(got.Clear) != 0 {
		t.Errorf("stamp=%v clear=%v, want no writes — we do not crawl this company's board", got.Stamp, got.Clear)
	}
	if got.Skips != (Skips{NoBoard: 1}) {
		t.Errorf("skipped = %+v, want NoBoard 1 — the gate must be visible in the report", got.Skips)
	}
}

// One or two titles are as often a stray tenant behind the slug as the employer's
// catalogue, so they are no coverage either.
func TestCrosscheck_TinyBoardMeansNoVerdict(t *testing.T) {
	got := Crosscheck(
		append(carried("acme"), posting(1, "acme", "Go Developer", false)),
		Board{Titles: []string{"Office Manager"}},
	)

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none from a one-title board", got.Stamp)
	}
	if got.Skips.NoBoard != 4 {
		t.Errorf("skipped = %+v, want NoBoard 4", got.Skips)
	}
}

func TestCrosscheck_AbsentRoleIsStamped(t *testing.T) {
	got := Crosscheck(append(carried("acme"), posting(1, "acme", "Go Developer", false)), onBoard())

	if !slices.Equal(got.Stamp, []int64{1}) {
		t.Errorf("stamp = %v, want [1]", got.Stamp)
	}
}

func TestCrosscheck_PresentRoleIsNotStamped(t *testing.T) {
	got := Crosscheck([]Posting{posting(1, "acme", "Go Developer", false)}, onBoard("Go Developer"))

	if len(got.Stamp)+len(got.Clear) != 0 {
		t.Errorf("stamp=%v clear=%v, want no writes — the role is on the company's own board", got.Stamp, got.Clear)
	}
}

// The stamp tracks the world rather than accumulating: a role that reappears on the
// company's board loses its stamp on the next run.
func TestCrosscheck_ReappearingRoleIsCleared(t *testing.T) {
	got := Crosscheck([]Posting{posting(1, "acme", "Go Developer", true)}, onBoard("Go Developer"))

	if !slices.Equal(got.Clear, []int64{1}) {
		t.Errorf("clear = %v, want [1]", got.Clear)
	}
	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none", got.Stamp)
	}
}

// The reader ignores a stamp that has aged out, so a still-absent role is re-stamped.
func TestCrosscheck_AlreadyStampedAndStillAbsentIsRestamped(t *testing.T) {
	got := Crosscheck(append(carried("acme"), posting(1, "acme", "Go Developer", true)), onBoard())

	if !slices.Equal(got.Stamp, []int64{1}) {
		t.Errorf("stamp = %v, want [1] — the stamp must be refreshed or it ages out", got.Stamp)
	}
}

// A board that carries nothing a source lists for the company is not evidence about
// that source: another tenant behind the slug, a regional board, or a source that
// rewrites every title.
func TestCrosscheck_UncorroboratedBatchIsNotJudged(t *testing.T) {
	got := Crosscheck(
		[]Posting{posting(1, "acme", "Go Developer", false), posting(2, "acme", "Rust Developer", true)},
		onBoard(),
	)

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none — the board carries none of the batch", got.Stamp)
	}
	if !slices.Equal(got.Clear, []int64{2}) {
		t.Errorf("clear = %v, want [2] — a stamp the current rules no longer support goes", got.Clear)
	}
	if got.Skips != (Skips{Uncorroborated: 2}) {
		t.Errorf("skipped = %+v, want Uncorroborated 2", got.Skips)
	}
}

// When most of a batch is absent, absence is the company's pattern on that source — an
// agency, or a board we only partly crawl — not a fact about any one posting.
func TestCrosscheck_MostlyAbsentBatchIsNotJudged(t *testing.T) {
	got := Crosscheck([]Posting{
		posting(1, "acme", "Office Manager", false),
		posting(2, "acme", "Go Developer", false),
		posting(3, "acme", "Rust Developer", false),
	}, onBoard())

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none — two of three absent is the batch's rule", got.Stamp)
	}
	if got.Skips != (Skips{MostlyAbsent: 2}) {
		t.Errorf("skipped = %+v, want MostlyAbsent 2", got.Skips)
	}
}

// The board's titles go through the same normalization as the postings', so a city
// suffix on either side does not manufacture an absence.
func TestCrosscheck_MatchesAcrossACitySuffix(t *testing.T) {
	got := Crosscheck(
		[]Posting{posting(1, "acme", "Senior Backend Engineer", false)},
		onBoard("Senior Backend Engineer, Krakow"),
	)

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none — the city variant is the same role", got.Stamp)
	}
}

// Each pair is one role retitled between an employer's board and an aggregator, taken
// from the 2026-11-01 precision sample, where every one of them was a false stamp.
func TestCrosscheck_MatchesARetitledRole(t *testing.T) {
	for _, pair := range [][2]string{
		{"Account Executive - Fraud", "Account Executive Fraud"},
		{"Operador(a) de Caixa - ATACADÃO", "Operadora de Caixa ATACADÃO"},
		{"Data Engineer (m/w/d) 100%", "Data Engineer: Build Data Lakehouse & AI Pipelines"},
		{"Service Desk/System Administrator", "Support System Administrator"},
		{"Senior/Assistant Manager (Clinical Service Planning)", "Senior Assistant Manager/Assistant Manager, Clinical Services Planning & Integration, PHCC (7799)"},
		{"Hôte - H/F", "Hôte - H/F ANNIVERSAIRES / ANIMATIONS: MERCREDI /SAMEDI /DIMANCHE"},
		{"Servicetechnikerin/Servicetechniker (w/m/d)", "Servicetechnikerin/Servicetechniker (w/m/d) (Fachinformatiker/in - Systemintegration)"},
	} {
		got := Crosscheck([]Posting{posting(1, "acme", pair[1], false)}, onBoard(pair[0]))
		if len(got.Stamp) != 0 {
			t.Errorf("board %q did not carry %q", pair[0], pair[1])
		}
	}
}

// Leaning toward a match must not erase the difference between two roles. (Two-letter
// words are dropped, so "Go Developer" WOULD carry a Rust role — the generous side.)
func TestCrosscheck_KeepsDifferentRolesApart(t *testing.T) {
	got := Crosscheck(
		append(carried("acme"), posting(1, "acme", "Rust Developer", false)),
		onBoard("Python Developer"),
	)

	if !slices.Equal(got.Stamp, []int64{1}) {
		t.Errorf("stamp = %v, want [1] — a Python board does not carry a Rust role", got.Stamp)
	}
}

// A posting whose title normalizes to nothing has no key, so it can neither match
// nor be judged absent. Stamping it would make every untitled posting evidence.
func TestCrosscheck_UntitledPostingIsSkipped(t *testing.T) {
	got := Crosscheck([]Posting{posting(1, "acme", "   ", false)}, onBoard("Go Developer"))

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none for a posting with no usable title", got.Stamp)
	}
	if got.Skips != (Skips{NoTitle: 1}) {
		t.Errorf("skipped = %+v, want NoTitle 1", got.Skips)
	}
}

// A posting with no company is not a company's posting. Grouped together, the
// empty-slug postings were checked against every board that also lacked a company
// and became the largest "company" in the stamp list; a stamp one still carries goes.
func TestCrosscheck_PostingWithNoCompanyIsSkippedAndCleared(t *testing.T) {
	got := Crosscheck(
		[]Posting{posting(1, "", "Go Developer", false), posting(2, "", "Rust Developer", true)},
		onBoard(),
	)

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none for postings that name no employer", got.Stamp)
	}
	if !slices.Equal(got.Clear, []int64{2}) {
		t.Errorf("clear = %v, want [2] — an old stamp on a company-less posting must go", got.Clear)
	}
	if got.Skips != (Skips{NoCompany: 2}) {
		t.Errorf("skipped = %+v, want NoCompany 2", got.Skips)
	}
}

// A recruiter advertising a client's role is legitimately absent from its own board,
// which is exactly the false accusation this signal is known for.
func TestCrosscheck_RecruiterPostingIsSkippedAndCleared(t *testing.T) {
	recruiter := posting(1, "acme-staffing", "Go Developer", true)
	recruiter.ViaRecruiter = true
	got := Crosscheck(append(carried("acme-staffing"), recruiter), onBoard())

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none for a recruiter's posting", got.Stamp)
	}
	if !slices.Equal(got.Clear, []int64{1}) {
		t.Errorf("clear = %v, want [1]", got.Clear)
	}
	if got.Skips.ViaRecruiter != 1 {
		t.Errorf("skipped = %+v, want ViaRecruiter 1", got.Skips)
	}
}

// An agency's postings advertise clients' roles whether or not each one says so, so a
// company that says it in enough of them is an intermediary throughout.
func TestCrosscheck_RecruiterCompanyIsNotJudged(t *testing.T) {
	board := onBoard()
	board.RecruiterPostings = minRecruiterPostings
	got := Crosscheck(append(carried("acme-staffing"), posting(1, "acme-staffing", "Go Developer", false)), board)

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none for an agency's posting", got.Stamp)
	}
	if got.Skips.RecruiterCompany != 4 {
		t.Errorf("skipped = %+v, want RecruiterCompany 4", got.Skips)
	}
}
