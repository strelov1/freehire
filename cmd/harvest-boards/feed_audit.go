package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"golang.org/x/net/html"
)

var auditFields = []string{
	"description", "location", "country", "work_mode", "employment_type",
	"skills", "salary", "salary_currency", "salary_period", "posted_at",
}

type auditInventory struct {
	Rows   int            `json:"rows"`
	Jobs   int            `json:"jobs"`
	Fields map[string]int `json:"fields_present"`
}

type feedComparison struct {
	Feed             auditInventory `json:"feed"`
	Adapter          auditInventory `json:"adapter"`
	Matched          int            `json:"matched_jobs"`
	MissingIDs       []string       `json:"missing_from_feed"`
	ExtraIDs         []string       `json:"only_in_feed"`
	BodyDifferences  []string       `json:"body_differences"`
	TitleDifferences []string       `json:"title_differences"`
	MissingFields    map[string]int `json:"matched_jobs_losing_fields"`
}

type feedAuditReport struct {
	Provider        string          `json:"provider"`
	Board           string          `json:"board"`
	CheckedAt       time.Time       `json:"checked_at"`
	URL             string          `json:"feed_url"`
	Schema          string          `json:"schema,omitempty"`
	Status          string          `json:"status"`
	Error           string          `json:"error,omitempty"`
	Feed            auditInventory  `json:"feed"`
	FeedRequests    int             `json:"feed_requests"`
	AdapterRequests int             `json:"adapter_requests"`
	Comparison      *feedComparison `json:"comparison,omitempty"`
}

func auditInventoryOf(o feedObservation) auditInventory {
	stats := auditInventory{Rows: o.Rows, Jobs: len(o.Jobs), Fields: map[string]int{}}
	for _, field := range auditFields {
		stats.Fields[field] = 0
	}
	for _, j := range o.Jobs {
		for _, field := range auditFields {
			if auditFieldPresent(j, field) {
				stats.Fields[field]++
			}
		}
	}
	return stats
}

func auditFieldPresent(j auditPosting, field string) bool {
	if field == "description" {
		return j.Body != ""
	}
	return j.Fields[field]
}

func adapterObservation(jobs []sources.Job) (feedObservation, error) {
	out := feedObservation{Rows: len(jobs), Jobs: map[string]auditPosting{}}
	for _, j := range jobs {
		if j.Unreadable {
			return out, fmt.Errorf("adapter could not read job %q", j.ExternalID)
		}
		if j.ExternalID == "" {
			return out, fmt.Errorf("adapter returned a job without an id")
		}
		if _, exists := out.Jobs[j.ExternalID]; exists {
			return out, fmt.Errorf("adapter repeated job %q", j.ExternalID)
		}
		out.Jobs[j.ExternalID] = auditPosting{
			ID: j.ExternalID, URL: j.URL, Title: strings.TrimSpace(j.Title),
			Body: auditBody(j.Description),
			Fields: map[string]bool{
				"location":        strings.TrimSpace(j.Location) != "",
				"country":         len(j.Countries) > 0,
				"work_mode":       j.WorkMode != "",
				"employment_type": j.EmploymentType != "",
				"skills":          len(j.Skills) > 0,
				"salary":          j.SalaryMin != nil || j.SalaryMax != nil,
				"salary_currency": j.SalaryCurrency != "",
				"salary_period":   j.SalaryPeriod != "",
				"posted_at":       j.PostedAt != nil,
			},
		}
	}
	return out, nil
}

