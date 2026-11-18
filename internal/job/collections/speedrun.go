package collections

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// speedrunDirectoryURL is the talent network's public company directory. The
// `source` parameter names our crawler, which the API asks every integration to do.
const speedrunDirectoryURL = "https://speedrun-talent-network.com/api/v1/companies"

// speedrunSourceTag identifies us to the API, matching internal/ingest/sources/speedrun.go.
const speedrunSourceTag = "freehire"

// speedrunUserAgent names us to the directory. Required: the network fails Go's default
// `Go-http-client/2.0` with a bare 500. Matches the liveness prober's agent string.
const speedrunUserAgent = "freehire/0.1 (+https://freehire.me)"

// speedrunPageLimit bounds the walk, since a malformed or hostile response could report an
// unbounded page count.
const speedrunPageLimit = 100

// The directory's tier value for the accelerator's cohorts. Its `a16z` tier is the
// subset of the fund's portfolio on the talent network; a16z-portfolio reads the full
// portfolio from a16z.com instead (see portfolios.go). `market` is the general job
// market, not an a16z relationship, and nothing here may import it.
const speedrunTierAccelerator = "speedrun"

// The a16z Speedrun talent network publishes its company directory as a paginated
// JSON API, the same one internal/ingest/sources/speedrun.go crawls for roles. It is the
// membership source for both a16z collections.
//
// Deriving membership from `source='speedrun'` on our jobs would miss the portfolio companies
// crawled through their own Ashby/Greenhouse/Lever boards.

// speedrunPage is one response: the companies on this page plus the page count the
// walker needs to know how many more to fetch.
type speedrunPage struct {
	Records    []Record
	TotalPages int
}

// speedrunPayload mirrors the API response. Only the fields membership depends on
// are read: the display name to match against our catalogue, and the tier that
// decides which collection the company belongs to.
type speedrunPayload struct {
	Companies []struct {
		Name   string `json:"name"`
		Tier   string `json:"tier"`
		Cohort string `json:"cohort"`
	} `json:"companies"`
	TotalPages int `json:"total_pages"`
}

// parseSpeedrunPage reads one directory page. An entry with no name or no tier is
// dropped: the first can never match a company, the second belongs to no collection,
// and keeping either would only inflate the counts a dry run is read for.
func parseSpeedrunPage(data []byte) (speedrunPage, error) {
	var payload speedrunPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return speedrunPage{}, fmt.Errorf("collections: parse speedrun directory: %w", err)
	}
	page := speedrunPage{TotalPages: payload.TotalPages}
	for _, c := range payload.Companies {
		if c.Name == "" || c.Tier == "" {
			continue
		}
		page.Records = append(page.Records, Record{
			Name: c.Name,
			Meta: map[string]string{"tier": c.Tier, "cohort": c.Cohort},
		})
	}
	return page, nil
}

// fetchSpeedrunDirectory walks every page of the directory and returns all of its
// records. The API pins its page size, so walking is the only way to read it all.
//
// Any page failing fails the whole call: a partial directory parses cleanly, so the import
// would treat unreached companies as having left the collection and reconcile the tag off them.
func fetchSpeedrunDirectory(ctx context.Context, client *http.Client, base string) ([]Record, error) {
	var all []Record
	for page := 0; page < speedrunPageLimit; page++ {
		got, err := fetchSpeedrunPage(ctx, client, base, page)
		if err != nil {
			return nil, err
		}
		all = append(all, got.Records...)
		if page+1 >= got.TotalPages {
			return all, nil
		}
	}
	return nil, fmt.Errorf("collections: speedrun directory exceeded %d pages", speedrunPageLimit)
}

// speedrunMembers builds the Dataset.Records source for one directory tier.
//
// Of the three tiers only `a16z` (the fund's portfolio) and `speedrun` (the accelerator's
// cohorts) matter; `market` is the general job market. Selecting by tier rather than excluding
// `market` means a tier added upstream is left out by default.
func speedrunMembers(tier string) func(context.Context, *http.Client) ([]Record, error) {
	return func(ctx context.Context, client *http.Client) ([]Record, error) {
		return speedrunMembersFrom(ctx, client, speedrunDirectoryURL, tier)
	}
}

func speedrunMembersFrom(ctx context.Context, client *http.Client, base, tier string) ([]Record, error) {
	all, err := fetchSpeedrunDirectory(ctx, client, base)
	if err != nil {
		return nil, err
	}
	members := make([]Record, 0, len(all))
	for _, r := range all {
		if r.Meta["tier"] == tier {
			members = append(members, r)
		}
	}
	return members, nil
}

func fetchSpeedrunPage(ctx context.Context, client *http.Client, base string, page int) (speedrunPage, error) {
	q := url.Values{"source": {speedrunSourceTag}, "page": {fmt.Sprint(page)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return speedrunPage{}, err
	}
	req.Header.Set("User-Agent", speedrunUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return speedrunPage{}, fmt.Errorf("collections: speedrun page %d: %w", page, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return speedrunPage{}, fmt.Errorf("collections: speedrun page %d: status %d", page, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return speedrunPage{}, fmt.Errorf("collections: speedrun page %d: %w", page, err)
	}
	return parseSpeedrunPage(body)
}
