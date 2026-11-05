package collections

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// The GOV.UK register of licensed sponsors. The CSV is republished each month at a
// fresh URL carrying the snapshot date, so the address is discovered per run rather
// than pinned: the Content API is asked first, and the publication page is scraped
// if that fails. Two mechanisms because they break independently — the API can
// change shape while the page still renders, and vice versa.
const (
	ukSponsorAPIURL  = "https://www.gov.uk/api/content/government/publications/register-of-licensed-sponsors-workers"
	ukSponsorPageURL = "https://www.gov.uk/government/publications/register-of-licensed-sponsors-workers"

	// The attachment and link text identifying the worker register, distinguishing it
	// from the student-sponsor register published alongside it.
	ukSponsorAttachmentTitle = "Register of Worker and Temporary Worker licensed sponsors"
)

// errNoSponsorCSV means a source was reachable but held no register link. It is a
// sentinel so the resolver can fall through to the next source rather than abort.
var errNoSponsorCSV = errors.New("collections: no worker-register CSV found")

// contentAPIResponse is the slice of the GOV.UK Content API payload we read.
type contentAPIResponse struct {
	Details struct {
		Attachments []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			ContentType string `json:"content_type"`
		} `json:"attachments"`
	} `json:"details"`
}

// pickSponsorCSV extracts the worker-register CSV URL from a Content API payload.
func pickSponsorCSV(payload []byte) (string, error) {
	var resp contentAPIResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return "", fmt.Errorf("collections: parse gov.uk content api: %w", err)
	}
	for _, a := range resp.Details.Attachments {
		if a.ContentType == "text/csv" && strings.EqualFold(strings.TrimSpace(a.Title), ukSponsorAttachmentTitle) {
			return a.URL, nil
		}
	}
	return "", errNoSponsorCSV
}

// sponsorCSVLink matches an anchor pointing at a CSV on the GOV.UK assets host whose
// link text names the worker register. Anchored on the assets host on purpose: the
// page also offers an on-site /csv-preview/ link to the same data, which serves HTML.
var sponsorCSVLink = regexp.MustCompile(
	`(?is)<a[^>]+href="(https://assets\.publishing\.service\.gov\.uk/[^"]+\.csv)"[^>]*>([^<]*)</a>`)

// scrapeSponsorCSV extracts the worker-register CSV URL from the publication page.
func scrapeSponsorCSV(page []byte) (string, error) {
	for _, m := range sponsorCSVLink.FindAllStringSubmatch(string(page), -1) {
		if strings.Contains(strings.ToLower(m[2]), strings.ToLower(ukSponsorAttachmentTitle)) {
			return m[1], nil
		}
	}
	return "", errNoSponsorCSV
}

// resolveUKSponsorCSV discovers the current register CSV URL, preferring the
// Content API and falling back to scraping the publication page. Only when neither
// yields a URL is this an error — and it must be an error, because an empty resolve
// would reconcile the credential off every company.
func resolveUKSponsorCSV(ctx context.Context, client *http.Client, apiURL, pageURL string) (string, error) {
	if body, err := get(ctx, client, apiURL); err == nil {
		if url, err := pickSponsorCSV(body); err == nil {
			return url, nil
		}
	}
	body, err := get(ctx, client, pageURL)
	if err != nil {
		return "", fmt.Errorf("collections: fetch uk sponsor register page: %w", err)
	}
	return scrapeSponsorCSV(body)
}

// ResolveUKSponsorCSV is the registry-facing resolver, bound to the live GOV.UK
// addresses.
func ResolveUKSponsorCSV(ctx context.Context, client *http.Client) (string, error) {
	return resolveUKSponsorCSV(ctx, client, ukSponsorAPIURL, ukSponsorPageURL)
}

// ParseUKSponsors reads the register CSV into one record per organisation-route
// row, carrying the town (the identity field that tells two same-named
// organisations apart), the licence rating, and the sponsorship route the gate
// reads. Every route is kept, including the temporary and study routes the gate
// then refuses — retaining them keeps the parse honest about what the register says.
func ParseUKSponsors(data []byte) ([]Record, error) {
	rows, err := csvColumns(data, ',', "Organisation Name", "Town/City", "Type & Rating", "Route")
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(rows))
	for _, row := range rows {
		if row[0] == "" {
			continue
		}
		records = append(records, Record{
			Name: row[0],
			Meta: map[string]string{"town": row[1], "rating": row[2], "route": row[3]},
		})
	}
	if len(records) == 0 {
		return nil, errors.New("collections: uk sponsor register parsed to no rows")
	}
	return records, nil
}

// rateLimitWait is the pause before the first retry of a 429; it doubles per retry.
// open.canada.ca answers about every second request of a quick burst with 429
// (measured 2026-11-05), which failed the whole ca-lmia-employer import.
var rateLimitWait = 2 * time.Second

const rateLimitAttempts = 4

// get fetches a URL and returns its body, treating any non-200 as an error. A 429 is
// retried after a growing pause, since it says "later", not "no".
func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	wait := rateLimitWait
	for attempt := 1; ; attempt++ {
		body, status, err := getOnce(ctx, client, url)
		if status != http.StatusTooManyRequests || attempt == rateLimitAttempts {
			return body, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		wait *= 2
	}
}

func getOnce(ctx context.Context, client *http.Client, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}
