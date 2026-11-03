package resumeextract

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/pii"
	"github.com/strelov1/freehire/internal/platform/llm"
)

func TestSanitize_BoundsStringsAndYears(t *testing.T) {
	s := Structured{
		FullName:   strings.Repeat("a", maxNameRunes+50),
		Summary:    strings.Repeat("b", maxSummaryRunes+500),
		TotalYears: maxYears + 100,
		Experience: []Experience{{
			Title:   strings.Repeat("c", maxShortRunes+50),
			Company: "Acme",
			Summary: strings.Repeat("d", maxEntrySummaryRunes+500),
		}},
	}
	s.Sanitize()

	if got := len([]rune(s.FullName)); got > maxNameRunes {
		t.Errorf("FullName runes = %d, want <= %d", got, maxNameRunes)
	}
	if got := len([]rune(s.Summary)); got > maxSummaryRunes {
		t.Errorf("Summary runes = %d, want <= %d", got, maxSummaryRunes)
	}
	if s.TotalYears > maxYears {
		t.Errorf("TotalYears = %d, want <= %d", s.TotalYears, maxYears)
	}
	if got := len([]rune(s.Experience[0].Title)); got > maxShortRunes {
		t.Errorf("Experience title runes = %d, want <= %d", got, maxShortRunes)
	}
	if got := len([]rune(s.Experience[0].Summary)); got > maxEntrySummaryRunes {
		t.Errorf("Experience summary runes = %d, want <= %d", got, maxEntrySummaryRunes)
	}
}

func TestSanitize_NegativeYearsCoercedToZero(t *testing.T) {
	s := Structured{TotalYears: -3}
	s.Sanitize()
	if s.TotalYears != 0 {
		t.Errorf("TotalYears = %d, want 0", s.TotalYears)
	}
}

func TestSanitize_CapsArrayCardinality(t *testing.T) {
	exp := make([]Experience, maxExperience+10)
	for i := range exp {
		exp[i] = Experience{Title: "Engineer", Company: "Acme"}
	}
	langs := make([]string, maxLanguages+10)
	for i := range langs {
		langs[i] = "English"
	}
	s := Structured{Experience: exp, Languages: langs}
	s.Sanitize()
	if len(s.Experience) > maxExperience {
		t.Errorf("Experience len = %d, want <= %d", len(s.Experience), maxExperience)
	}
	if len(s.Languages) > maxLanguages {
		t.Errorf("Languages len = %d, want <= %d", len(s.Languages), maxLanguages)
	}
}

func TestSanitize_DropsEmptyEntries(t *testing.T) {
	s := Structured{
		Experience: []Experience{
			{Title: "Engineer", Company: "Acme"},
			{},                            // wholly empty → dropped
			{Title: "   ", Company: "\t"}, // whitespace-only → dropped
		},
		Languages: []string{"English", "", "  ", "German"},
		Links:     []string{"https://x.dev", ""},
	}
	s.Sanitize()
	if len(s.Experience) != 1 {
		t.Errorf("Experience len = %d, want 1 (empties dropped)", len(s.Experience))
	}
	if len(s.Languages) != 2 {
		t.Errorf("Languages = %v, want 2 non-empty", s.Languages)
	}
	if len(s.Links) != 1 {
		t.Errorf("Links = %v, want 1 non-empty", s.Links)
	}
}

// queuedModel returns a canned response for the single extraction call.
type queuedModel struct {
	resp string
	err  error
	n    int
}

func (m *queuedModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.n++
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: m.resp}}}, nil
}
func (*queuedModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

func TestExtract_ParsesAndSanitizes(t *testing.T) {
	raw := `{"full_name":"Jane Doe","summary":"Backend engineer.","total_years":-5,` +
		`"experience":[{"title":"Senior Go Engineer","company":"Acme","start":"2020","end":"Present"}],` +
		`"languages":["English",""]}`
	m := &queuedModel{resp: raw}
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), "Senior Go Engineer, Acme, 2020 - Present")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 1 {
		t.Errorf("LLM calls = %d, want 1", m.n)
	}
	// Contacts now come from detection (none here), so the model's full_name is ignored;
	// the semantic fields are what this test covers.
	if len(got.Experience) != 1 {
		t.Errorf("parsed = %+v, want 1 experience", got)
	}
	if got.FullName != "" {
		t.Errorf("FullName = %q, want empty (no detector spans, not the model's value)", got.FullName)
	}
	if got.TotalYears != 0 { // sanitized: negative → 0
		t.Errorf("TotalYears = %d, want 0 (sanitized)", got.TotalYears)
	}
	if len(got.Languages) != 1 { // sanitized: empty dropped
		t.Errorf("Languages = %v, want 1 (empty dropped)", got.Languages)
	}
}

