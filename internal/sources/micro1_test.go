package sources

import (
	"context"
	"encoding/xml"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/skilltag"
)

// micro1Fake serves the sitemap fixture for GetXML and one parsed post page for any GetHTML
// URL, satisfying the micro1HTTP transport the adapter needs.
type micro1Fake struct {
	page       *html.Node
	sitemapXML string
}

func (f micro1Fake) GetHTML(_ context.Context, _ string) (*html.Node, error) { return f.page, nil }

func (f micro1Fake) GetXML(_ context.Context, _ string, v any) error {
	return xml.Unmarshal([]byte(f.sitemapXML), v)
}

func micro1Fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func micro1ParseFixture(t *testing.T, name string) *html.Node {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	root, err := html.Parse(strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return root
}

func micro1FlightFromFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	root, err := html.Parse(strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	flight, err := decodeNextFlight(root)
	if err != nil {
		t.Fatalf("decodeNextFlight %s: %v", name, err)
	}
	return flight
}

// TestMicro1ExtractData pins that the job payload is read out of the RSC flight's single
// "data" object with the fields the adapter maps.
func TestMicro1ExtractData(t *testing.T) {
	flight := micro1FlightFromFixture(t, "micro1_post.html")
	d, ok := extractMicro1Data(flight)
	if !ok {
		t.Fatal("extractMicro1Data: data object not found in fixture flight")
	}
	if got, want := d.ClientJobID, "dcb37b06-8e05-434d-ac22-372a4c04cefc"; got != want {
		t.Errorf("ClientJobID = %q, want %q", got, want)
	}
	if got, want := d.JobRoleName, "Open Source Contributor"; got != want {
		t.Errorf("JobRoleName = %q, want %q", got, want)
	}
	if got, want := d.JobStatus, "open"; got != want {
		t.Errorf("JobStatus = %q, want %q", got, want)
	}
	if got, want := d.CreateDatetime, "2026-07-16 18:59:08"; got != want {
		t.Errorf("CreateDatetime = %q, want %q", got, want)
	}
	if !strings.HasPrefix(d.JobDescription, "$") {
		t.Errorf("JobDescription = %q, want a $-reference into the flight", d.JobDescription)
	}
	wantSkills := []string{"Python3", "JAVA", "Rust", "Basics C++", "Typescript", "GoLang"}
	if strings.Join(d.RequiredSkills, ",") != strings.Join(wantSkills, ",") {
		t.Errorf("RequiredSkills = %v, want %v", d.RequiredSkills, wantSkills)
	}
}

// TestMicro1ExtractDataIgnoresDecoy pins that a stray earlier "data": (unrelated component
// state) does not shadow the job payload: the anchor targets the object that holds
// client_job_id, not the first "data": in the stream.
func TestMicro1ExtractDataIgnoresDecoy(t *testing.T) {
	flight := `x:{"data":{"foo":"bar"}}` +
		`y:{"data":{"client_job_id":"abc-123","job_role_name":"Engineer"}}`
	d, ok := extractMicro1Data(flight)
	if !ok {
		t.Fatal("extractMicro1Data: expected the client_job_id-bearing object")
	}
	if d.ClientJobID != "abc-123" || d.JobRoleName != "Engineer" {
		t.Errorf("matched wrong data object: %+v", d)
	}
}

func TestMicro1PostID(t *testing.T) {
	cases := map[string]string{
		"https://jobs.micro1.ai/post/dcb37b06-8e05-434d-ac22-372a4c04cefc":  "dcb37b06-8e05-434d-ac22-372a4c04cefc",
		"https://jobs.micro1.ai/post/416f4a44-95ff-455f-ab01-fa318a30dda8/": "416f4a44-95ff-455f-ab01-fa318a30dda8",
		"https://jobs.micro1.ai":                                                 "", // board root
		"https://jobs.micro1.ai/":                                                "",
		"https://jobs.micro1.ai/post/not-a-uuid":                                 "",
		"https://jobs.micro1.ai/post/dcb37b06-8e05-434d-ac22-372a4c04cefc/apply": "", // deeper path
		"https://www.micro1.ai/jobs":                                             "", // wrong host
	}
	for u, want := range cases {
		if got := micro1PostID(u); got != want {
			t.Errorf("micro1PostID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestMicro1Provider(t *testing.T) {
	if got := NewMicro1(nil).Provider(); got != "micro1" {
		t.Errorf("Provider() = %q, want %q", got, "micro1")
	}
}

func TestMicro1IsBoardless(t *testing.T) {
	if _, ok := NewMicro1(nil).(boardless); !ok {
		t.Error("micro1 adapter must be boardless (single-company, fixed host)")
	}
}

func TestMicro1RegisteredInAllAndBoardless(t *testing.T) {
	s, ok := All(nil)["micro1"]
	if !ok {
		t.Fatal(`All(nil)["micro1"] missing`)
	}
	if _, isBoardless := s.(boardless); !isBoardless {
		t.Error("micro1 should be boardless (single company, no board id)")
	}
}

func TestMicro1Detail(t *testing.T) {
	m := micro1{http: micro1Fake{page: micro1ParseFixture(t, "micro1_post.html")}}
	url := "https://jobs.micro1.ai/post/dcb37b06-8e05-434d-ac22-372a4c04cefc"
	job, ok := m.detail(context.Background(), CompanyEntry{Company: "micro1"}, url)
	if !ok {
		t.Fatal("detail: expected ok=true for a valid post page")
	}
	if got, want := job.ExternalID, "dcb37b06-8e05-434d-ac22-372a4c04cefc"; got != want {
		t.Errorf("ExternalID = %q, want %q (must equal client_job_id)", got, want)
	}
	if got, want := job.Title, "Open Source Contributor"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
	if got, want := job.Company, "micro1"; got != want {
		t.Errorf("Company = %q, want %q", got, want)
	}
	if job.URL != url {
		t.Errorf("URL = %q, want %q", job.URL, url)
	}
	// Description is a "$N" flight reference resolved to sanitized description HTML (the policy
	// keeps structural tags like <p>). It must carry the posting's prose and must NOT still be
	// the raw "$N" reference or carry the RSC "T<hexlen>," row marker.
	if !strings.Contains(job.Description, "Open Source Contributor") {
		t.Errorf("Description missing posting prose: %.80q…", job.Description)
	}
	if strings.HasPrefix(job.Description, "$") || strings.Contains(job.Description, "T1339,") {
		t.Errorf("Description still carries an unresolved ref/row marker: %.40q", job.Description)
	}
	// Skills are canonicalized through the skilltag seam, not emitted raw: the adapter must
	// return skilltag.Parse of the required_skills list (so "GoLang"→Go, "Typescript"→TypeScript,
	// etc.), and never leak a raw non-canonical token like "GoLang".
	wantSkills := skilltag.Parse("Python3 JAVA Rust Basics C++ Typescript GoLang")
	if len(wantSkills) == 0 {
		t.Fatal("precondition: skilltag dictionary yielded no skills for the sample — test is vacuous")
	}
	if strings.Join(job.Skills, "|") != strings.Join(wantSkills, "|") {
		t.Errorf("Skills = %v, want %v (canonicalized via skilltag)", job.Skills, wantSkills)
	}
	for _, s := range job.Skills {
		if s == "GoLang" || s == "Typescript" || s == "JAVA" {
			t.Errorf("raw non-canonical skill leaked: %q", s)
		}
	}
	if job.PostedAt == nil || job.PostedAt.Format("2006-01-02") != "2026-07-16" {
		t.Errorf("PostedAt = %v, want 2026-07-16", job.PostedAt)
	}
	// The fixture posting has location_type null → no structured work-mode claim.
	if job.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty (location_type is null)", job.WorkMode)
	}
}

// TestMicro1DetailNoID pins that a page whose URL is not a canonical post is skipped so it
// cannot collide on the (source, external_id) dedup key.
func TestMicro1DetailNoID(t *testing.T) {
	m := micro1{http: micro1Fake{page: micro1ParseFixture(t, "micro1_post.html")}}
	if _, ok := m.detail(context.Background(), CompanyEntry{Company: "micro1"}, "https://jobs.micro1.ai"); ok {
		t.Error("detail: expected ok=false for the board root URL")
	}
}

// TestMicro1Fetch pins that Fetch enumerates the sitemap, keeps only the /post/<uuid> URLs
// (excluding the board root), and returns one Job per posting.
func TestMicro1Fetch(t *testing.T) {
	fake := micro1Fake{
		page:       micro1ParseFixture(t, "micro1_post.html"),
		sitemapXML: micro1Fixture(t, "micro1_sitemap.xml"),
	}
	jobs, err := NewMicro1(fake).Fetch(context.Background(), CompanyEntry{Company: "micro1"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The sitemap fixture has the board root plus 3 posts; the root is skipped.
	if got, want := len(jobs), 3; got != want {
		t.Fatalf("Fetch returned %d jobs, want %d (board root must be excluded)", got, want)
	}
	for _, j := range jobs {
		if j.Title != "Open Source Contributor" || j.Company != "micro1" {
			t.Errorf("job = %+v, want fixture posting mapped under company micro1", j)
		}
	}
}
