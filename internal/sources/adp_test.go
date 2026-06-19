package sources

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func adpListJSON(total int, items ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `{"meta":{"totalNumber":%d,"startSequence":0},"jobRequisitions":[`, total)
	for i := 0; i < len(items); i += 2 {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"itemID":%q,"requisitionTitle":%q,"postDate":"2026-06-14T19:00:00.000-05:00","requisitionLocations":[{"nameCode":{"shortName":" Remote, EST, US"}}]}`, items[i], items[i+1])
	}
	b.WriteString(`]}`)
	return b.String()
}

func adpDetailJSON(itemID, title, descHTML string) string {
	return fmt.Sprintf(`{"itemID":%q,"requisitionTitle":%q,"postDate":"2026-06-14T19:00:00.000-05:00","requisitionDescription":%q,"requisitionLocations":[{"nameCode":{"shortName":" Remote, EST, US"}}]}`, itemID, title, descHTML)
}

func TestADPFetchListThenDetailAndMaps(t *testing.T) {
	fake := (&routedHTTP{}).
		route("%24skip=0", adpListJSON(1, "9201_1", "Legal Counsel")).
		route("job-requisitions/9201_1?", adpDetailJSON("9201_1", "Legal Counsel", "<p>Do <b>law</b>.</p><script>x</script>"))

	jobs, err := NewADP(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Alludo", Provider: "adp", Board: "thecid:9201289910657_2",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "9201_1" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.Title != "Legal Counsel" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Alludo" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Remote, EST, US" {
		t.Errorf("Location = %q (want trimmed shortName)", j.Location)
	}
	if !j.Remote {
		t.Errorf("Remote = false, want true (location says Remote)")
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "<b>law</b>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-14T19:00-05:00 = 2026-06-15T00:00Z", j.PostedAt)
	}
}

func TestADPPaginatesUntilTotal(t *testing.T) {
	// total=2 across two pages of 1 (page size is 50 in prod, but the stop is by count).
	fake := (&routedHTTP{}).
		route("%24skip=0", `{"meta":{"totalNumber":2,"startSequence":0},"jobRequisitions":[{"itemID":"a","requisitionTitle":"A"}]}`).
		route("%24skip=50", `{"meta":{"totalNumber":2,"startSequence":50},"jobRequisitions":[{"itemID":"b","requisitionTitle":"B"}]}`).
		route("%24skip=100", `{"meta":{"totalNumber":2,"startSequence":100},"jobRequisitions":[]}`).
		route("job-requisitions/a?", adpDetailJSON("a", "A", "<p>x</p>")).
		route("job-requisitions/b?", adpDetailJSON("b", "B", "<p>y</p>"))

	jobs, err := NewADP(fake).Fetch(context.Background(), CompanyEntry{Company: "Alludo", Board: "c:cc"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
}

func TestADPBoardMustBeCidCcid(t *testing.T) {
	_, err := NewADP(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "missingcolon"})
	if err == nil {
		t.Fatal("want error for board without cid:ccId")
	}
}

func TestADPProvider(t *testing.T) {
	if got := NewADP(nil).Provider(); got != "adp" {
		t.Errorf("Provider() = %q, want adp", got)
	}
}

func TestADPRegisteredInAll(t *testing.T) {
	if s, ok := All(nil)["adp"]; !ok || s.Provider() != "adp" {
		t.Fatal("All() missing provider adp")
	}
}
