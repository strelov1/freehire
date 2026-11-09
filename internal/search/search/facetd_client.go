package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// FacetdClient is a FacetEngine that talks to the facetd service over HTTP.
//
// Wire format (JSON, the server implements exactly this):
//
//	POST {base}/facets   {"filter": F, "facets": ["skills", ...]}
//	  200 {"total": 123, "distribution": {"skills": {"go": 10}},
//	       "stats": {"enrichment.salary_min": {"min": 1, "max": 2}}}
//	POST {base}/distinct {"filter": F, "attr": "company_slug"}
//	  200 {"count": 42}
//
// F is null, a string, an array of strings, or an array of arrays of strings
// (the Meilisearch filter shapes). 422 {"error": "..."} means the filter or
// attribute is unsupported (ErrEngineUnsupported); 503 means not ready
// (ErrEngineNotReady); 400 to a /write is ErrFacetdRejected; anything else is a plain error. The deadline is the ctx's.
type FacetdClient struct {
	base string
	hc   *http.Client
}

// NewFacetdClient returns a client for the facetd service at baseURL.
func NewFacetdClient(baseURL string, hc *http.Client) *FacetdClient {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &FacetdClient{base: strings.TrimRight(baseURL, "/"), hc: hc}
}

type facetdFilter struct {
	Filter json.RawMessage `json:"filter"`
}

type facetdFacetsReq struct {
	facetdFilter
	Facets []string `json:"facets"`
}

type facetdDistinctReq struct {
	facetdFilter
	Attr string `json:"attr"`
}

type facetdFacetsResp struct {
	Total        int64                       `json:"total"`
	Distribution map[string]map[string]int64 `json:"distribution"`
	Stats        map[string]FacetStat        `json:"stats"`
}

func (c *FacetdClient) Facets(ctx context.Context, filter any, facets []string) (FacetEngineAnswer, error) {
	f, err := facetdFilterJSON(filter)
	if err != nil {
		return FacetEngineAnswer{}, err
	}
	var r facetdFacetsResp
	if err := c.post(ctx, "/facets", facetdFacetsReq{facetdFilter{f}, facets}, &r); err != nil {
		return FacetEngineAnswer{}, err
	}
	return FacetEngineAnswer(r), nil
}

func (c *FacetdClient) DistinctCount(ctx context.Context, filter any, attr string) (int64, error) {
	f, err := facetdFilterJSON(filter)
	if err != nil {
		return 0, err
	}
	var r struct {
		Count int64 `json:"count"`
	}
	if err := c.post(ctx, "/distinct", facetdDistinctReq{facetdFilter{f}, attr}, &r); err != nil {
		return 0, err
	}
	return r.Count, nil
}

// facetdFilterJSON marshals filter and refuses anything that is not null, a
// string, an array of strings, or an array of arrays of strings.
func facetdFilterJSON(filter any) (json.RawMessage, error) {
	b, err := json.Marshal(filter)
	if err != nil {
		return nil, fmt.Errorf("%w: filter: %v", ErrEngineUnsupported, err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("%w: filter: %v", ErrEngineUnsupported, err)
	}
	ok := false
	switch t := v.(type) {
	case nil, string:
		ok = true
	case []any:
		ok = true
		for _, e := range t {
			switch e := e.(type) {
			case string:
			case []any:
				for _, s := range e {
					if _, isStr := s.(string); !isStr {
						ok = false
					}
				}
			default:
				ok = false
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("%w: filter shape %T", ErrEngineUnsupported, filter)
	}
	return b, nil
}

func (c *FacetdClient) post(ctx context.Context, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("facetd %s: %w", path, err) // keeps context.DeadlineExceeded matchable
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("facetd %s: decode: %w", path, err)
		}
		return nil
	case http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: %s", ErrEngineUnsupported, facetdErrBody(resp.Body))
	case http.StatusServiceUnavailable:
		return fmt.Errorf("%w: %s", ErrEngineNotReady, facetdErrBody(resp.Body))
	case http.StatusBadRequest:
		return fmt.Errorf("%w: facetd %s: %s", ErrFacetdRejected, path, facetdErrBody(resp.Body))
	}
	return fmt.Errorf("facetd %s: status %d: %s", path, resp.StatusCode, facetdErrBody(resp.Body))
}

func facetdErrBody(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 512))
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil && e.Error != "" {
		return e.Error
	}
	return strings.TrimSpace(string(b))
}

// FacetdWrite is one /write batch: documents to upsert and ids to close, mark duplicate or
// delete. Empty groups are omitted from the wire.
type FacetdWrite struct {
	Upserts   []JobDocument
	Close     []int64
	Duplicate []int64
	Delete    []int64
}

// Empty reports whether the batch carries nothing to send.
func (w FacetdWrite) Empty() bool {
	return len(w.Upserts)+len(w.Close)+len(w.Duplicate)+len(w.Delete) == 0
}

// Write applies one batch to facetd (POST /write). The documents go without their
// `_vectors`: facetd indexes none, and the description chunks are most of a document's bytes.
// A not-ready facetd answers ErrEngineNotReady.
func (c *FacetdClient) Write(ctx context.Context, w FacetdWrite) error {
	slim := make([]JobDocument, len(w.Upserts))
	for i, d := range w.Upserts {
		d.Vectors = DocumentVectors{}
		slim[i] = d
	}
	body := struct {
		Upserts   []JobDocument `json:"upserts"`
		Close     []int64       `json:"close"`
		Duplicate []int64       `json:"duplicate"`
		Delete    []int64       `json:"delete"`
	}{slim, w.Close, w.Duplicate, w.Delete}
	return c.post(ctx, "/write", body, &struct{}{})
}

// Ready returns nil when facetd answers GET /ready with 200, ErrEngineNotReady on 503.
func (c *FacetdClient) Ready(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/ready", nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("facetd /ready: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusServiceUnavailable:
		return fmt.Errorf("%w: %s", ErrEngineNotReady, facetdErrBody(resp.Body))
	}
	return fmt.Errorf("facetd /ready: status %d", resp.StatusCode)
}

// IDs lists the job id of every change in the batch.
func (w FacetdWrite) IDs() []int64 {
	ids := make([]int64, 0, len(w.Upserts)+len(w.Close)+len(w.Duplicate)+len(w.Delete))
	for _, d := range w.Upserts {
		ids = append(ids, d.ID)
	}
	ids = append(ids, w.Close...)
	ids = append(ids, w.Duplicate...)
	return append(ids, w.Delete...)
}
