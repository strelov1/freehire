package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// google adapts Google's own careers catalogue
// (www.google.com/about/careers/applications/jobs/results), a single-company source with
// no per-tenant board id (boardless). The old public careers JSON API is gone, but the list
// pages are server-rendered: each page inlines its full job payload in an
// AF_initDataCallback({key:'ds:1', data:[…]}) script block, so one paged list crawl
// assembles every Job with no per-posting detail request.
type google struct {
	http HTMLGetter
}

const (
	// googleListURL is the paged careers results page; %d is the 1-based page number.
	googleListURL = "https://www.google.com/about/careers/applications/jobs/results?page=%d"
	// googleJobURL is the public, slug-optional results page for one posting; %s is the id.
	// Google ignores the trailing slug (id-only resolves), so no slug is reproduced.
	googleJobURL = "https://www.google.com/about/careers/applications/jobs/results/%s"
)

// googleDS1Data isolates the JSON array passed to AF_initDataCallback for the ds:1 key:
// `…data:[…], sideChannel: {}…`. The capture is the bracketed array, ended by the
// `, sideChannel` that always follows it. The match is lazy, so it assumes no posting body
// contains the literal `], sideChannel`; if one did, the capture would truncate and the
// page's JSON would fail to decode (an error, never silent bad data) — an accepted edge of
// the brittle embedded-payload seam.
var googleDS1Data = regexp.MustCompile(`(?s)data:(\[.*?\]), sideChannel`)

// NewGoogle builds the Google careers adapter over the given HTML client.
func NewGoogle(c HTMLGetter) Source { return google{http: c} }

func (google) Provider() string { return "google" }

// google is single-company, so its config entries carry no board.
func (google) boardless() {}

func (g google) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; ; page++ {
		root, err := g.http.GetHTML(ctx, fmt.Sprintf(googleListURL, page))
		if err != nil {
			return nil, fmt.Errorf("google: list page %d: %w", page, err)
		}
		records, total, err := extractGoogleDS1(root)
		if err != nil {
			return nil, fmt.Errorf("google: list page %d: %w", page, err)
		}
		if len(records) == 0 {
			break // past the last page: ds:1 carries a null/empty job list
		}
		for _, rec := range records {
			if j, ok := g.toJob(e, rec); ok {
				jobs = append(jobs, j)
			}
		}
		if total > 0 && len(jobs) >= total {
			break
		}
	}
	return jobs, nil
}

// extractGoogleDS1 pulls the embedded ds:1 payload out of a results page and returns the
// job records (data[0]) and the catalogue total (data[3]). A page past the last one carries
// a null job list, which decodes to zero records.
func extractGoogleDS1(root *html.Node) ([]json.RawMessage, int, error) {
	script := findGoogleDS1Script(root)
	if script == "" {
		return nil, 0, fmt.Errorf("ds:1 script not found")
	}
	m := googleDS1Data.FindStringSubmatch(script)
	if m == nil {
		return nil, 0, fmt.Errorf("ds:1 data array not found")
	}
	// data is [records, _, total, pageSize]; records is null past the last page.
	var data []json.RawMessage
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		return nil, 0, fmt.Errorf("decode ds:1 data: %w", err)
	}
	var records []json.RawMessage
	if len(data) > 0 {
		_ = json.Unmarshal(data[0], &records) // null → nil records, not an error
	}
	var total int
	if len(data) > 2 {
		_ = json.Unmarshal(data[2], &total) // [2] = catalogue total ([3] is the page size)
	}
	return records, total, nil
}

// findGoogleDS1Script returns the text of the <script> that invokes AF_initDataCallback for
// the ds:1 key, or "" when no such script is present. The page carries several framework
// scripts that reference the bare string `'ds:1'`, so the match anchors on the exact callback
// argument `key: 'ds:1'` to select the one script that actually carries the data payload.
func findGoogleDS1Script(root *html.Node) string {
	var found string
	walk(root, func(n *html.Node) bool {
		if found != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "script" {
			if t := textContent(n); strings.Contains(t, "key: 'ds:1'") {
				found = t
				return false
			}
		}
		return true
	})
	return found
}

// toJob maps one ds:1 job record (a positional JSON array) to a Job. ok is false when the
// record carries no id, which would collide on the (source, external_id) dedup key.
func (google) toJob(e CompanyEntry, record json.RawMessage) (Job, bool) {
	var rec []json.RawMessage
	if err := json.Unmarshal(record, &rec); err != nil {
		return Job{}, false
	}
	id := googleString(rec, 0) // [0] = numeric job id
	if id == "" {
		return Job{}, false
	}
	// Description assembles about-the-job [10], responsibilities [3], qualifications [4] in
	// reading order; each is a [null, "<html>"] pair.
	desc := googleHTMLField(rec, 10) + googleHTMLField(rec, 3) + googleHTMLField(rec, 4)
	return Job{
		ExternalID:  id,
		URL:         fmt.Sprintf(googleJobURL, id),
		Title:       googleString(rec, 1),                           // [1] = title
		Company:     firstNonEmpty(googleString(rec, 7), e.Company), // [7] = hiring brand
		Location:    googleLocations(rec, 9),                        // [9] = locations array
		Description: sanitizeHTML(desc),
		PostedAt:    parseEpochSeconds(googleEpochSeconds(rec, 12)), // [12] = [seconds, nanos]
	}, true
}

// googleString decodes rec[i] as a string, returning "" when out of range or not a string.
func googleString(rec []json.RawMessage, i int) string {
	if i >= len(rec) {
		return ""
	}
	var s string
	_ = json.Unmarshal(rec[i], &s)
	return s
}

// googleHTMLField decodes a [null, "<html>"] field at rec[i], returning the HTML string (the
// second element) or "" when absent.
func googleHTMLField(rec []json.RawMessage, i int) string {
	if i >= len(rec) {
		return ""
	}
	var pair []json.RawMessage
	if err := json.Unmarshal(rec[i], &pair); err != nil || len(pair) < 2 {
		return ""
	}
	var s string
	_ = json.Unmarshal(pair[1], &s)
	return s
}

// googleLocations joins the display names of the locations array at rec[i]. Each entry is
// ["City, Region, Country", […], …, "CC"]; the first element is the human-readable name.
func googleLocations(rec []json.RawMessage, i int) string {
	if i >= len(rec) {
		return ""
	}
	var locs [][]json.RawMessage
	if err := json.Unmarshal(rec[i], &locs); err != nil {
		return ""
	}
	var names []string
	for _, loc := range locs {
		if len(loc) == 0 {
			continue
		}
		var name string
		if json.Unmarshal(loc[0], &name) == nil && name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, "; ")
}

// googleEpochSeconds reads the Unix seconds from a [seconds, nanos] timestamp at rec[i],
// returning 0 when absent (treated as "no date" by parseEpochSeconds).
func googleEpochSeconds(rec []json.RawMessage, i int) int64 {
	if i >= len(rec) {
		return 0
	}
	var ts []int64
	if err := json.Unmarshal(rec[i], &ts); err != nil || len(ts) == 0 {
		return 0
	}
	return ts[0]
}
