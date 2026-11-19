package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// Shapes of the live listing (profile-api) and CMS (kontent-proxy) answers, trimmed.
const crossoverList = `{"totalSize":4,"done":true,"records":[
{"ProductCode":"5865","Name":"Executive Recruiter","Brand__r":{"Name":"Alpha"},"DisplayUrl":"https://alpha.crossover.com/jobs/5865/executive-recruiter",
 "Job_Type__c":"full-time","Work_Model__c":"Remote","Work_Country__c":"United States","Yearly_Rate__c":200000,"Work_Locations__r":null},
{"ProductCode":"5841","Name":"Ecosystem Director","Brand__r":{"Name":"Alpha"},"DisplayUrl":"https://alpha.crossover.com/jobs/5841/ecosystem-director",
 "Job_Type__c":"part-time","Work_Model__c":"On-site","Work_Country__c":"United States","Yearly_Rate__c":400000,
 "Work_Locations__r":{"records":[{"Location__r":{"Name":"Dallas, United States"}},{"Location__r":{"Name":"Boston, United States"}}]}},
{"ProductCode":"5781","Name":"Gauntlet Fellow","Brand__r":{"Name":"Gauntlet AI"},"DisplayUrl":"https://www.crossover.com/jobs/5781/gauntlet-ai/gauntlet-fellow",
 "Job_Type__c":"full-time","Work_Model__c":"Remote","Yearly_Rate__c":0},
{"ProductCode":"9","Name":"No brand","DisplayUrl":"https://www.crossover.com/jobs/9/x"}
]}`

const crossoverBodiesJSON = `{"items":[
{"system":{"codename":"pipeline_5865"},"elements":{"hook":{"value":"<p>Find leaders.</p>"},"what_you_will_be_doing":{"value":"<ul><li>Search</li></ul>"},
 "responsibilities":{"value":"<p><br></p>"},"requirements":{"value":"<ul><li>Experience</li></ul>"},"nice_to_have":{"value":""}}},
{"system":{"codename":"pipeline_5841"},"elements":{"hook":{"value":"<p>Lead.</p>"}}}
]}`

func TestCrossoverRegisteredBoardlessAggregator(t *testing.T) {
	s, ok := All(nil)["crossover"]
	if !ok {
		t.Fatal("All() should register provider crossover")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("crossover should be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("crossover should be an aggregator")
	}
	if !slices.Contains(FilterableProviders(), "crossover") {
		t.Error("FilterableProviders() should include crossover")
	}
}

func TestCrossoverFetchMapsPostings(t *testing.T) {
	fake := (&routedHTTP{}).route("kontent-proxy", crossoverBodiesJSON).route("profile-api", crossoverList)
	jobs, err := NewCrossover(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The brandless row is dropped; the posting with no CMS item stays as an unreadable stub.
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "5865" || j.Company != "Alpha" || j.Location != "United States" || j.WorkMode != "remote" || !j.Remote || j.EmploymentType != "full_time" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 200000 || j.SalaryCurrency != "USD" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %+v", j)
	}
	for _, want := range []string{"Find leaders.", "What you will be doing", "Requirements"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q: %q", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "Responsibilities") || strings.Contains(j.Description, "Nice to have") {
		t.Errorf("empty sections should be dropped: %q", j.Description)
	}
	onsite := jobs[1]
	if onsite.Location != "Dallas, United States; Boston, United States" || onsite.WorkMode != "onsite" || onsite.Remote || onsite.EmploymentType != "part_time" {
		t.Errorf("on-site mapping: %+v", onsite)
	}
	if !jobs[2].Unreadable || jobs[2].ExternalID != "5781" {
		t.Errorf("body-less posting should be an unreadable stub: %+v", jobs[2])
	}
}

func TestCrossoverLocationFallback(t *testing.T) {
	if got := (crossoverPipeline{Model: "Remote"}).location(); got != "Remote" {
		t.Errorf("remote with no place = %q, want Remote", got)
	}
	if got := (crossoverPipeline{Model: "On-site"}).location(); got != "" {
		t.Errorf("on-site with no place = %q, want empty", got)
	}
}

func TestCrossoverFetchFailsOnEmptyListOrBodies(t *testing.T) {
	empty := (&routedHTTP{}).route("profile-api", `{"records":[]}`)
	if _, err := NewCrossover(empty).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("an empty pipeline list should fail the crawl")
	}
	noBodies := (&routedHTTP{}).routeErr("kontent-proxy", context.DeadlineExceeded).route("profile-api", crossoverList)
	if _, err := NewCrossover(noBodies).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a failed CMS read should fail the crawl")
	}
	emptyBodies := (&routedHTTP{}).route("kontent-proxy", `{"items":[]}`).route("profile-api", crossoverList)
	if _, err := NewCrossover(emptyBodies).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a CMS answer with no readable body should fail the crawl")
	}
}
