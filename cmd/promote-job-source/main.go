// Command promote-job-source transfers a verified LinkedIn discovery posting to
// its native ATS identity and onboards that board for normal ingestion.
package main

import (
	"context"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ingest/boardresolve"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	slug := flag.String("slug", "", "existing public posting slug")
	raw := flag.String("url", "", "verified ATS posting URL; defaults to the stored application destination")
	website := flag.String("company-website", "", "employer's published website, used to verify a new ATS board independently")
	apply := flag.Bool("apply", false, "transfer identity and onboard the verified ATS board; otherwise preview")
	flag.Parse()
	if strings.TrimSpace(*slug) == "" || flag.NArg() != 0 {
		log.Print("promote-job-source: --slug is required")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ctx, _, pool, cleanup, err := worker.Bootstrap(ctx)
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	q := db.New(pool)
	row, err := q.GetJobBySlug(ctx, *slug)
	if err != nil || row.IsPrivate || row.ClosedAt.Valid || row.DuplicateOf.Valid {
		log.Printf("promote-job-source: posting must be open, public and canonical: %v", err)
		return 1
	}
	*raw = destinationURL(row, *raw)
	client := sources.NewClient()
	registry := sources.All(client)
	if err := sources.ApplyProxyEgress(registry); err != nil {
		log.Printf("promote-job-source: %v", err)
		return 1
	}
	if err := sources.ApplyScraplingEgress(registry); err != nil {
		log.Printf("promote-job-source: %v", err)
		return 1
	}
	im := linkimport.New(pool, q, nil, client, registry, boardresolve.New())
	native, err := im.ResolveNative(ctx, row, *raw, *website)
	if err != nil {
		log.Printf("promote-job-source: resolve native destination: %v", err)
		return 1
	}
	if row.Source == native.Entry.Provider && row.ExternalID == native.Job.ExternalID && row.URL == native.Job.URL && row.ApplicationUrl == "" {
		log.Print("promote-job-source: already owned by the native ATS")
		return 0
	}
	if row.Source != "linkedin" {
		log.Print("promote-job-source: only LinkedIn discovery postings may be transferred")
		return 2
	}
	log.Printf("posting=%s transfer=%s/%s -> %s/%s board=%s url=%s", row.PublicSlug, row.Source, row.ExternalID, native.Entry.Provider, native.Job.ExternalID, native.Entry.Board, native.Job.URL)
	if !*apply {
		log.Print("promote-job-source: dry run; pass --apply to transfer and onboard")
		return 0
	}
	if _, err := im.WriteNative(ctx, &row, native); err != nil {
		log.Printf("promote-job-source: transfer refused: %v", err)
		return 1
	}
	log.Print("promote-job-source: native identity saved, board onboarded, search queued; run normal native ingest and ensure its provider is scheduled")
	return 0
}

func destinationURL(row db.Job, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if row.Source != "linkedin" && row.ApplicationUrl == "" {
		return row.URL
	}
	return row.ApplicationUrl
}
