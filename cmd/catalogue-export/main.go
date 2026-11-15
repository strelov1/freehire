// Command catalogue-export writes today's snapshot of the open, canonical, public catalogue
// to the export bucket as gzip JSON Lines plus a manifest, then prunes old snapshots. It is a
// new binary, not part of reindex: that worker is bound to Meilisearch and holds the heavy
// index lock for hours, while this one needs neither. It reads the same rows the search index
// admits (ListIndexableJobsByIDAfter plus search.Withheld) in the public job shape, from the
// analytics replica when one is configured. Cadence is whatever the ops timer says. Without
// the CATALOGUE_EXPORT_S3_* settings it refuses to run.
package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/catalogexport"
	"github.com/strelov1/freehire/internal/search/search"
)

const batchSize = 1000

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	bucket, err := blobstore.NewBucket(blobstore.Config{
		Endpoint: cfg.ExportS3Endpoint, Bucket: cfg.ExportS3Bucket,
		AccessKey: cfg.ExportS3AccessKey, SecretKey: cfg.ExportS3SecretKey, Region: cfg.ExportS3Region,
	})
	if err != nil {
		log.Printf("catalogue-export: %v", err)
		return 1
	}
	if bucket == nil {
		log.Print("catalogue-export: CATALOGUE_EXPORT_S3_* is not configured, refusing to run")
		return 1
	}
	analytics, err := worker.AnalyticsPool(ctx, pool)
	if err != nil {
		log.Printf("analytics database: %v", err)
		return 1
	}
	defer analytics.Close()

	now := time.Now()
	m, err := catalogexport.Write(ctx, bucket, catalogexport.Day(now), now, batches(db.New(analytics), now))
	if err != nil {
		log.Printf("catalogue-export: %v", err)
		return 1
	}
	log.Printf("catalogue-export: %s: %d rows, %d bytes, sha256 %s", m.Date, m.Rows, m.Bytes, m.SHA256)

	n, err := catalogexport.Prune(ctx, bucket, cfg.ExportKeep)
	if err != nil {
		log.Printf("catalogue-export: prune: %v", err)
		return 1
	}
	log.Printf("catalogue-export: pruned %d old objects (keeping %d snapshots)", n, cfg.ExportKeep)
	return 0
}

// batches pages the rows search would index, by primary key, and renders each as the public
// job shape. Withheld is the index's own filter, so the file lists what a visitor can search.
func batches(q *db.Queries, now time.Time) catalogexport.Batch {
	unseen := search.UnseenWindows(sources.UnseenWindows(sources.Taxonomy(), search.UnseenWindow))
	return func(ctx context.Context, after int64) ([][]byte, int64, error) {
		// A page the filter empties is not the end of the catalogue: read on until a page
		// has lines or the scan runs out.
		for {
			rows, err := q.ListIndexableJobsByIDAfter(ctx, db.ListIndexableJobsByIDAfterParams{AfterID: after, BatchSize: batchSize})
			if err != nil || len(rows) == 0 {
				return nil, after, err
			}
			after = rows[len(rows)-1].ID
			var lines [][]byte
			for _, r := range rows {
				if search.Withheld(r, now, unseen) {
					continue
				}
				v, err := jobview.FromRow(r)
				if err != nil {
					return nil, 0, err
				}
				line, err := json.Marshal(v)
				if err != nil {
					return nil, 0, err
				}
				lines = append(lines, line)
			}
			if len(lines) > 0 {
				return lines, after, nil
			}
		}
	}
}
