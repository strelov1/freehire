package sources

import (
	"context"
	"strings"
	"testing"
)

const paycorBoard = "8a7883c664f8df9e0165119e33353606"

// paycorListingHTML is a CareerHome listing: absolute anchors to the board's postings, the
// same posting linked twice under different tracking parameters, the board's id-less "submit
// your resume" link, and a posting belonging to a different clientId (a themed listing
// renders inside the employer's own website).
const paycorListingHTML = `<html><body>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=8a7883c664f8df9e0165119e33353606&amp;id=8a78879e9f4435c7019f6b4f5c3a548a&amp;source=&amp;lang=en">Line Cook</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=8a7883c664f8df9e0165119e33353606&amp;id=8a78879e9f4435c7019f6b4f5c3a548a">Apply</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=8a7883c664f8df9e0165119e33353606">Submit Resume</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=4028f88b2456f3b601247ccb93d40fa2&amp;id=8a7883ac9f8b941b019fc81def5d42e0">Role at another employer</a>
<a href="https://example.com/about">About us</a>
</body></html>`

// paycorDetailHTML is a JobIntroduction page: labelled cells whose label is a leading <b>,
// the location split across a label cell and a value cell, and a description carrying a
// <script> that sanitizeHTML must strip.
const paycorDetailHTML = `<html><body><div id="gnewtonCareerBody"><table>
<tr><td id="gnewtonJobPosition">
	<b>Position:</b>&nbsp;
	Line Cook, <b>Grill</b>
</td></tr>
<tr><td><table><tr>
	<td id="gnewtonJobLocation"><b>Location:</b>&nbsp;</td>
	<td id="gnewtonJobLocationInfo">
		Remote - Dublin, Ireland<br/>
	</td>
</tr></table></td></tr>
<tr><td id="gnewtonJobID"><b>Job Id:</b>&nbsp; 2414</td></tr>
<tr><td id="gnewtonJobDescriptionText">
	<div><p>Cook the food.</p><script>alert(1)</script></div>
</td></tr>
</table></div></body></html>`

// paycorInactiveHTML is what a filled, withdrawn or never-existing posting answers: HTTP 200
// with a notice in place of every field cell.
const paycorInactiveHTML = `<html><body>
<div id="gnewtonConfirmation">Sorry, this job is no longer active.</div>
</body></html>`

func TestPaycorProvider(t *testing.T) {
	if got := NewPaycor(nil).Provider(); got != "paycor" {
		t.Errorf("Provider() = %q, want %q", got, "paycor")
	}
}

func TestPaycorPostingID(t *testing.T) {
	cases := []struct {
		href string
		want string
	}{
		{"https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=" + paycorBoard + "&id=abc123", "abc123"},
		// The client id is hex, and the portal serves the same board in either case.
		{"https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=" + strings.ToUpper(paycorBoard) + "&id=abc123", "abc123"},
		// The board's own "submit your resume" link: same action, no posting.
		{"https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=" + paycorBoard, ""},
		// A posting on someone else's board, linked by the employer's site chrome.
		{"https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=4028f88b2456f3b601247ccb93d40fa2&id=abc123", ""},
		{"https://recruitingbypaycor.com/career/CareerHome.action?clientId=" + paycorBoard, ""},
		{"/about", ""},
	}
	for _, c := range cases {
		if got := paycorPostingID(c.href, paycorBoard); got != c.want {
			t.Errorf("paycorPostingID(%q) = %q, want %q", c.href, got, c.want)
		}
	}
}

func TestPaycorFetchListingThenDetailAndMaps(t *testing.T) {
	fake := (&routedHTTP{}).
		route("JobIntroduction.action", paycorDetailHTML).
		route("CareerHome.action", paycorListingHTML)

	jobs, err := NewPaycor(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Afiniti", Provider: "paycor", Board: paycorBoard,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the two anchors de-duped, the foreign board skipped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "8a78879e9f4435c7019f6b4f5c3a548a" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	// The listing's tracking parameters are dropped: the stored URL is the same string on
	// every crawl whatever the listing appended to its href.
	want := "https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=" + paycorBoard +
		"&id=8a78879e9f4435c7019f6b4f5c3a548a"
	if j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	// The leading "<b>Position:</b>" is the cell's label and goes; the "<b>Grill</b>" inside
	// the value is the employer's own emphasis and its words stay.
	if j.Title != "Line Cook, Grill" {
		t.Errorf("Title = %q, want the position cell without its label", j.Title)
	}
	if j.Company != "Afiniti" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Remote - Dublin, Ireland" {
		t.Errorf("Location = %q", j.Location)
	}
	if !j.Remote {
		t.Error("Remote = false, want true for a remote location")
	}
	if j.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil — Paycor states no date", j.PostedAt)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Cook the food") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
}

func TestPaycorSkipsInactivePosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route("JobIntroduction.action", paycorInactiveHTML).
		route("CareerHome.action", paycorListingHTML)

	jobs, err := NewPaycor(fake).Fetch(context.Background(), CompanyEntry{Company: "Afiniti", Board: paycorBoard})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want none — the page answers 200 with no field cells", len(jobs))
	}
}

func TestPaycorFailedDetailDropsOnlyThatPosting(t *testing.T) {
	listing := `<html><body>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=` + paycorBoard + `&amp;id=kept">kept</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=` + paycorBoard + `&amp;id=dropped">dropped</a>
</body></html>`
	// No route for the second posting → GetHTML errors → that posting drops, the board does not.
	fake := (&routedHTTP{}).
		route("id=kept", paycorDetailHTML).
		route("CareerHome.action", listing)

	jobs, err := NewPaycor(fake).Fetch(context.Background(), CompanyEntry{Company: "Afiniti", Board: paycorBoard})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "kept" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestPaycorListingFailureFailsTheBoard(t *testing.T) {
	if _, err := NewPaycor(&routedHTTP{}).Fetch(context.Background(),
		CompanyEntry{Company: "Afiniti", Board: paycorBoard}); err == nil {
		t.Fatal("Fetch: want an error when the listing cannot be read")
	}
}
