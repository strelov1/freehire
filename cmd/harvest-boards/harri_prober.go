package main

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// harriProber validates a Harri board (a brand's career-page slug) in three cheap requests: the
// slug lookup naming the brand id, the brand's basic info for its display name, and a one-row
// search whose `hits` is the brand tree's open-posting count. It mirrors the requests
// internal/ingest/sources/harri.go makes, minus the per-posting detail fan-out (a group's brand
// lists hundreds of postings). The name is the brand's own, not a store's, so it is safe as a
// label.
type harriProber struct{}

const harriGateway = "https://gateway.harri.com"

func (harriProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	slug := strings.ToLower(strings.TrimSpace(board))
	if slug == "" {
		return "", 0, nil
	}
	// An unknown slug answers HTTP 400 with a string where the profile would be; either way
	// there is no brand id, and the board is not live.
	var profile struct {
		Data struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, harriGateway+"/core/api/v1/profile/slug/"+url.PathEscape(slug), &profile); err != nil ||
		profile.Data.ID == 0 || profile.Data.Type != "brand" {
		return "", 0, nil
	}
	id := profile.Data.ID

	var search struct {
		Data struct {
			Hits int `json:"hits"`
		} `json:"data"`
	}
	body := map[string]any{
		"start": 0, "size": 1, "source": "web", "brand_level_ids": []int64{id},
		"sort": []string{"publish_date"}, "sort_type": "desc", "flow": "CAREER_PORTAL",
	}
	if err := c.PostJSON(ctx, harriGateway+"/core/api/v1/harri_search/search_jobs", body, &search); err != nil {
		return "", 0, err
	}
	if search.Data.Hits == 0 {
		return "", 0, nil
	}

	// The name is a nicety: a brand whose basic info cannot be read is still live, and the
	// caller falls back to the seed's company.
	var info struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, harriGateway+"/core/api/v2/career_portal/brands/"+strconv.FormatInt(id, 10)+"/basic_info", &info); err != nil {
		return "", search.Data.Hits, nil
	}
	return strings.Join(strings.Fields(info.Data.Name), " "), search.Data.Hits, nil
}
