package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

type joinGraphQLFake struct {
	mu        sync.Mutex
	pages     map[int]string
	detail    string
	detailErr error
	inputs    []map[string]any
}

func (f *joinGraphQLFake) PostJSON(_ context.Context, endpoint string, body, result any) error {
	if endpoint != joinGraphQLURL {
		return fmt.Errorf("unexpected endpoint %s", endpoint)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	request := body.(map[string]any)
	input := request["variables"].(map[string]any)["input"].(map[string]any)
	f.inputs = append(f.inputs, input)
	if request["query"] == joinListQuery {
		page := input["paginationPage"].(map[string]int)["page"]
		value, ok := f.pages[page]
		if !ok {
			return fmt.Errorf("missing page %d", page)
		}
		return json.Unmarshal([]byte(value), result)
	}
	if f.detailErr != nil {
		return f.detailErr
	}
	return json.Unmarshal([]byte(f.detail), result)
}
func joinPage(items string, pages, rows int) string {
	return fmt.Sprintf(`{"data":{"publicJobs":{"items":%s,"pageInfo":{"pageCount":%d,"rowCount":%d}}}}`, items, pages, rows)
}
func joinItem(id int) string {
	return fmt.Sprintf(`{"id":%d,"idParam":"%d-engineer","title":"Engineer","workplaceType":"REMOTE","city":{"cityName":"Berlin","countryName":"Germany"}}`, id, id+100)
}
func TestJoinGraphQLMapsAndPaginates(t *testing.T) {
	f := &joinGraphQLFake{pages: map[int]string{1: joinPage("["+joinItem(1)+"]", 2, 2), 2: joinPage("["+joinItem(2)+"]", 2, 2)}, detail: `{"data":{"publicJob":{"descriptionHtml":"<p>Build things</p><script>alert(1)</script>","company":{"domain":"acme"}}}}`}
	jobs, err := (join{http: f}).Fetch(context.Background(), CompanyEntry{Board: "54", Company: "Acme"})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	for _, j := range jobs {
		if !j.Remote || j.Location != "Berlin, Germany" || j.Company != "Acme" || !strings.Contains(j.Description, "<p>Build things</p>") || strings.Contains(j.Description, "<script") {
			t.Fatalf("bad mapping: %+v", j)
		}
	}
	if jobs[0].ExternalID != "1" || jobs[0].URL != "https://join.com/companies/acme/101-engineer" {
		t.Fatal(jobs[0])
	}
	if f.inputs[0]["companyId"] != int64(54) {
		t.Fatal(f.inputs[0])
	}
}
func TestJoinGraphQLRejectsIncompleteListings(t *testing.T) {
	for name, pages := range map[string]map[int]string{
		"missing data": {1: `{}`}, "errors with partial data": {1: `{"errors":[{"message":"denied"}],"data":{"publicJobs":{"items":[],"pageInfo":{"pageCount":0,"rowCount":0}}}}`},
		"null items": {1: joinPage("null", 0, 0)}, "missing metadata": {1: `{"data":{"publicJobs":{"items":[]}}}`},
		"missing later page": {1: joinPage("["+joinItem(1)+"]", 2, 2)},
		"repeat":             {1: joinPage("["+joinItem(1)+"]", 2, 2), 2: joinPage("["+joinItem(1)+"]", 2, 2)},
		"short count":        {1: joinPage("["+joinItem(1)+"]", 1, 2)},
		"changed total":      {1: joinPage("["+joinItem(1)+"]", 2, 2), 2: joinPage("["+joinItem(2)+"]", 2, 3)},
	} {
		t.Run(name, func(t *testing.T) {
			jobs, err := (join{http: &joinGraphQLFake{pages: pages}}).Fetch(context.Background(), CompanyEntry{Board: "54"})
			if err == nil || len(jobs) > 0 {
				t.Fatalf("jobs=%v err=%v", jobs, err)
			}
		})
	}
}
func TestJoinGraphQLEmpty(t *testing.T) {
	jobs, err := (join{http: &joinGraphQLFake{pages: map[int]string{1: joinPage("[]", 0, 0)}}}).Fetch(context.Background(), CompanyEntry{Board: "54"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("%v %v", jobs, err)
	}
}
func TestJoinGraphQLFailedDetailRetainsIdentity(t *testing.T) {
	for _, detail := range []string{`{"data":{"publicJob":null}}`, `{"errors":[{"message":"temporarily unavailable"}]}`, `{"data":{"publicJob":{"descriptionHtml":""}}}`} {
		f := &joinGraphQLFake{pages: map[int]string{1: joinPage("["+joinItem(1)+"]", 1, 1)}, detail: detail}
		jobs, err := (join{http: f}).Fetch(context.Background(), CompanyEntry{Board: "54"})
		if err != nil || len(jobs) != 1 || !jobs[0].Unreadable {
			t.Fatalf("%v %v", jobs, err)
		}
	}
}
