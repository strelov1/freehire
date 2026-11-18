package main

import (
	"context"
	"fmt"
)

// traffitProber probes a Traffit tenant (the board is the subdomain). The keyless list endpoint
// returns JSON with the open-posting count for a real tenant and an HTML placeholder otherwise (fails
// to decode, skipped). Every tenant sits under a wildcard cert/DNS, so there is no keyless way to
// enumerate candidates: the prober validates a supplied seed list.
type traffitProber struct{}

// probe queries one tenant's list for its open-posting count. A missing tenant or one with no live
// postings yields ("", 0, nil). The name falls back to the slug; display names are curated in the seed file.
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
	if len(resp.Items) == 0 {
		return "", 0, nil
	}
	return "", resp.Count, nil
}
