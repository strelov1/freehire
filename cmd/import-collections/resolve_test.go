package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strelov1/freehire/internal/job/collections"
)

func TestResolveOne_TreatsAZeroRecordParseAsFailure(t *testing.T) {
	// A source that has changed shape parses to zero rows just as convincingly as a
	// genuinely empty register. Letting it through would reconcile the tag off every
	// company, so it must fail like a fetch failure does.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := collections.Collection{
		Slug:    "x",
		Kind:    collections.KindEditorial,
		Dataset: &collections.Dataset{URL: srv.URL, Parse: func([]byte) ([]collections.Record, error) { return nil, nil }},
	}
	if _, err := resolveOne(context.Background(), srv.Client(), c); err == nil {
		t.Error("resolveOne accepted a dataset that parsed to no records")
	}
}

func TestResolveOne_FetchesFromTheDatasetResolver(t *testing.T) {
	var fetched string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched = r.URL.Path
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	c := collections.Collection{
		Slug: "x",
		Kind: collections.KindEditorial,
		Dataset: &collections.Dataset{
			ResolveURL: func(context.Context, *http.Client) (string, error) { return srv.URL + "/resolved.csv", nil },
			Parse:      func([]byte) ([]collections.Record, error) { return []collections.Record{{Name: "Acme"}}, nil },
		},
	}
	got, err := resolveOne(context.Background(), srv.Client(), c)
	if err != nil {
		t.Fatalf("resolveOne: %v", err)
	}
	if fetched != "/resolved.csv" {
		t.Errorf("fetched %q, want the resolver's URL", fetched)
	}
	if len(got) != 1 {
		t.Errorf("records = %+v, want one", got)
	}
}

func TestResolveOne_UsesASelfFetchingDatasetsRecords(t *testing.T) {
	// A paginated directory cannot be expressed as one URL, so it fetches itself. The
	// worker must route it the same as any other dataset — including through the
	// zero-record guard below.
	c := collections.Collection{
		Slug: "a16z-portfolio",
		Kind: collections.KindBacker,
		Dataset: &collections.Dataset{
			Records: func(context.Context, *http.Client) ([]collections.Record, error) {
				return []collections.Record{{Name: "Anduril Industries"}}, nil
			},
		},
	}
	got, err := resolveOne(context.Background(), http.DefaultClient, c)
	if err != nil {
		t.Fatalf("resolveOne: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Anduril Industries" {
		t.Errorf("records = %+v, want the directory's one record", got)
	}
}

func TestResolveOne_ASelfFetchingDatasetThatReadsNothingIsAFailure(t *testing.T) {
	// Same reasoning as every other source: zero records parses just as cleanly as a
	// genuinely empty directory would, and would reconcile the tag off every company.
	c := collections.Collection{
		Slug: "a16z-speedrun",
		Kind: collections.KindBacker,
		Dataset: &collections.Dataset{
			Records: func(context.Context, *http.Client) ([]collections.Record, error) { return nil, nil },
		},
	}
	if _, err := resolveOne(context.Background(), http.DefaultClient, c); err == nil {
		t.Error("an empty directory read was accepted as an empty membership")
	}
}

func TestResolveOne_PropagatesASelfFetchingDatasetFailure(t *testing.T) {
	c := collections.Collection{
		Slug: "a16z-portfolio",
		Kind: collections.KindBacker,
		Dataset: &collections.Dataset{
			Records: func(context.Context, *http.Client) ([]collections.Record, error) {
				return nil, errors.New("page 2: status 502")
			},
		},
	}
	if _, err := resolveOne(context.Background(), http.DefaultClient, c); err == nil {
		t.Error("resolveOne swallowed a directory fetch failure")
	}
}

func TestResolveOne_PropagatesAResolverFailure(t *testing.T) {
	c := collections.Collection{
		Slug: "x",
		Kind: collections.KindCredential,
		Dataset: &collections.Dataset{
			ResolveURL: func(context.Context, *http.Client) (string, error) { return "", errors.New("no csv link") },
			Parse:      func([]byte) ([]collections.Record, error) { return nil, nil },
		},
	}
	if _, err := resolveOne(context.Background(), http.DefaultClient, c); err == nil {
		t.Error("resolveOne swallowed a resolver failure")
	}
}
