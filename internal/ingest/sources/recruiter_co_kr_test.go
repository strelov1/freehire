package sources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

// rckFake answers the listing POST (by page number) and detail GETs (by position id).
type rckFake struct {
	pages   map[int]string
	details map[string]string
	errs    map[string]error

	mu      sync.Mutex
	headers []string
}

func (f *rckFake) PostJSONWithHeaders(_ context.Context, _ string, h map[string]string, body, v any) error {
	f.mu.Lock()
	f.headers = append(f.headers, h["prefix"])
	f.mu.Unlock()
	raw, _ := json.Marshal(body)
	var b struct {
		Pageable struct {
			Page int `json:"page"`
		} `json:"pageableRq"`
	}
	_ = json.Unmarshal(raw, &b)
	return json.Unmarshal([]byte(f.pages[b.Pageable.Page]), v)
}

func (f *rckFake) GetJSONWithHeaders(_ context.Context, u string, _ map[string]string, v any) error {
	id := u[strings.LastIndex(u, "/")+1:]
	if err := f.errs[id]; err != nil {
		return err
	}
	raw, ok := f.details[id]
	if !ok {
		return &StatusError{Method: "GET", Code: 400, URL: u}
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestRecruiterCoKRProvider(t *testing.T) {
	if got := NewRecruiterCoKR(nil).Provider(); got != "recruiter_co_kr" {
		t.Errorf("Provider() = %q", got)
	}
}

func TestRecruiterCoKRFetch(t *testing.T) {
	f := &rckFake{
		pages: map[int]string{
			1: `{"pagination":{"totalPages":2},"list":[
			 {"positionSn":1,"title":" 백엔드 개발자 ","submissionStatus":"IN_SUBMISSION","recruitmentType":"GENERAL","startDateTime":"2026-10-30T00:00:00","endDateTime":"2026-11-30T00:00:59"},
			 {"positionSn":2,"title":"마감 공고","submissionStatus":"POST_SUBMISSION","recruitmentType":"GENERAL","startDateTime":"2026-09-01T09:00:00","endDateTime":"2026-09-10T23:00:00"},
			 {"positionSn":0,"title":"no id","submissionStatus":"IN_SUBMISSION"}]}`,
			2: `{"pagination":{"totalPages":2},"list":[
			 {"positionSn":3,"title":"인재 Pool","submissionStatus":"IN_SUBMISSION","recruitmentType":"PERMANENT","startDateTime":"2025-10-14T16:00:00","endDateTime":"2030-12-31T00:00:59"},
			 {"positionSn":4,"title":"flaky","submissionStatus":"IN_SUBMISSION","recruitmentType":"GENERAL"},
			 {"positionSn":5,"title":"gone","submissionStatus":"IN_SUBMISSION","recruitmentType":"GENERAL"}]}`,
		},
		details: map[string]string{
			"1": `{"jobDescription":"<p>서버 개발</p>"}`,
			"3": `{"jobDescription":"<p>상시</p>"}`,
		},
		errs: map[string]error{"4": &StatusError{Method: "GET", Code: 503, URL: "x"}},
	}
	jobs, err := NewRecruiterCoKR(f).Fetch(context.Background(), CompanyEntry{Board: "Cosmax.recruiter.co.kr", Company: "Cosmax"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (closed, id-less and gone skipped): %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "1" || j.Title != "백엔드 개발자" || j.URL != "https://cosmax.recruiter.co.kr/career/jobs/1" ||
		j.Company != "Cosmax" || !strings.Contains(j.Description, "서버 개발") {
		t.Errorf("job 1 = %+v", j)
	}
	// 2026-10-30T00:00 KST is the previous day 15:00 UTC.
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02T15:04") != "2026-10-29T15:00" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.ApplicationDeadline == nil || j.ApplicationDeadline.Format("2006-01-02") != "2026-11-30" {
		t.Errorf("ApplicationDeadline = %v", j.ApplicationDeadline)
	}
	if jobs[1].ExternalID != "3" || jobs[1].ApplicationDeadline != nil {
		t.Errorf("permanent posting should carry no deadline: %+v", jobs[1])
	}
	if !jobs[2].Unreadable || jobs[2].ExternalID != "4" {
		t.Errorf("a 503 detail must yield an unreadable marker: %+v", jobs[2])
	}
	for _, h := range f.headers {
		if h != "cosmax.recruiter.co.kr" {
			t.Errorf("prefix header = %q", h)
		}
	}
}

func TestRecruiterCoKRBadBoard(t *testing.T) {
	for _, b := range []string{"", "cosmax", "https://cosmax.recruiter.co.kr", "a.b/c"} {
		if _, err := NewRecruiterCoKR(&rckFake{}).Fetch(context.Background(), CompanyEntry{Board: b}); err == nil {
			t.Errorf("board %q: want error", b)
		}
	}
}

func TestRecruiterCoKRErrorDocumentFails(t *testing.T) {
	f := &rckFake{pages: map[int]string{1: `{"code":"InvalidAccessRequestException"}`}}
	if _, err := NewRecruiterCoKR(f).Fetch(context.Background(), CompanyEntry{Board: "x.recruiter.co.kr"}); err == nil {
		t.Error("an error document must fail the crawl, not read as an empty board")
	}
}

func TestRCKGone(t *testing.T) {
	if !rckGone(&StatusError{Code: 400}) || rckGone(&StatusError{Code: 503}) || rckGone(errors.New("net")) {
		t.Error("rckGone misclassifies")
	}
}
