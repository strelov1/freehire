package sources

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
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
//
// The same shape means a live ad that does not change is never re-sent, so its last_seen_at
// stops moving three days after it was published and says nothing about whether it is still
// up; and a removal the crawl fails to apply inside the window is never sent again.
// cmd/liveness's jobtech pass is that miss's backstop, asking JobSearch about each ad unseen
// past the sweep window (see cmd/liveness/jobtech.go).
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

// FetchStream reads the trailing change window and emits its removals — each a Job with
// Removed set, which the pipeline closes by identity — and then its live ads, as Jobs to
// upsert.
//
// The window is spooled to a temporary file before anything is emitted. Every emit is a
// database write from the crawl host, a few a second, and the window runs to ~18k items
// (80 MB on 2026-10-31); decoding while writing held the download open behind the writes until
// the stream client's timeout ended it a few thousand items in. JobStream lists live ads first
// and removals after them, so that cut fell before every removal: from 2026-10-27 no run closed
// anything, and a removal that ages out of the window is never sent again.
//
// Removals go first for the same reason — they are the only close signal this source has,
// while a live ad not reached this run is re-sent by every run for the window's 72h. An ad the
// window lists as removed is not emitted live as well, so the order cannot reopen it.
//
// A window that could not be read whole — the connection dropped, the body ended mid-array —
// still emits what it decoded, then returns the failure: the run did not read its window, and
// the part a cut-short read loses is exactly the removals.
func (s jobtech) FetchStream(ctx context.Context, _ CompanyEntry, emit func(Job)) error {
	spool, err := os.CreateTemp("", "jobtech-stream-*.json")
	if err != nil {
		return fmt.Errorf("jobtech: spool: %w", err)
	}
	defer func() {
		_ = spool.Close()
		_ = os.Remove(spool.Name())
	}()

	since := time.Now().UTC().Add(-jobtechStreamWindow).Format(jobtechDateLayout)
	q := url.Values{}
	q.Set("date", since)
	var read int64
	readErr := s.http.GetStream(ctx, jobtechStreamURL+"?"+q.Encode(), "application/json", func(r io.Reader) error {
		var err error
		read, err = io.Copy(spool, r)
		return err
	})
	if read == 0 {
		if readErr != nil {
			return readErr
		}
		return errors.New("jobtech: empty stream body")
	}
	if readErr != nil {
		readErr = fmt.Errorf("jobtech: stream cut short after %d bytes: %w", read, readErr)
	}

	removed := make(map[string]bool)
	removalsErr := eachJobtechItem(ctx, spool, func(it jobtechItem) {
		if it.Removed && it.ID != "" && !removed[it.ID] {
			removed[it.ID] = true
			emit(Job{ExternalID: it.ID, Removed: true})
		}
	})
	liveErr := eachJobtechItem(ctx, spool, func(it jobtechItem) {
		if it.Removed || removed[it.ID] {
			return
		}
		if job, ok := it.toJob(); ok {
			emit(job)
		}
	})
	// A bad body fails both passes at the same item; the read failure, when there is one, is
	// the cause worth reporting.
	return cmp.Or(readErr, liveErr, removalsErr)
}

// eachJobtechItem decodes the spooled window from its start and hands each item to fn, one at
// a time so memory stays flat however wide the window is. It stops at the first item that does
// not decode (a body cut short) and reports it.
func eachJobtechItem(ctx context.Context, spool *os.File, fn func(jobtechItem)) error {
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("jobtech: rewind spool: %w", err)
	}
	dec := json.NewDecoder(bufio.NewReader(spool))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
		return errors.New("jobtech: stream body is not a JSON array")
	}
	for dec.More() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var it jobtechItem
		if err := dec.Decode(&it); err != nil {
			return fmt.Errorf("jobtech: decode item: %w", err)
		}
		fn(it)
	}
	// A body cut right after a complete item ends the loop as cleanly as a whole one does; only
	// the closing bracket tells them apart.
	if tok, err := dec.Token(); err != nil || tok != json.Delim(']') {
		return errors.New("jobtech: stream body ended before its closing ]")
	}
	return nil
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