func compareFeedAudit(feed, baseline feedObservation) feedComparison {
	out := feedComparison{
		Feed: auditInventoryOf(feed), Adapter: auditInventoryOf(baseline),
		MissingIDs: []string{}, ExtraIDs: []string{},
		BodyDifferences: []string{}, TitleDifferences: []string{},
		MissingFields: map[string]int{},
	}
	for _, field := range auditFields {
		out.MissingFields[field] = 0
	}
	for id, old := range baseline.Jobs {
		fresh, ok := feed.Jobs[id]
		if !ok {
			out.MissingIDs = append(out.MissingIDs, id)
			continue
		}
		out.Matched++
		if old.Body != fresh.Body {
			out.BodyDifferences = append(out.BodyDifferences, id)
		}
		if old.Title != fresh.Title {
			out.TitleDifferences = append(out.TitleDifferences, id)
		}
		for _, field := range auditFields {
			if auditFieldPresent(old, field) && !auditFieldPresent(fresh, field) {
				out.MissingFields[field]++
			}
		}
	}
	for id := range feed.Jobs {
		if _, ok := baseline.Jobs[id]; !ok {
			out.ExtraIDs = append(out.ExtraIDs, id)
		}
	}
	slices.Sort(out.MissingIDs)
	slices.Sort(out.ExtraIDs)
	slices.Sort(out.BodyDifferences)
	slices.Sort(out.TitleDifferences)
	return out
}

// These providers use only XML and HTML. Count failures even when a source treats them
// as best-effort and returns a successful partial Fetch. Counts are client calls, not
// transport retries or redirects performed inside sources.Client.
type auditClient struct {
	httpClient
	mu         sync.Mutex
	requests   int
	firstError error
}

func newAuditClient(c httpClient) *auditClient { return &auditClient{httpClient: c} }

func (c *auditClient) record(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests++
	if err != nil && c.firstError == nil {
		c.firstError = err
	}
}
func (c *auditClient) GetXML(ctx context.Context, u string, v any) error {
	err := c.httpClient.GetXML(ctx, u, v)
	c.record(err)
	return err
}
func (c *auditClient) GetHTML(ctx context.Context, u string) (*html.Node, error) {
	root, err := c.httpClient.GetHTML(ctx, u)
	c.record(err)
	return root, err
}
func (c *auditClient) stats() (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests, c.firstError
}

func readAuditBaseline(ctx context.Context, c *auditClient, src sources.Source, e sources.CompanyEntry) (feedObservation, error) {
	jobs, err := src.Fetch(ctx, e)
	if err != nil {
		return feedObservation{}, err
	}
	if err = ctx.Err(); err != nil {
		return feedObservation{}, err
	}
	if _, err = c.stats(); err != nil {
		return feedObservation{}, fmt.Errorf("adapter request failed: %w", err)
	}
	return adapterObservation(jobs)
}

func auditFeedBoard(ctx context.Context, c httpClient, e sources.CompanyEntry) feedAuditReport {
	report := feedAuditReport{Provider: e.Provider, Board: e.Board, CheckedAt: time.Now().UTC()}
	feedClient := newAuditClient(c)
	feed, err := readAuditFeed(ctx, feedClient, e.Provider, e.Board)
	report.URL, report.Schema = feed.URL, feed.Schema
	report.Feed = auditInventoryOf(feed)
	report.FeedRequests, _ = feedClient.stats()
	if err != nil {
		report.Status = "feed_failed"
		var status *sources.StatusError
		// A missing later RSS page means an incomplete feed, not an unavailable endpoint.
		if feed.Rows == 0 && errors.As(err, &status) && (status.Code == http.StatusNotFound || status.Code == http.StatusGone) {
			report.Status = "feed_unavailable"
		}
		report.Error = err.Error()
		return report
	}
	baselineClient := newAuditClient(c)
	var src sources.Source
	switch e.Provider {
	case "scalis":
		src = sources.NewScalis(baselineClient)
	case "teamtailor":
		src = sources.NewTeamtailor(baselineClient)
	}
	baseline, err := readAuditBaseline(ctx, baselineClient, src, e)
	report.AdapterRequests, _ = baselineClient.stats()
	if err != nil {
		report.Status = "baseline_failed"
		report.Error = err.Error()
		return report
	}
	comparison := compareFeedAudit(feed, baseline)
	report.Comparison = &comparison
	report.Status = "compared"
	return report
}
