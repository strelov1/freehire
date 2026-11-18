// Command backfill-telegram-prefilter re-offers the Telegram posts the crawl's prefilter
// declined, for when its markers have since widened.
//
// A declined post is stamped extracted_at by InsertTelegramPost at INSERT time and nothing
// revisits it, so widening the markers reaches NEW posts only. Run this after any marker change.
//
// It reports by default and writes only under --apply, like cmd/merge-companies: the markers are
// hand-maintained, so read the report first. It names the channels it would reach, which is how
// a widening that admits an unintended channel becomes visible.
//
// It decides with telegram.AdmitsPost, the function the crawl calls, so it cannot requeue what
// the next crawl would refuse. It needs no reindex: a requeued post re-enters the extraction
// queue and reaches the catalogue through cmd/tg-extract's ordinary write path.
//
// Idempotent and free to interrupt: the UPDATE repeats the read's predicate, so a post already
// requeued, or claimed by the extractor since the read, matches nothing. A bounded run prints
// the key to continue from. Needs only DATABASE_URL.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"sort"

	"github.com/strelov1/freehire/internal/ingest/linksource"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/ingest/telegram"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	apply := flag.Bool("apply", false, "actually requeue; without it the run only reports what it would")
	flag.Parse()

	// Unset is unbounded and 0 is refused, the strict-reader rule every one-off pass follows.
	maxScan, err := worker.EnvInt64("BACKFILL_TG_PREFILTER_MAX", 0)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	// Where a previous bounded run stopped, EXCLUSIVE. Without it the bound is a trap: a
	// refused post never leaves the predicate, so a second run from the top rescans what the
	// first rejected.
	afterChannel := os.Getenv("BACKFILL_TG_PREFILTER_AFTER_CHANNEL")
	afterMsgID, err := worker.EnvInt64("BACKFILL_TG_PREFILTER_AFTER_MSG_ID", 0)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	// Half a cursor silently starts from the wrong place (a channel with no message id re-reads
	// that channel whole; an id with no channel applies to the first channel alphabetically).
	if (afterChannel == "") != (afterMsgID == 0) {
		log.Printf("config: BACKFILL_TG_PREFILTER_AFTER_CHANNEL and _AFTER_MSG_ID are one cursor — set both or neither")
		return 1
	}

	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	runner := telegram.RefilterRunner{
		Store: &refilterStore{q: db.New(pool)},
		// The registry cmd/tg-ingest hands its crawl, so admission here matches the crawl.
		Links: linkMatcher{reg: linksource.All(sources.NewClient())},
		// The same judge the crawl uses.
		Judge:        telegram.JudgeFromKey(cfg.TypesafeAPIKey),
		Apply:        *apply,
		Max:          maxScan,
		AfterChannel: afterChannel,
		AfterMsgID:   afterMsgID,
	}

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("refilter: %v", err)
		return 1
	}

	mode := "dry-run (nothing written; pass --apply to requeue)"
	if *apply {
		mode = "applied"
	}
	log.Printf("backfill-telegram-prefilter %s: scanned=%d admitted=%d requeued=%d",
		mode, stats.Scanned, stats.Admitted, stats.Requeued)

	channels := make([]string, 0, len(stats.ByChannel))
	for ch := range stats.ByChannel {
		channels = append(channels, ch)
	}
	sort.Slice(channels, func(i, j int) bool {
		if stats.ByChannel[channels[i]] != stats.ByChannel[channels[j]] {
			return stats.ByChannel[channels[i]] > stats.ByChannel[channels[j]]
		}
		return channels[i] < channels[j]
	})
	for _, ch := range channels {
		log.Printf("  channel %s: %d", ch, stats.ByChannel[ch])
	}

	// One report on every exit path, saying whether there is more; never inferred from a row
	// count (see cmd/backfill-derive).
	if stats.Stopped {
		log.Printf("stopped on BACKFILL_TG_PREFILTER_MAX; continue with "+
			"BACKFILL_TG_PREFILTER_AFTER_CHANNEL=%s BACKFILL_TG_PREFILTER_AFTER_MSG_ID=%d",
			stats.NextChannel, stats.NextMsgID)
	} else {
		log.Printf("done: reached the end of the declined posts")
	}
	return 0
}

// refilterStore adapts the generated queries to telegram.RefilterStore.
type refilterStore struct {
	q *db.Queries
}

func (s *refilterStore) ListRejected(ctx context.Context, afterChannel string, afterMsgID int64, limit int32) ([]telegram.StoredPost, error) {
	rows, err := s.q.ListPrefilterRejectedTelegramPosts(ctx, db.ListPrefilterRejectedTelegramPostsParams{
		AfterChannel: afterChannel,
		AfterMsgID:   afterMsgID,
		BatchSize:    limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]telegram.StoredPost, 0, len(rows))
	for _, r := range rows {
		p := telegram.StoredPost{Channel: r.Channel, MsgID: r.MsgID, Text: r.Text}
		// An undecodable links column is not worth failing the run: the text still decides.
		if len(r.Links) > 0 {
			if err := json.Unmarshal(r.Links, &p.Links); err != nil {
				log.Printf("telegram refilter: %s/%d has unreadable links, deciding on text alone: %v", r.Channel, r.MsgID, err)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *refilterStore) Requeue(ctx context.Context, channel string, msgID int64) (int64, error) {
	return s.q.RequeueTelegramPost(ctx, db.RequeueTelegramPostParams{Channel: channel, MsgID: msgID})
}

var _ telegram.RefilterStore = (*refilterStore)(nil)

// linkMatcher adapts the linksource registry to telegram.LinkMatcher. cmd/tg-ingest carries
// the same four lines, beside the binary that constructs the registry, rather than in a shared
// package neither block owns.
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
