package main

import (
	"context"
	"fmt"
)

// traffitProber probes a Traffit tenant. A board is the tenant subdomain; the keyless
// list endpoint returns JSON with the open-posting count for a real tenant, and an HTML
// placeholder for a non-tenant subdomain (which fails to decode and is skipped). Traffit
// serves every tenant under a wildcard cert/DNS, so there is no keyless way to enumerate
// candidates — the prober validates a supplied seed list rather than discovering its own.
type traffitProber struct{}

// probe queries one tenant's list for its open-posting count. A missing tenant (getter
// error on the HTML placeholder) or one with no live postings yields ("", 0, nil) — a
// skip. The company name falls back to the slug; display names are curated in the seed file.
func (traffitProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Count int `json:"count"`
		Items []struct {
			AdvertID int64 `json:"advertId"`
		} `json:"items"`
	}
	url := fmt.Sprintf("https://%s.traffit.com/public/an/list/?limit=1", slug)
	if err := c.GetJSON(ctx, url, &resp); err != nil {
		return "", 0, nil
	}
	// Liveness is "the endpoint returned a posting", like every other prober.
	if len(resp.Items) == 0 {
		return "", 0, nil
	}
	return slug, resp.Count, nil
}
