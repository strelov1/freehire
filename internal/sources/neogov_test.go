package sources

import "testing"

// Fixture mirrors the real NEOGOV listing fragment (schooljobs/governmentjobs share it):
// li.list-item[data-job-id] cards, each with an item-details-link, a list-meta whose first
// <li> is the location, and a list-entry description snippet.
const neogovListingFixture = `
<ul class="list-items">
  <li class="list-item" data-job-id="5371857">
    <h3 class="job-item-link-container">
      <a class="item-details-link" data-department-name="FT--Student Services"
         href="/careers/cochisecollege/jobs/5371857/ft-academic-career-advisor-svc">FT Academic/Career Advisor SVC</a>
    </h3>
    <ul class="list-meta">
      <li>Sierra Vista Campus, AZ</li>
      <li>Full-time <span>-</span> $50,585.60 Annually</li>
    </ul>
    <div class="list-entry">Position Summary: provide student-centered academic advising.</div>
    <div class="list-published"><span class="list-entry-starts"><span>Posted 3 weeks ago</span></span></div>
  </li>
  <li class="list-item" data-job-id="5379875">
    <h3><a class="item-details-link" href="/careers/cochisecollege/jobs/5379875/ft-accountant">FT Accountant</a></h3>
    <ul class="list-meta"><li>Douglas Campus, AZ</li></ul>
    <div class="list-entry">Maintain the college's financial records.</div>
  </li>
  <li class="list-item">No job id — skipped.</li>
</ul>`

func TestNeogovParseListing(t *testing.T) {
	jobs, err := neogovParseListing(neogovListingFixture, "schooljobs.com", "cochisecollege")
	if err != nil {
		t.Fatalf("neogovParseListing: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "5371857" {
		t.Errorf("ExternalID = %q, want 5371857", j.ExternalID)
	}
	if j.Title != "FT Academic/Career Advisor SVC" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://www.schooljobs.com/careers/cochisecollege/jobs/5371857/ft-academic-career-advisor-svc" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Sierra Vista Campus, AZ" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.Description != "Position Summary: provide student-centered academic advising." {
		t.Errorf("Description = %q", j.Description)
	}
	if jobs[1].ExternalID != "5379875" || jobs[1].Location != "Douglas Campus, AZ" {
		t.Errorf("job1 = %q/%q", jobs[1].ExternalID, jobs[1].Location)
	}
}

func TestNeogovTotal(t *testing.T) {
	if n := neogovTotal(`<span id="job-postings-number">20</span>`); n != 20 {
		t.Errorf("neogovTotal = %d, want 20", n)
	}
	if n := neogovTotal(`<div>no count here</div>`); n != 0 {
		t.Errorf("neogovTotal(absent) = %d, want 0", n)
	}
}
