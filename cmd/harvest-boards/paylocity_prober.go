package main

import (
	"context"
	"fmt"
	"strings"
)

// paylocityProber probes a recruiting.paylocity.com company board (slug = company GUID). The
// listing page embeds the openings in window.pageData's Jobs[] array; counting the JobId keys
// is enough to tell a live board (>=1 job) from an empty/dead one without a full parse. The
// employer name comes from the seed (the listing exposes none the prober bothers to read).
type paylocityProber struct{}

func (paylocityProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	body, err := c.GetText(ctx, fmt.Sprintf("https://recruiting.paylocity.com/Recruiting/Jobs/All/%s", slug))
	if err != nil {
		return "", 0, nil
	}
	return "", strings.Count(body, `"JobId":`), nil
}