func TestExtract_UnconfiguredReturnsErrDisabled(t *testing.T) {
	_, err := NewExtractor(nil, noopDetector{}).Extract(context.Background(), "cv")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
}

func TestExtract_BadJSONErrors(t *testing.T) {
	m := &queuedModel{resp: "not json"}
	if _, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), "cv"); err == nil {
		t.Fatal("want error on unparseable model output, got nil")
	}
}

func TestExtractor_EnabledReflectsClient(t *testing.T) {
	if NewExtractor(nil, noopDetector{}).Enabled() {
		t.Error("nil-client extractor should be disabled")
	}
	if NewExtractor(llm.NewWithModel(&queuedModel{}), nil).Enabled() {
		t.Error("nil-detector extractor should be disabled (fail-closed)")
	}
	if !NewExtractor(llm.NewWithModel(&queuedModel{}), noopDetector{}).Enabled() {
		t.Error("configured extractor should be enabled")
	}
}

// cvWithDates is a CV whose dates the tests below hold the model's answer against.
const cvWithDates = `Senior Go Engineer - Acme
05/2016 - 06/2024 | Berlin
Go Engineer - Initech
12/2013 - 04/2016 | Berlin`

// scriptedModel answers each call in turn, repeating the last answer when it runs out.
type scriptedModel struct {
	resps []string
	n     int
}

func (m *scriptedModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	i := min(m.n, len(m.resps)-1)
	m.n++
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: m.resps[i]}}}, nil
}
func (*scriptedModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

const (
	rightDates = `{"experience":[` +
		`{"company":"Acme","start":{"year":2016,"month":5},"end":{"year":2024,"month":6}},` +
		`{"company":"Initech","start":{"year":2013,"month":12},"end":{"year":2016,"month":4}}]}`
	// the shape seen in production: months right, years pulled from the neighbouring role
	shiftedDates = `{"experience":[` +
		`{"company":"Acme","start":{"year":2023,"month":5},"end":{"year":2024,"month":6}},` +
		`{"company":"Initech","start":{"year":2016,"month":12},"end":{"year":2016,"month":4}}]}`
	// a year the CV never prints
	inventedYear = `{"experience":[` +
		`{"company":"Acme","start":{"year":2019,"month":5},"end":{"year":2024,"month":6}}]}`
)

func TestExtract_CorrectDatesCostOneCall(t *testing.T) {
	m := &scriptedModel{resps: []string{rightDates}}
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), cvWithDates)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 1 {
		t.Errorf("LLM calls = %d, want 1: supported dates must not trigger a retry", m.n)
	}
	if got.Experience[0].Start == nil || got.Experience[0].Start.Year != 2016 {
		t.Errorf("start = %v, want 2016", got.Experience[0].Start)
	}
}

func TestExtract_ShiftedDatesAreRetriedAndTheBetterAnswerKept(t *testing.T) {
	m := &scriptedModel{resps: []string{shiftedDates, rightDates}}
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), cvWithDates)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 2 {
		t.Fatalf("LLM calls = %d, want 2", m.n)
	}
	if st := got.Experience[0].Start; st == nil || st.Year != 2016 || st.Month != 5 {
		t.Errorf("Acme start = %v, want 05/2016 from the retry", st)
	}
}

func TestExtract_RetryNeverReplacesABetterAnswerWithAWorseOne(t *testing.T) {
	// First answer: one doubtful month. Retry: a year the CV never prints, which is worse.
	doubtful := `{"experience":[{"company":"Acme","start":{"year":2016,"month":9},"end":{"year":2024,"month":6}}]}`
	m := &scriptedModel{resps: []string{doubtful, inventedYear}}
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), cvWithDates)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 2 {
		t.Fatalf("LLM calls = %d, want 2", m.n)
	}
	if st := got.Experience[0].Start; st == nil || st.Year != 2016 {
		t.Errorf("start = %v, want the first answer's 2016 kept", st)
	}
}

func TestExtract_YearTheCVNeverPrintsIsDroppedNotStored(t *testing.T) {
	m := &scriptedModel{resps: []string{inventedYear}} // the retry repeats it
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), cvWithDates)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	e := got.Experience[0]
	if e.Start != nil {
		t.Errorf("start = %v, want nil: 2019 is not in the CV", e.Start)
	}
	if e.End == nil || e.End.Year != 2024 {
		t.Errorf("end = %v, want 2024 kept: it is in the CV", e.End)
	}
}

