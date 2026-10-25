package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// meincheckinFixture reads pages captured from mein-check-in.de on 2026-10-25 with their inline
// <style>/<script> bodies and base64 images emptied: two tenants' start pages (ksk-ratzeburg,
// 19 menu entries; sv-informatik, 16; the vr tenant 85095004), three position pages, and the 404 an unknown tenant gets.
func meincheckinFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/meincheckin/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// meincheckinFake serves pages by exact URL. A start page is asked for on www and answered from
// the tenant's own host, as the platform redirects it.
type meincheckinFake struct {
	t      *testing.T
	starts map[string]struct{ final, page string }
	pages  map[string]string
	errs   map[string]error
	mu     sync.Mutex
	gets   []string
}

func (f *meincheckinFake) record(u string) {
	f.mu.Lock()
	f.gets = append(f.gets, u)
	f.mu.Unlock()
}

func (f *meincheckinFake) GetHTMLResolved(_ context.Context, u string) (*html.Node, string, error) {
	f.record(u)
	s, ok := f.starts[u]
	if !ok {
		return nil, "", &StatusError{Code: 404}
	}
	root, err := html.Parse(strings.NewReader(meincheckinFixture(f.t, s.page)))
	return root, s.final, err
}

func (f *meincheckinFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.record(u)
	if err := f.errs[u]; err != nil {
		return nil, err
	}
	name, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Code: 404}
	}
	return html.Parse(strings.NewReader(meincheckinFixture(f.t, name)))
}

const (
	mciKsk  = "https://sparkasse.mein-check-in.de/ksk-ratzeburg"
	mciSVIT = "https://sparkasse.mein-check-in.de/sv-informatik"
)

func newMeinCheckInFake(t *testing.T) *meincheckinFake {
	return &meincheckinFake{
		t: t,
		starts: map[string]struct{ final, page string }{
			"https://www.mein-check-in.de/ksk-ratzeburg": {mciKsk, "list_ksk-ratzeburg.html"},
			"https://www.mein-check-in.de/sv-informatik": {mciSVIT, "list_sv-informatik.html"},
		},
		pages: map[string]string{
			mciKsk + "/position-60759":   "detail_ksk-ratzeburg_60759.html",
			mciKsk + "/position-156":     "detail_ksk-ratzeburg_156.html",
			mciSVIT + "/position-114970": "detail_sv-informatik_114970.html",
		},
		errs: map[string]error{},
	}
}

func TestMeinCheckInProvider(t *testing.T) {
	s, ok := All(nil)["meincheckin"]
	if !ok || s.Provider() != "meincheckin" {
		t.Fatal("All() must register meincheckin")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("meincheckin must be a HydratingSource: the body costs a page request")
	}
}

