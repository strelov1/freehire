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

// speedrunSourceTag identifies us to the API, matching internal/sources/speedrun.go.
const speedrunSourceTag = "freehire"

// speedrunPageLimit bounds the walk. The directory reports its own page count, but a
// malformed or hostile response could report an unbounded one; 9 pages is the live
// size, so this leaves a wide margin while keeping the run finite.
const speedrunPageLimit = 100

// The directory's own tier values. `market` is deliberately absent: it is the
// general job market, not an a16z relationship, and nothing here may import it.
const (
	speedrunTierPortfolio   = "a16z"
	speedrunTierAccelerator = "speedrun"
)

// The a16z Speedrun talent network publishes its company directory as a paginated
// JSON API — the same one internal/sources/speedrun.go crawls for roles. It is the
// membership source for both a16z collections.
//
// The directory is the only honest source for this membership. Deriving it from
// `source='speedrun'` on our jobs would find only the portfolio companies that host
// their application on the network itself (11 of them); the rest are crawled through
// their own Ashby/Greenhouse/Lever boards and would be missed.

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
// records. The API pins its page size at 100 regardless of what is asked for, so the
// walk is the only way to read the whole thing.
//
// Any page failing fails the whole call. A partial directory is worse than none: it
// parses cleanly, so the import worker would treat the companies it never reached as
// having left the collection and reconcile the tag off them.
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
// The directory carries three tiers and only two of them mean anything here: `a16z`
// is the fund's portfolio, `speedrun` is the accelerator's cohorts, and `market` is
// the general job market — Walmart, TikTok, Amazon — which has no relationship to
// a16z at all. Selecting by tier rather than filtering `market` out is deliberate: a
// tier added upstream tomorrow is then excluded by default rather than silently
// swept into whichever tag ran the exclusion.
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