func TestExtract_PairMissingFromNumericCVIsRetriedButNotDeleted(t *testing.T) {
	// Both answers use only years the CV prints, so nothing is certainly wrong: the doubtful
	// pair buys a retry, and when the retry is no better the model's answer stands.
	wrongMonth := `{"experience":[{"company":"Acme","start":{"year":2016,"month":9},"end":{"year":2024,"month":6}}]}`
	m := &scriptedModel{resps: []string{wrongMonth}}
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), cvWithDates)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 2 {
		t.Errorf("LLM calls = %d, want 2", m.n)
	}
	if st := got.Experience[0].Start; st == nil || st.Year != 2016 {
		t.Errorf("start = %v, want the model's 2016 kept", st)
	}
}

func TestDateProblems_NonNumericCVOnlyChecksYears(t *testing.T) {
	cv := "Engineer, Acme, August 2019 to March 2021"
	s := Structured{Experience: []Experience{{Start: &perioddate.PeriodDate{Year: 2019, Month: 8}}}}
	if hard, soft := dateProblems(s, cv); hard != 0 || soft != 0 {
		t.Errorf("problems = %d/%d, want 0/0: month names are not numeric evidence", hard, soft)
	}
}

// The model only ever sees the redacted text, so a year the redactor eats is a year the model
// must invent. This is the production report end to end: five consecutive "MM/YYYY - MM/YYYY"
// roles, every year present in the text the model is given.
func TestExtract_ModelSeesEveryYearOfAMonthPrecisionHistory(t *testing.T) {
	cv := "A\n08/2023 - 07/2024 | X\nB\n05/2016 - 06/2024 | X\nC\n12/2015 - 04/2016 | X\nD\n07/2013 - 02/2015 | X\nE\n06/2010 - 04/2013 | X\n"
	red, err := pii.Build(context.Background(), cv, noopDetector{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	seen := red.Redact(cv)
	for _, want := range []string{"08/2023 - 07/2024", "05/2016 - 06/2024", "12/2015 - 04/2016", "07/2013 - 02/2015", "06/2010 - 04/2013"} {
		if !strings.Contains(seen, want) {
			t.Errorf("model text lost %q:\n%s", want, seen)
		}
	}
}

// A CV that abbreviates years prints no four-digit year at all. The model's reading of it is
// still right, and deleting it for lacking a four-digit match would undate the whole history.
func TestExtract_TwoDigitYearCVKeepsItsDates(t *testing.T) {
	cv := "Engineer - Acme\n01/16 - 06/24 | Berlin\nAnalyst - Initech\nJan '12 - Mar '15 | Berlin\nIntern - Hooli\nSep 10 to Dec 11"
	resp := `{"experience":[` +
		`{"company":"Acme","start":{"year":2016,"month":1},"end":{"year":2024,"month":6}},` +
		`{"company":"Initech","start":{"year":2012,"month":1},"end":{"year":2015,"month":3}},` +
		`{"company":"Hooli","start":{"year":2010,"month":9},"end":{"year":2011,"month":12}}]}`
	m := &scriptedModel{resps: []string{resp}}
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), cv)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 1 {
		t.Errorf("LLM calls = %d, want 1", m.n)
	}
	for _, e := range got.Experience {
		if e.Start == nil || e.End == nil {
			t.Errorf("%s: dates dropped: start=%v end=%v", e.Company, e.Start, e.End)
		}
	}
}

func TestDateEvidence_BareTwoDigitNumberIsNotAYear(t *testing.T) {
	ev := collectDateEvidence("Automated reporting for 14 cement plants, 10-20 managers")
	if ev.years[2014] || ev.years[2010] {
		t.Errorf("years = %v: a bare number must not count as a year", ev.years)
	}
}

func TestDateEvidence_YearsGluedToLettersCount(t *testing.T) {
	ev := collectDateEvidence("FY2016 plan; Aug2019 launch; ref 120161 is not a year")
	if !ev.years[2016] || !ev.years[2019] {
		t.Errorf("years = %v, want 2016 and 2019", ev.years)
	}
	if ev.years[2016] && ev.years[2012] {
		t.Errorf("years = %v: digits inside a longer number are not a year", ev.years)
	}
}

