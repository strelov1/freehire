package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
)

// engageFake serves the recorded search page as page 1 and checks the p_t stitch on later pages.
type engageFake struct {
	t     *testing.T
	pages map[string]string // page number -> body
	asked []url.Values
}

func newEngageFake(t *testing.T) *engageFake {
	t.Helper()
	b, err := os.ReadFile("testdata/engage/search_400000_p1.json")
	if err != nil {
		t.Fatal(err)
	}
	// The recorded page is the whole IT category (17,152 rows); the fake states a total inside the
	// search's result window, as one of its narrower codes would.
	body := strings.Replace(string(b), `"totalCount": 17152`, `"totalCount": 9152`, 1)
	return &engageFake{t: t, pages: map[string]string{"1": body}}
}

func (f *engageFake) GetJSON(_ context.Context, raw string, v any) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	q := u.Query()
	f.asked = append(f.asked, q)
	body, ok := f.pages[q.Get("page")]
	if !ok {
		return &StatusError{Method: "GET", Code: 500, URL: raw}
	}
	return json.Unmarshal([]byte(body), v)
}

func engageByID(jobs []Job) map[string]Job {
	m := map[string]Job{}
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

func TestEngageMapsSearchRowsAndSkipsSyndicatedOnes(t *testing.T) {
	f := newEngageFake(t)
	// The recorded page says hasMorePages; make page 2 the end.
	f.pages["2"] = `{"result":"success","totalCount":17152,"searchResult":[],"paginator":{"hasMorePages":false}}`
	jobs, err := NewEngage(f).Fetch(context.Background(), CompanyEntry{Board: "400000"})
	if err != nil {
		t.Fatal(err)
	}
	got := engageByID(jobs)
	// Four rows: three engage postings and one syndicated (is_kb_work, hex id) row.
	if len(got) != 3 {
		t.Fatalf("jobs = %d, want 3: %+v", len(got), jobs)
	}
	if p2 := f.asked[1]; p2.Get("p_t") != "9152" || p2.Get("f_t") != "0" || p2.Get("job") != "400000" {
		t.Errorf("page 2 query = %v, want p_t carried from page 1", p2)
	}

	j := got["6870203"]
	if j.URL != "https://en-gage.net/staffservice-engineering_saiyo/work_6870203/" {
		t.Errorf("url = %q", j.URL)
	}
	if j.Title != "ITサポート" || j.Company != "株式会社スタッフサービス エンジニアリング事業本部" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if !j.IsTechHint || j.Location != "長崎県長崎市" || len(j.Countries) != 1 || j.Countries[0] != "jp" {
		t.Errorf("tech/location/countries = %v %q %v", j.IsTechHint, j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment = %q, want permanent dispatch read as full_time", j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 3000000 || j.SalaryMax == nil || *j.SalaryMax != 7000000 ||
		j.SalaryPeriod != "year" || j.SalaryCurrency != "JPY" {
		t.Errorf("salary = %v %v %q %q", j.SalaryMin, j.SalaryMax, j.SalaryPeriod, j.SalaryCurrency)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format("2006-01-02 15:04") != "2026-10-21 15:00" {
		t.Errorf("posted = %v, want 2026-10-22 00:00 JST", j.PostedAt)
	}
	for _, h := range []string{"仕事内容", "応募資格", "待遇・福利厚生"} {
		if !strings.Contains(j.Description, h) {
			t.Errorf("description lacks %s", h)
		}
	}

	// The second row shape: numbers as strings, an hourly wage.
	h := got["16313033"]
	if h.Company != "株式会社マーキュリー" || h.SalaryPeriod != "hour" || h.SalaryMin == nil || *h.SalaryMin != 1750 ||
		h.EmploymentType != "contract" {
		t.Errorf("hourly row = %q %q %v %q", h.Company, h.SalaryPeriod, h.SalaryMin, h.EmploymentType)
	}

	// A fully remote posting, and a registered name typed in full-width letters.
	r := got["18443386"]
	if r.WorkMode != "remote" || !r.Remote || r.Location != "" || r.Company != "株式会社Widsley" {
		t.Errorf("remote row = %q %v %q %q", r.WorkMode, r.Remote, r.Location, r.Company)
	}
}

func TestEngageFirstPageFailingFailsTheBoard(t *testing.T) {
	f := newEngageFake(t)
	f.pages["1"] = `{"result":"error","error_message":"","searchResult":[]}`
	if _, err := NewEngage(f).Fetch(context.Background(), CompanyEntry{Board: "400000"}); err == nil {
		t.Error("want an error when the search refuses the first page")
	}
}

func TestEngageLaterPageFailingKeepsWhatWasRead(t *testing.T) {
	f := newEngageFake(t) // page 2 is not served: a 500
	jobs, err := NewEngage(f).Fetch(context.Background(), CompanyEntry{Board: "400000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Errorf("jobs = %d, want the 3 read before the failed page", len(jobs))
	}
}

func TestEngageRejectsABoardThatIsNotACategory(t *testing.T) {
	_, err := NewEngage(newEngageFake(t)).Fetch(context.Background(), CompanyEntry{Board: "amazon_career2"})
	var se *StatusError
	if err == nil || errors.As(err, &se) {
		t.Errorf("err = %v, want a validation error before any request", err)
	}
}

func TestEngageWorkIDTakesOnlyNumericIDs(t *testing.T) {
	for raw, want := range map[string]string{
		`6870203`:                            "6870203",
		`"16313033"`:                         "16313033",
		`"133ffbc2d39e8e312f77c94ff9d1180a"`: "",
		`null`:                               "",
	} {
		if got := engageWorkID(json.RawMessage(raw)); got != want {
			t.Errorf("engageWorkID(%s) = %q, want %q", raw, got, want)
		}
	}
}

func TestEngageCategoryPastTheResultWindowFails(t *testing.T) {
	f := newEngageFake(t)
	f.pages["1"] = strings.Replace(f.pages["1"], `"totalCount": 9152`, `"totalCount": 17152`, 1)
	if _, err := NewEngage(f).Fetch(context.Background(), CompanyEntry{Board: "400000"}); err == nil {
		t.Error("want a category the search cannot page through whole refused")
	}
}

func TestEngageIgnoredCodeFailsTheBoard(t *testing.T) {
	// The recorded rows are filed under 4055 (IT support) and others; asked as 401000 (IT
	// development), they are what the search answers when it ignores a code.
	if _, err := NewEngage(newEngageFake(t)).Fetch(context.Background(), CompanyEntry{Board: "401000"}); err == nil {
		t.Error("want a first page outside the board's category refused")
	}
}

func TestEngageCategoryPrefix(t *testing.T) {
	for code, want := range map[string]string{"400000": "40", "401000": "4010", "405500": "4055", "401015": "401015"} {
		if got := engageCategoryPrefix(code); got != want {
			t.Errorf("engageCategoryPrefix(%s) = %q, want %q", code, got, want)
		}
	}
}
