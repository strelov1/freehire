package sources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeHurma serves the paginated list from list[page] and each vacancy's detail from
// detail[id], recording the XHR header the adapter must present.
type fakeHurma struct {
	list      map[int]string
	detail    map[string]string
	gotHeader string
	gotList   []string
}

func (f *fakeHurma) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	f.gotHeader = headers["X-Requested-With"]
	// Detail URLs end in /public-vacancies/<id>; list URLs carry ?page=.
	if i := strings.Index(url, "?page="); i >= 0 {
		f.gotList = append(f.gotList, url)
		page := url[i+len("?page=") : i+len("?page=")+1]
		raw, ok := f.list[int(page[0]-'0')]
		if !ok {
			return errors.New("fakeHurma: no canned list page " + page)
		}
		return json.Unmarshal([]byte(raw), v)
	}
	id := url[strings.LastIndex(url, "/")+1:]
	raw, ok := f.detail[id]
	if !ok {
		return errors.New("fakeHurma: no canned detail for " + id)
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestHurmaProvider(t *testing.T) {
	if got := NewHurma(nil).Provider(); got != "hurma" {
		t.Errorf("Provider() = %q, want %q", got, "hurma")
	}
}

func TestHurmaFetch(t *testing.T) {
	fake := &fakeHurma{
		list: map[int]string{
			1: `{"data":[
				{"name":"#406 Senior UI/UX Designer","public_url":"https://scrumlaunch.hurma.work/public-vacancies/406","residence":"remote","work_types":"Full-time, Remote work"}
			],"meta":{"current_page":1,"last_page":1}}`,
		},
		detail: map[string]string{
			"406": `{"data":{
				"name":"#406 Senior UI/UX Designer","residence":"remote","work_types":"Full-time, Remote work",
				"description":"We are looking for a designer.",
				"demand":"- 5+ years<br />",
				"responsibility":"- Design things<br />",
				"working_conditions":"- 12 vacation days<br />",
				"addition":"<script>x()</script>"
			}}`,
		},
	}

	jobs, err := NewHurma(fake).Fetch(context.Background(), CompanyEntry{
		Company: "ScrumLaunch", Provider: "hurma", Board: "scrumlaunch",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if fake.gotHeader != "XMLHttpRequest" {
		t.Errorf("X-Requested-With header = %q, want XMLHttpRequest", fake.gotHeader)
	}
	if len(fake.gotList) == 0 || !strings.Contains(fake.gotList[0], "scrumlaunch.hurma.work") {
		t.Errorf("list URL %v should target the board subdomain", fake.gotList)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "406" {
		t.Errorf("ExternalID = %q, want 406 from the public_url", j.ExternalID)
	}
	if j.URL != "https://scrumlaunch.hurma.work/public-vacancies/406" {
		t.Errorf("URL = %q", j.URL)
	}
	// The "#406 " tag is stripped from the title.
	if j.Title != "Senior UI/UX Designer" {
		t.Errorf("Title = %q, want the tag stripped", j.Title)
	}
	if j.Company != "ScrumLaunch" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "remote" {
		t.Errorf("Location = %q", j.Location)
	}
	// Description stitches the sections and drops the script tag.
	for _, want := range []string{"looking for a designer", "5+ years", "Design things", "12 vacation days"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q, got %q", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "<script") {
		t.Errorf("Description retained a script tag, got %q", j.Description)
	}
	if !j.Remote {
		t.Error("Remote = false, want true from residence/work_types")
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
}

// A vacancy whose public_url carries no numeric id is skipped rather than emitted with a
// colliding empty external id.
func TestHurmaSkipsVacancyWithoutID(t *testing.T) {
	fake := &fakeHurma{
		list: map[int]string{
			1: `{"data":[
				{"name":"No id","public_url":"https://x.hurma.work/public-vacancies/","residence":"remote","work_types":"Full-time"}
			],"meta":{"current_page":1,"last_page":1}}`,
		},
		detail: map[string]string{},
	}
	jobs, err := NewHurma(fake).Fetch(context.Background(), CompanyEntry{Company: "X", Provider: "hurma", Board: "x"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len(jobs) = %d, want 0 (id-less vacancy skipped)", len(jobs))
	}
}

func TestHurmaEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Full-time, Remote work": "full_time",
		"Part-time":              "part_time",
		"Contract":               "contract",
		"Internship":             "internship",
		"Remote work":            "",
		"":                       "",
	}
	for in, want := range cases {
		if got := hurmaEmploymentType(in); got != want {
			t.Errorf("hurmaEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}
