package sources

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// These bounded fixtures retain the two public descriptions stored on 2026-11-14,
// checked against the live /32 and /40 pages on 2026-11-16.
func halykJuniorBodies(t *testing.T) (string, string) {
	t.Helper()
	read := func(name string) string {
		t.Helper()
		body, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	return read("halyk_junior_32.html"), read("halyk_junior_40.html")
}

func halykJuniorCard(path, branch string) string {
	return fmt.Sprintf(`<a href="/about/career/vacancies-inner/%s">
		<div class="bg-secondary text-white">%s</div>
		<div class="flex-x"><div>Алматы</div></div>
		<div class="text-lg font-bold">Junior Developer</div></a>`, path, branch)
}

func TestHalykSharedTitleKeepsBothLiveRoles(t *testing.T) {
	crm, services := halykJuniorBodies(t)
	const crmID = "12106e354bfe9a65:ddb8dc2bcc268b18"
	const servicesID = "12106e354bfe9a65:1a51477fe215da03"
	for _, swap := range []bool{false, true} {
		name := "original paths"
		if swap {
			name = "reshuffled paths and listing order"
		}
		t.Run(name, func(t *testing.T) {
			crmPath, servicesPath := "32", "40"
			if swap {
				crmPath, servicesPath = "40", "32"
			}
			cards := halykJuniorCard(crmPath, "Головной банк") + halykJuniorCard(servicesPath, "")
			if swap {
				cards = halykJuniorCard(servicesPath, "") + halykJuniorCard(crmPath, "Головной банк")
			}
			client := (&routedHTTP{}).
				route("vacancies-inner/"+crmPath, crm).
				route("vacancies-inner/"+servicesPath, services).
				route(halykListURL, `<p>Всего: 2 вакансий</p>`+cards)
			jobs, err := NewHalyk(client).Fetch(context.Background(), CompanyEntry{Company: "Halyk Bank"})
			if err != nil {
				t.Fatal(err)
			}
			if len(jobs) != 2 {
				t.Fatalf("got %d jobs, want both roles", len(jobs))
			}
			byID := map[string]Job{}
			for _, job := range jobs {
				byID[job.ExternalID] = job
			}
			if !strings.Contains(byID[crmID].Description, "Creatio") ||
				!strings.Contains(byID[servicesID].Description, "ReportServer") {
				t.Fatalf("roles collapsed or identities changed: %+v", byID)
			}
			if !strings.HasSuffix(byID[crmID].URL, "/"+crmPath) ||
				!strings.HasSuffix(byID[servicesID].URL, "/"+servicesPath) {
				t.Fatalf("urls did not follow reshuffled paths: %+v", byID)
			}
		})
	}
}

func TestHalykSharedTitleRejectsUnreadableIdentity(t *testing.T) {
	crm, _ := halykJuniorBodies(t)
	for _, missing := range []bool{false, true} {
		name := "empty body"
		if missing {
			name = "failed request"
		}
		t.Run(name, func(t *testing.T) {
			client := (&routedHTTP{}).route("vacancies-inner/32", crm)
			if missing {
				client.routeErr("vacancies-inner/40", &StatusError{Code: 503})
			} else {
				client.route("vacancies-inner/40", `<div class="text-3xl font-bold">Junior Developer</div>`)
			}
			client.route(halykListURL, `<p>Всего: 2 вакансий</p>`+
				halykJuniorCard("32", "Головной банк")+halykJuniorCard("40", ""))
			jobs, err := NewHalyk(client).Fetch(context.Background(), CompanyEntry{})
			if err == nil || !strings.Contains(err.Error(), "unreadable") || len(jobs) != 0 {
				t.Fatalf("unresolved identity must fail without a partial listing: jobs=%+v err=%v", jobs, err)
			}
		})
	}
}

func TestHalykSharedTitleIgnoresFormatting(t *testing.T) {
	crm, services := halykJuniorBodies(t)
	crm = strings.ReplaceAll(strings.ReplaceAll(crm, "\n", "\t"), "Creatio", "CREATIO")
	client := (&routedHTTP{}).
		route("vacancies-inner/32", crm).
		route("vacancies-inner/40", strings.ReplaceAll(services, "<br/>", "\n")).
		route(halykListURL, `<p>Всего: 2 вакансий</p>`+
			halykJuniorCard("32", "Головной банк")+halykJuniorCard("40", ""))
	jobs, err := NewHalyk(client).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[0].ExternalID != "12106e354bfe9a65:ddb8dc2bcc268b18" ||
		jobs[1].ExternalID != "12106e354bfe9a65:1a51477fe215da03" {
		t.Fatalf("formatting changed identities: %+v", jobs)
	}
}

func TestHalykResolvedCollisionReturnsLegacyIdentity(t *testing.T) {
	crm, _ := halykJuniorBodies(t)
	client := (&routedHTTP{}).route("vacancies-inner/7", crm).
		route(halykListURL, `<p>Всего: 1 вакансий</p>`+halykJuniorCard("7", "Головной банк"))
	jobs, err := NewHalyk(client).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "12106e354bfe9a65" {
		t.Fatalf("a singleton must retain the legacy identity: %+v", jobs)
	}
}
