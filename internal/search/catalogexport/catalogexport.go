// Package catalogexport writes the bulk catalogue snapshots sold with the API "Data" plan and
// reads them back for the download endpoint.
//
// A snapshot is two objects under catalogue/<UTC date>/: jobs.jsonl.gz (one public job per
// line, gzip) and manifest.json. The manifest is uploaded last, so a snapshot without one is
// unfinished and is neither listed nor counted by retention.
package catalogexport

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/platform/blobstore"
)

// SchemaVersion is the shape of a line of jobs.jsonl.gz (jobview.Job). Bump it when a field is
// removed or changes meaning; adding a field does not.
const SchemaVersion = 1

const prefix = "catalogue/"

// Manifest describes one finished snapshot.
type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	Date          string    `json:"date"` // UTC day, YYYY-MM-DD
	Rows          int64     `json:"rows"`
	Bytes         int64     `json:"bytes"` // size of the gzip file
	SHA256        string    `json:"sha256"`
	GeneratedAt   time.Time `json:"generated_at"`
}

// DataKey is the object key of the snapshot's gzip file.
func DataKey(date string) string { return prefix + date + "/jobs.jsonl.gz" }

// ManifestKey is the object key of the snapshot's manifest.
func ManifestKey(date string) string { return prefix + date + "/manifest.json" }

// Day is the UTC date a snapshot taken at t is named by.
func Day(t time.Time) string { return t.UTC().Format(time.DateOnly) }

// List returns the finished snapshots, newest first.
func List(ctx context.Context, b blobstore.Store) ([]Manifest, error) {
	dates, err := finishedDates(ctx, b)
	if err != nil {
		return nil, err
	}
	out := make([]Manifest, 0, len(dates))
	for _, d := range dates {
		m, err := readManifest(ctx, b, d)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func readManifest(ctx context.Context, b blobstore.Store, date string) (m Manifest, err error) {
	r, err := b.Get(ctx, ManifestKey(date))
	if err != nil {
		return m, err
	}
	defer r.Close()
	body, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return m, fmt.Errorf("catalogexport: read manifest %s: %w", date, err)
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return m, fmt.Errorf("catalogexport: manifest %s: %w", date, err)
	}
	return m, nil
}

// dateOf is the day directory of a key under prefix, or "" for a key outside the layout.
func dateOf(key string) string {
	rest, ok := strings.CutPrefix(key, prefix)
	if !ok {
		return ""
	}
	date, _, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return ""
	}
	return date
}

// finishedDates are the days that have a manifest, newest first.
func finishedDates(ctx context.Context, b blobstore.Store) ([]string, error) {
	keys, err := b.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	var dates []string
	for _, k := range keys {
		if d := dateOf(k); d != "" && k == ManifestKey(d) {
			dates = append(dates, d)
		}
	}
	slices.Sort(dates)
	slices.Reverse(dates)
	return dates, nil
}

// Prune deletes every snapshot file in the bucket older than the keep newest finished
// snapshots, including the leftovers of a run that never finished. A day newer than the
// oldest kept one is left alone: it may be a run in progress. Only objects under catalogue/
// are touched.
func Prune(ctx context.Context, b blobstore.Store, keep int) (deleted int, err error) {
	dates, err := finishedDates(ctx, b)
	if err != nil || keep < 1 || len(dates) < keep {
		return 0, err
	}
	oldestKept := dates[keep-1]
	keys, err := b.List(ctx, prefix)
	if err != nil {
		return 0, err
	}
	var expired []string
	for _, k := range keys {
		if d := dateOf(k); d != "" && d < oldestKept {
			expired = append(expired, k)
		}
	}
	// Manifests first: a listed snapshot must never point at a deleted data file.
	slices.SortStableFunc(expired, func(x, y string) int {
		return cmp.Compare(btoi(y == ManifestKey(dateOf(y))), btoi(x == ManifestKey(dateOf(x))))
	})
	for _, k := range expired {
		if err := b.Delete(ctx, k); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func btoi(v bool) int {
	if v {
		return 1
	}
	return 0
}
