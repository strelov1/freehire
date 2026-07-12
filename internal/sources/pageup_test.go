package sources

import "testing"

// A tenant's fragment is homogeneous — Monash renders a <tr> table, Melbourne Water a <li>
// list. Each is tested on its own valid fragment (mixing the two in one document would
// trigger the HTML parser's table foster-parenting and reorder nodes, which never happens in
// a real single-tenant response). Both must yield the same fields off the stable a.job-link.
// The real endpoint returns bare <tr> rows with NO <table> wrapper (the page's JS injects
// them into an existing table), so the parser must supply the table context itself — parsing
// bare rows directly triggers foster-parenting that drops the row structure.
const pageupTableFixture = `
  <tr>
    <td><a class="job-link" href="/513/cw/en/job/694504/senior-audiovisual-design-engineer">Senior Audiovisual Design Engineer</a></td>
    <td><span class="location">Clayton campus</span></td>
    <td>HEW 8</td>
    <td><span class="close-date"><time datetime="2026-08-02T13:55:00Z">2 Aug 2026</time></span></td>
  </tr>
  <tr class="summary"><td colspan="4">Support the University's audiovisual design team.</td></tr>
  <tr><td>No anchor here — ignored.</td></tr>`

const pageupListFixture = `
<ul>
  <li class="jobs-item">
    <a class="job-link" href="/391/cw/en/job/980277/education-and-events-lead">Education and Events Lead</a>
    <span class="jobs-info">
      <span class="label">Work type:</span> <span class="work-type permanent-part-time">Permanent Part Time</span>
      <span class="label">Location:</span> <span class="location">Melbourne - Docklands</span>
    </span>
    <p class="jobs-summary">Lead education and events programs.</p>
  </li>
</ul>`

func TestPageupParseListing(t *testing.T) {
	t.Run("table layout", func(t *testing.T) {
		jobs, err := pageupParseListing(pageupTableFixture, "513")
		if err != nil {
			t.Fatalf("pageupParseListing: %v", err)
		}
		if len(jobs) != 1 {
			t.Fatalf("got %d jobs, want 1: %+v", len(jobs), jobs)
		}
		j := jobs[0]
		if j.ExternalID != "694504" || j.Title != "Senior Audiovisual Design Engineer" {
			t.Errorf("id/title = %q/%q", j.ExternalID, j.Title)
		}
		if j.URL != "https://careers.pageuppeople.com/513/cw/en/job/694504/senior-audiovisual-design-engineer" {
			t.Errorf("URL = %q", j.URL)
		}
		if j.Location != "Clayton campus" {
			t.Errorf("Location = %q", j.Location)
		}
		if j.Description != "Support the University's audiovisual design team." {
			t.Errorf("Description = %q", j.Description)
		}
	})

	t.Run("list layout", func(t *testing.T) {
		jobs, err := pageupParseListing(pageupListFixture, "391")
		if err != nil {
			t.Fatalf("pageupParseListing: %v", err)
		}
		if len(jobs) != 1 {
			t.Fatalf("got %d jobs, want 1: %+v", len(jobs), jobs)
		}
		j := jobs[0]
		if j.ExternalID != "980277" || j.Location != "Melbourne - Docklands" {
			t.Errorf("id/loc = %q/%q", j.ExternalID, j.Location)
		}
		if j.Description != "Lead education and events programs." {
			t.Errorf("Description = %q", j.Description)
		}
	})
}