func TestMeinCheckInListsTheMenuWithoutOpenApplications(t *testing.T) {
	fake := newMeinCheckInFake(t)
	jobs, err := NewMeinCheckIn(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Provider: "meincheckin", Board: "ksk-ratzeburg", Company: "Kreissparkasse Herzogtum Lauenburg"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// 19 menu entries, one of them the open application.
	if len(jobs) != 18 || len(fake.gets) != 1 {
		t.Fatalf("jobs/requests = %d/%d, want 18 refreshes from the one start page", len(jobs), len(fake.gets))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || !strings.HasPrefix(j.ExternalID, "ksk-ratzeburg/") || !strings.HasPrefix(j.URL, mciKsk+"/position-") {
			t.Errorf("job = %+v, want a tenant-qualified SeenRefresh on the tenant's host", j)
		}
		if strings.Contains(j.Title, "Initiativbewerbung") {
			t.Errorf("the open application must not be listed: %q", j.Title)
		}
	}
}

func TestMeinCheckInMapsAPosition(t *testing.T) {
	fake := newMeinCheckInFake(t)
	jobs, err := NewMeinCheckIn(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Provider: "meincheckin", Board: "sv-informatik", Company: "SV Informatik"},
		func(id string) bool { return id != "sv-informatik/114970" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var j Job
	for _, x := range jobs {
		if x.ExternalID == "sv-informatik/114970" {
			j = x
		}
	}
	if j.SeenRefresh || j.Unreadable {
		t.Fatalf("114970 = %+v, want a hydrated posting", j)
	}
	if j.Title != "Anwendungsentwickler:in Versicherungstechnik COR (C/C++) (m/w/d)" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "SV Informatik GmbH" || j.Location == "" || len(j.Countries) != 1 || j.Countries[0] != "de" {
		t.Errorf("company/location/countries = %q %q %v", j.Company, j.Location, j.Countries)
	}
	if j.PostedAt == nil || !strings.Contains(j.Description, "C++") || strings.Contains(j.Description, "Jetzt hier bewerben") {
		t.Errorf("posted/description = %v %.120q", j.PostedAt, j.Description)
	}
	if j.EmploymentType != "" {
		t.Errorf("EmploymentType = %q: a vacancy section states no hours", j.EmploymentType)
	}
}

func TestMeinCheckInReadsAnInternshipSection(t *testing.T) {
	fake := newMeinCheckInFake(t)
	jobs, err := NewMeinCheckIn(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Provider: "meincheckin", Board: "ksk-ratzeburg", Company: "Kreissparkasse Herzogtum Lauenburg"},
		func(id string) bool { return id != "ksk-ratzeburg/156" && id != "ksk-ratzeburg/60759" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	got := byExternalID(jobs)
	if got["ksk-ratzeburg/156"].EmploymentType != "internship" {
		t.Errorf("156 EmploymentType = %q, want internship for the Praktika section", got["ksk-ratzeburg/156"].EmploymentType)
	}
	if k := got["ksk-ratzeburg/60759"]; k.Company != "Kreissparkasse Herzogtum Lauenburg" || k.Location != "Mölln" {
		t.Errorf("60759 company/location = %q %q", k.Company, k.Location)
	}
}

func TestMeinCheckInClosedAndUnreadablePositions(t *testing.T) {
	fake := newMeinCheckInFake(t)
	fake.errs[mciKsk+"/position-61083"] = &StatusError{Code: 503}
	jobs, err := NewMeinCheckIn(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Provider: "meincheckin", Board: "ksk-ratzeburg", Company: "Kreissparkasse Herzogtum Lauenburg"},
		func(id string) bool { return id != "ksk-ratzeburg/61083" && id != "ksk-ratzeburg/63084" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	got := byExternalID(jobs)
	if !got["ksk-ratzeburg/61083"].Unreadable {
		t.Error("a 503 must be an unreadable marker, not a drop")
	}
	if _, ok := got["ksk-ratzeburg/63084"]; ok {
		t.Error("a 404 is the platform's own answer that the position closed: drop it")
	}
}

func TestMeinCheckInUnknownTenantIsGone(t *testing.T) {
	fake := newMeinCheckInFake(t)
	_, err := NewMeinCheckIn(fake).Fetch(context.Background(),
		CompanyEntry{Provider: "meincheckin", Board: "no-such-tenant"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
	if len(fake.gets) != 2 {
		t.Errorf("requests = %v, want www then vr", fake.gets)
	}
}

// A cooperative bank's tenant is a 404 on www and answers only on vr.
func TestMeinCheckInFallsBackToTheVRHost(t *testing.T) {
	fake := newMeinCheckInFake(t)
	fake.starts["https://vr.mein-check-in.de/85095004"] = struct{ final, page string }{
		"https://vr.mein-check-in.de/85095004", "list_85095004.html"}
	jobs, err := NewMeinCheckIn(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Provider: "meincheckin", Board: "85095004"}, func(string) bool { return true })
	if err != nil || len(fake.gets) < 2 || fake.gets[1] != "https://vr.mein-check-in.de/85095004" {
		t.Fatalf("err = %v, requests = %v, want the vr start page read after the www 404", err, fake.gets)
	}
	if len(jobs) == 0 || !strings.HasPrefix(jobs[0].URL, "https://vr.mein-check-in.de/85095004/position-") {
		t.Fatalf("jobs = %+v, want the vr tenant's positions on its own host", jobs)
	}
}
