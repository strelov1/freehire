// Command set-job-application-url records a verified employer application destination
// without changing the source posting used for identity and liveness. Preview is the default.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	slug := flag.String("slug", "", "public slug of the posting to update")
	raw := flag.String("url", "", "verified employer application URL")
	clear := flag.Bool("clear", false, "remove the destination and use the source posting again")
	apply := flag.Bool("apply", false, "write the change and queue search; otherwise only preview")
	flag.Parse()
	destination, err := applicationURL(*raw, *clear)
	if err != nil || strings.TrimSpace(*slug) == "" || flag.NArg() != 0 {
		log.Printf("set-job-application-url: --slug and either --url or --clear are required: %v", err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx, _, pool, cleanup, err := worker.Bootstrap(ctx)
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	q := db.New(pool)
	row, err := q.GetJobBySlug(ctx, *slug)
	if err != nil {
		log.Printf("read posting: %v", err)
		return 1
	}
	if row.IsPrivate || job.IsDirectApply(row.Source, row.ExternalID) {
		log.Print("set-job-application-url: private and on-site application postings cannot link out")
		return 2
	}
	log.Printf("posting=%s source=%s source_url=%q application_url=%q proposed=%q", row.PublicSlug, row.Source, row.URL, row.ApplicationUrl, destination)
	if row.ApplicationUrl == destination {
		log.Print("set-job-application-url: already configured")
		return 0
	}
	if !*apply {
		log.Print("set-job-application-url: dry run; pass --apply to write")
		return 0
	}
	_, err = q.SetJobApplicationURL(ctx, db.SetJobApplicationURLParams{
		JobID: row.ID, ExpectedApplicationUrl: row.ApplicationUrl, ApplicationUrl: destination,
	})
	if err != nil {
		log.Printf("set-job-application-url: write failed (a concurrent edit may have changed the destination): %v", err)
		return 1
	}
	log.Print("set-job-application-url: destination saved and search queued")
	return 0
}

func applicationURL(raw string, clear bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if clear && raw == "" {
		return "", nil
	}
	if clear || raw == "" {
		return "", fmt.Errorf("use either --url or --clear")
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return "", fmt.Errorf("application URL must be an absolute HTTP(S) URL without credentials")
	}
	return raw, nil
}
