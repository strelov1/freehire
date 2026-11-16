package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// personioProber lists a board like the adapter: the XML feed on the .com host, and search.json when
// the tenant's feed answers 404. An empty feed that answers is not live, since the adapter does not
// fall back from it either. A tenant served only on a regional TLD (.de/.es) is skipped, as the
// adapter could not crawl it. No company name, so it falls back to the slug.
type personioProber struct{}

func (personioProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	host := fmt.Sprintf("https://%s.jobs.personio.com", slug)
	var resp struct {
		Positions []struct{} `xml:"position"`
	}
	err := c.GetXML(ctx, host+"/xml", &resp)
	if err == nil {
		return "", len(resp.Positions), nil
	}
	var se *sources.StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		return "", 0, nil
	}
	var jobs []struct{}
	if err := c.GetJSON(ctx, host+"/search.json", &jobs); err != nil {
		return "", 0, nil
	}
	return "", len(jobs), nil
}
