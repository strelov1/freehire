// Command tg-ingest is the standalone Telegram crawl worker. It loads the
// configured channels from sources/telegram.yml, fetches each channel's latest posts from
// the public t.me web preview, prefilters obvious non-vacancies, and stores new
// posts in the telegram_posts queue for the extraction worker (cmd/tg-extract).
// Run it on a schedule (e.g. cron); it crawls every channel once and exits. It
// exits non-zero when the run finished with any failures, so cron can alert.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/linksource"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/telegram"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	os.Exit(run())
}

func run() int {
	chanCfg, err := telegram.LoadChannels()
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	runner := telegram.CrawlRunner{
		Fetcher: telegram.NewFetcher(),
		Store:   &postStore{q: db.New(pool)},
		Delay:   2 * time.Second, // polite pacing toward t.me
		Links:   linkMatcher{reg: linksource.All(sources.NewClient())},
	}

	stats, err := runner.Run(ctx, chanCfg.Channels)
	if err != nil {
		log.Printf("crawl: %v", err)
		return 1
	}
	log.Printf("tg-ingest done: stored=%d filtered=%d failed=%d",
		stats.Stored, stats.Filtered, stats.Failed)
	return worker.ExitCode(stats.Failed, 0)
}

// postStore adapts the generated queries to telegram.PostStore.
type postStore struct {
	q *db.Queries
}

func (s *postStore) Insert(ctx context.Context, channel string, p telegram.Post, done bool) (bool, error) {
	var extractedAt pgtype.Timestamptz
	if done {
		extractedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	links := []byte("[]")
	if len(p.Links) > 0 {
		b, err := json.Marshal(p.Links)
		if err != nil {
			return false, err
		}
		links = b
	}
	rows, err := s.q.InsertTelegramPost(ctx, db.InsertTelegramPostParams{
		Channel:     channel,
		MsgID:       p.MsgID,
		Text:        p.Text,
		Links:       links,
		PostedAt:    pgtype.Timestamptz{Time: p.PostedAt, Valid: true},
		ExtractedAt: extractedAt,
	})
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

var _ telegram.PostStore = (*postStore)(nil)

// linkMatcher adapts the linksource registry to telegram.LinkMatcher, so the crawl keeps
// link-out digest posts whose teaser text alone does not look like a vacancy.
type linkMatcher struct {
	reg []linksource.Source
}

func (m linkMatcher) Matches(links []telegram.Link) bool {
	urls := make([]string, len(links))
	for i, l := range links {
		urls[i] = l.URL
	}
	return linksource.MatchesAny(m.reg, urls)
}

var _ telegram.LinkMatcher = linkMatcher{}
