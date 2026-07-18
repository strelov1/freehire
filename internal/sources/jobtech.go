package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"time"
)

// jobtech adapts the Swedish Public Employment Service (Arbetsförmedlingen) feed behind
// Platsbanken, via JobStream — the bulk mirror API at jobstream.api.jobtechdev.se. Like the
// other multi-company feeds it is boardless (one national API, no per-tenant board) and an
// aggregator (it stays in the source facet and takes each posting's employer from the feed).
//
// JobStream is heavily throttled and its /search sibling is far too slow to page the whole
// catalogue, so this adapter consumes the incremental /stream endpoint: each run asks for
// the changes in a trailing window and applies them — live ads upserted, ads flagged
// "removed" closed by identity. That makes jobtech a self-closing source: it manages its own
// closes from the stream and is excluded from the post-run unseen sweep (which would wrongly
// close every still-open ad the incremental stream did not re-report).
//
// Consequence (a documented seam, not a bug): the stream only carries CHANGES, so the
// catalogue is a rolling window of recent Swedish postings, not the full ~40k back-catalogue.
// Seeding the full history would need a one-time /snapshot bootstrap — the throttled,
// multi-hour path deliberately not built here.
type jobtech struct {
	http StreamGetter
}

const (
	// jobtechStreamURL is the JobStream incremental endpoint. Keyless (open government data).
	jobtechStreamURL = "https://jobstream.api.jobtechdev.se/stream"
	// jobtechStreamWindow is the trailing change window each run requests. It must comfortably
	// exceed the cron interval so a delayed or skipped run cannot miss a removal event: at 72h
	// a daily cron has three runs of overlap. The idempotent upsert absorbs the re-sent
	// overlap, so a wider window only costs a little bandwidth.
	jobtechStreamWindow = 72 * time.Hour
	// jobtechDateLayout matches both the date query parameter and publication_date
	// ("2026-06-28T00:47:00") — no timezone, so time.RFC3339 would not parse it.
	jobtechDateLayout = "2006-01-02T15:04:05"
)

// NewJobtech builds the JobTech (Arbetsförmedlingen) adapter over the given streaming client.
func NewJobtech(c StreamGetter) Source { return jobtech{http: c} }

func (jobtech) Provider() string { return "jobtech" }

func (jobtech) boardless() {}

func (jobtech) aggregator() {}

func (jobtech) selfClosing() {}

// jobtechItem is one element of the JobStream array: an ad, body inline
// (description.text_formatted is HTML), plus the removed flag that drives a close.
type jobtechItem struct {
	ID              string `json:"id"`
	Removed         bool   `json:"removed"`
	WebpageURL      string `json:"webpage_url"`
	Headline        string `json:"headline"`
	PublicationDate string `json:"publication_date"`
	Description     struct {
		Text          string `json:"text"`
		TextFormatted string `json:"text_formatted"`
	} `json:"description"`
	Employer struct {
		// Name is the legal entity (often a numbered shell AB, e.g. "Miro 461704 AB");
		// Workplace is the trading name the shop actually operates under ("Direkten Nöje
		// Casablanca"). Workplace is the meaningful company display — see toJob.
		Name      string `json:"name"`
		Workplace string `json:"workplace"`
	} `json:"employer"`
	WorkplaceAddress struct {
		Municipality string `json:"municipality"`
		Region       string `json:"region"`
		Country      string `json:"country"`
	} `json:"workplace_address"`
}

// Fetch collects the window into a slice (non-streaming callers and tests); the pipeline
// uses FetchStream so the crawl persists incrementally rather than buffering.
func (s jobtech) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	err := s.FetchStream(ctx, e, func(j Job) { jobs = append(jobs, j) })
	return jobs, err
}

// FetchStream streams the trailing change window and emits each item as it is decoded: a
// live ad as a Job to upsert, a removed ad as a Job with Removed set (the pipeline closes it
// by identity). The JobStream body is a single large, throttled JSON array, so it is decoded
// token-by-token (constant memory) rather than buffered. A mid-stream read failure after the
// first emit returns nil — the throttled feed dropping the connection is expected, and the
// already-emitted items are persisted; only a failure before any progress is a board error.
func (s jobtech) FetchStream(ctx context.Context, _ CompanyEntry, emit func(Job)) error {
	since := time.Now().UTC().Add(-jobtechStreamWindow).Format(jobtechDateLayout)
	q := url.Values{}
	q.Set("date", since)
	streamURL := jobtechStreamURL + "?" + q.Encode()

	emitted := 0
	return s.http.GetStream(ctx, streamURL, "application/json", func(r io.Reader) error {
		dec := json.NewDecoder(r)
		if _, err := dec.Token(); err != nil { // consume the opening '['
			return fmt.Errorf("jobtech: stream open: %w", err)
		}
		for dec.More() {
			var it jobtechItem
			if err := dec.Decode(&it); err != nil {
				if emitted > 0 {
					return nil // throttled stream dropped mid-flight: keep what was emitted
				}
				return fmt.Errorf("jobtech: decode item: %w", err)
			}
			if it.Removed {
				if it.ID != "" {
					emit(Job{ExternalID: it.ID, Removed: true})
					emitted++
				}
				continue
			}
			if job, ok := it.toJob(); ok {
				emit(job)
				emitted++
			}
		}
		return nil
	})
}

// toJob maps a live ad to a Job, returning ok=false for an ad with no id to key on or no
// company to attribute it to (which would break the company slug). The company is the
// trading name (employer.workplace) when present, falling back to the legal entity
// (employer.name) — the numbered shell AB the workplace registers under is both a poor
// display and a magnet for a wrong-brand logo.dev name match. Remote/WorkMode are left
// unset: the API's remote_work flag is effectively always null, so the work arrangement is
// derived downstream from the location string by the deterministic dictionary, not guessed.
func (it jobtechItem) toJob() (Job, bool) {
	company := it.Employer.Workplace
	if company == "" {
		company = it.Employer.Name
	}
	if it.ID == "" || company == "" {
		return Job{}, false
	}
	desc := it.Description.TextFormatted
	if desc == "" {
		desc = it.Description.Text
	}
	return Job{
		ExternalID:  it.ID,
		URL:         it.WebpageURL,
		Title:       it.Headline,
		Company:     company,
		Location:    joinNonEmpty(it.WorkplaceAddress.Municipality, it.WorkplaceAddress.Region, it.WorkplaceAddress.Country),
		Description: sanitizeHTML(desc),
		PostedAt:    parseLayout(jobtechDateLayout, it.PublicationDate),
	}, true
}
