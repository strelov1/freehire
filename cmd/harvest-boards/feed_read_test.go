package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type feedFixtureClient struct {
	httpClient
	pages map[string]string
}

func (c feedFixtureClient) GetXML(_ context.Context, u string, v any) error {
	body, ok := c.pages[u]
	if !ok {
		return errors.New("unexpected request: " + u)
	}
	return xml.Unmarshal([]byte(body), v)
}

func TestReadScalisAuditGroupsLocationsByCanonicalURL(t *testing.T) {
	const endpoint = "https://acme.scalis.ai/api/job-listings/feed"
	const id = "a1b2ba0f-53d3-4b43-99dd-6ed80e4834f5"
	row := func(ref, city string) string {
		return `<job><jobinformation>
<reference>` + ref + `</reference><title>Engineer</title>
<job_description><![CDATA[<p>Build &amp; ship systems</p>]]></job_description>
<job_location_name>` + city + `</job_location_name><country>US</country><remote>Remote</remote>
</jobinformation><contact_information>
<vacancy_url>https://acme.scalis.ai/job/` + id + `?utm_source=organic</vacancy_url>
</contact_information></job>`
	}
	c := feedFixtureClient{pages: map[string]string{
		endpoint: `<jobs>` + row(id, "Boston") + row(id+"-location", "Austin") + `</jobs>`,
	}}
	got, err := readAuditFeed(context.Background(), c, "scalis", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got.Rows != 2 || len(got.Jobs) != 1 {
		t.Fatalf("rows/jobs = %d/%d", got.Rows, len(got.Jobs))
	}
	j, ok := got.Jobs[id]
	if !ok || j.URL != "https://acme.scalis.ai/job/"+id || j.Body != "Build & ship systems" {
		t.Fatalf("job = %+v", j)
	}
	if !j.Fields["location"] || !j.Fields["country"] || !j.Fields["work_mode"] || j.Fields["posted_at"] {
		t.Fatalf("fields = %v", j.Fields)
	}
}

func TestReadAuditRejectsWrongShapeAndIdentity(t *testing.T) {
	for _, tc := range []struct{ name, provider, board, endpoint, body string }{
		{
			name: "html masquerading as xml", provider: "scalis", board: "acme",
			endpoint: "https://acme.scalis.ai/api/job-listings/feed",
			body:     `<html><body>Challenge</body></html>`,
		},
		{
			name: "missing channel", provider: "teamtailor", board: "jobs.acme.com",
			endpoint: "https://jobs.acme.com/jobs.rss?offset=0&per_page=500", body: `<rss/>`,
		},
		{
			name: "missing id", provider: "scalis", board: "acme",
			endpoint: "https://acme.scalis.ai/api/job-listings/feed",
			body:     `<jobs><job><jobinformation><title>Engineer</title></jobinformation></job></jobs>`,
		},
		{
			name: "foreign board", provider: "scalis", board: "acme",
			endpoint: "https://acme.scalis.ai/api/job-listings/feed",
			body: `<jobs><job><jobinformation><title>Engineer</title></jobinformation><contact_information>
<vacancy_url>https://other.scalis.ai/job/a1b2ba0f-53d3-4b43-99dd-6ed80e4834f5</vacancy_url>
</contact_information></job></jobs>`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := feedFixtureClient{pages: map[string]string{tc.endpoint: tc.body}}
			_, err := readAuditFeed(context.Background(), client, tc.provider, tc.board)
			if err == nil {
				t.Fatal("accepted invalid feed")
			}
		})
	}
}

func TestReadTeamtailorAuditPaginatesAndRejectsPartialLists(t *testing.T) {
	const first = "https://jobs.acme.com/jobs.rss?offset=0&per_page=500"
	const next = "https://jobs.acme.com/jobs.rss?offset=500&per_page=500"
	var items strings.Builder
	for id := 1; id <= 500; id++ {
		fmt.Fprintf(&items, `<item><title>Engineer</title>
<link>https://jobs.acme.com/jobs/%d-engineer</link><guid>not-the-native-id</guid>
<description><![CDATA[<p>Full body</p>]]></description></item>`, id)
	}
	page := `<rss><channel>` + items.String() + `</channel></rss>`
	for _, tc := range []struct {
		name, second string
		wantErr      bool
	}{
		{name: "complete", second: `<rss><channel/></rss>`},
		{name: "failed second page", wantErr: true},
		{name: "repeated page", second: page, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := map[string]string{first: page}
			if tc.second != "" {
				pages[next] = tc.second
			}
			got, err := readAuditFeed(context.Background(), feedFixtureClient{pages: pages}, "teamtailor", "jobs.acme.com")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v", err)
			}
			if !tc.wantErr && (len(got.Jobs) != 500 || got.Jobs["500"].Body != "Full body") {
				t.Fatalf("jobs = %d", len(got.Jobs))
			}
		})
	}
}

func TestReadAuditEmptyFeedIsValid(t *testing.T) {
	client := feedFixtureClient{pages: map[string]string{
		"https://acme.scalis.ai/api/job-listings/feed": `<jobs/>`,
	}}
	got, err := readAuditFeed(context.Background(), client, "scalis", "acme")
	if err != nil || got.Rows != 0 || len(got.Jobs) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// A Teamtailor site on its own domain links every feed item there, not to the board host
// (ascom.teamtailor.com -> career.ascom.com, measured 2026-11-01 on 11 of 200 boards).
func TestReadTeamtailorAuditAcceptsTheCareerSiteDomain(t *testing.T) {
	client := feedFixtureClient{pages: map[string]string{
		"https://ascom.teamtailor.com/jobs.rss?offset=0&per_page=500": `<rss><channel><item>
<title>Engineer</title><link>https://career.ascom.com/jobs/8607362-engineer</link>
</item></channel></rss>`,
	}}
	got, err := readAuditFeed(context.Background(), client, "teamtailor", "ascom.teamtailor.com")
	if err != nil {
		t.Fatal(err)
	}
	if j, ok := got.Jobs["8607362"]; !ok || j.URL != "https://career.ascom.com/jobs/8607362-engineer" {
		t.Fatalf("jobs = %+v", got.Jobs)
	}
}