func TestDateEvidence_RangeSeamIsNotAPair(t *testing.T) {
	ev := collectDateEvidence("05/2016 - 06/2024\n12/2013 - 04/2016")
	if ev.pairs[perioddate.PeriodDate{Year: 2016, Month: 6}] {
		t.Error("pair (2016, 06) comes from the seam between two dates, not from any date")
	}
	if !ev.pairs[perioddate.PeriodDate{Year: 2016, Month: 5}] {
		t.Error("pair (2016, 05) is printed and must be evidence")
	}
}

func TestExtract_SkipsTheRetryWhenTheDeadlineCannotAffordIt(t *testing.T) {
	m := &scriptedModel{resps: []string{shiftedDates, rightDates}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(ctx, cvWithDates)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 1 {
		t.Errorf("LLM calls = %d, want 1: 20s left is not enough for a second call", m.n)
	}
	if got.Experience[0].End == nil {
		t.Error("the first answer must still be returned and stored")
	}
}

// A certain error outranks any number of doubtful ones: the retry below has fewer problems in
// total but one of them is an invented year, which would cost the role its date.
func TestExtract_RetryWithAHardProblemDoesNotReplaceDoubtfulOnes(t *testing.T) {
	twoDoubtful := `{"experience":[` +
		`{"company":"Acme","start":{"year":2016,"month":9},"end":{"year":2024,"month":2}},` +
		`{"company":"Initech","start":{"year":2013,"month":2},"end":{"year":2016,"month":4}}]}`
	oneInvented := `{"experience":[` +
		`{"company":"Acme","start":{"year":2019,"month":5},"end":{"year":2024,"month":6}},` +
		`{"company":"Initech","start":{"year":2013,"month":12},"end":{"year":2016,"month":4}}]}`
	m := &scriptedModel{resps: []string{twoDoubtful, oneInvented}}
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), cvWithDates)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 2 {
		t.Fatalf("LLM calls = %d, want 2", m.n)
	}
	if st := got.Experience[0].Start; st == nil || st.Year != 2016 {
		t.Errorf("Acme start = %v, want 2016 from the first answer: the retry's 2019 is invented", st)
	}
}

func TestExtract_EmptyRetryDoesNotReplaceAPopulatedFirstAnswer(t *testing.T) {
	doubtful := `{"experience":[{"company":"Acme","start":{"year":2016,"month":9},"end":{"year":2024,"month":6}}]}`
	m := &scriptedModel{resps: []string{doubtful, `{"experience":[]}`}}
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), cvWithDates)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 2 {
		t.Fatalf("LLM calls = %d, want 2", m.n)
	}
	if len(got.Experience) != 1 {
		t.Errorf("experience = %d roles, want the first answer's 1: an empty retry has no date problems only because it has no roles", len(got.Experience))
	}
}

func TestDateEvidence_AbbreviatedRangeEndCountsAsAYear(t *testing.T) {
	ev := collectDateEvidence("Engineer, Acme, 2016–24\nAnalyst, Initech, 2010 - 13")
	for _, y := range []int{2024, 2013} {
		if !ev.years[y] {
			t.Errorf("years = %v, want %d from the abbreviated end of the range", ev.years, y)
		}
	}
}

func TestDateEvidence_MonthAfterARangeStartIsNotAnAbbreviatedYear(t *testing.T) {
	ev := collectDateEvidence("05/2016 - 06/2024")
	if ev.years[2006] || ev.years[1906] {
		t.Errorf("years = %v: \"06\" here is a month, not the end of the range", ev.years)
	}
}

func TestDateEvidence_IsoMonthAndHyphenSeamAreNotAbbreviatedYears(t *testing.T) {
	ev := collectDateEvidence("Analyst 2016-08 to 2019-02\nEngineer 05-2016 - 06-2024")
	for _, y := range []int{2008, 2006, 2002, 1906} {
		if ev.years[y] {
			t.Errorf("years = %v: %d comes from a month, not a year", ev.years, y)
		}
	}
}

func TestExtract_RetryThatErasesTheDatesDoesNotWin(t *testing.T) {
	doubtful := `{"experience":[{"company":"Acme","start":{"year":2016,"month":9},"end":{"year":2024,"month":6}}]}`
	undated := `{"experience":[{"company":"Acme"}]}`
	m := &scriptedModel{resps: []string{doubtful, undated}}
	got, err := NewExtractor(llm.NewWithModel(m), noopDetector{}).Extract(context.Background(), cvWithDates)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 2 {
		t.Fatalf("LLM calls = %d, want 2", m.n)
	}
	if e := got.Experience[0]; e.Start == nil || e.End == nil {
		t.Errorf("dates = %v..%v, want the first answer's: a retry with no dates has no date problems only because it has no dates", e.Start, e.End)
	}
}
