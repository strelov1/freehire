package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type adpMyJobsDetailFailure int

func (s adpMyJobsDetailFailure) GetJSONWithHeaders(_ context.Context, raw string, _ map[string]string, dst any) error {
	body := adpMyJobsSiteBody
	switch {
	case strings.Contains(raw, "job-requisitions/5002"):
		if s != 200 {
			return &StatusError{Code: int(s)}
		}
		body = `{"jobRequisitions":[]}`
	case strings.Contains(raw, "job-requisitions/5001"):
		body = adpMyJobsDetailBody("5001", "<p>Readable role</p>")
	case strings.Contains(raw, "job-requisitions?"):
		body = adpMyJobsListBody(2, "5001", "5002")
	}
	return json.Unmarshal([]byte(body), dst)
}

func TestADPMyJobsPartialDetailFailureKeepsListedIdentity(t *testing.T) {
	for _, status := range []int{503, 429, 200, 404, 410} {
		jobs, err := NewADPMyJobs(adpMyJobsDetailFailure(status)).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "acme", Company: "Acme"}, func(string) bool { return false })
		if err != nil {
			t.Fatal(err)
		}
		byID := byExternalID(jobs)
		if byID["5001"].Description == "" || byID["5001"].Unreadable {
			t.Fatalf("readable role lost: %+v", jobs)
		}
		failed, present := byID["5002"]
		if status == 404 || status == 410 {
			if present {
				t.Fatalf("status %d kept removed posting: %+v", status, failed)
			}
			continue
		}
		if !present || !failed.Unreadable || failed.URL != "https://myjobs.adp.com/acme/cx/job/5002" || failed.Company != "Acme" {
			t.Fatalf("status %d lost unreadable identity: %+v", status, jobs)
		}
	}
}
