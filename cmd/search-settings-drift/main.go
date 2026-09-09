// Command search-settings-drift checks whether the LIVE Meilisearch jobs and companies
// indexes carry every sortable attribute, filterable attribute, and embedder the
// deployed binary may ask for, and publishes what it finds as a Prometheus gauge
// through the node_exporter textfile collector. Schedule it every few minutes.
//
// It exists because nothing checked this. internal/search/search/AGENTS.md documents
// the hazard by hand, for both indexes: a binary rolled out before its settings patch
// reaches the live index turns every request for the new attribute or embedder into a
// Meili 400, which this package's error mapping turns into a 500 for every caller of
// the affected sort, filter, or the match ranking — not only the request that named the
// new one, since a settings patch replaces sortableAttributes/filterableAttributes
// wholesale. "There is no operator script for this" is that doc's own words for the
// gap this worker closes: it does not fix the ordering (deploy settings before binary
// stays a human's job, in either order the app or ops repo's own runbook decides), it
// makes the drift visible on a schedule instead of by a caller hitting the 500 first.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// textfileName is the collector file this worker owns. Like queue-metrics and
// llm-probe it deliberately does NOT follow the name-after-the-binary convention:
// worker.Main writes the run outcome to <binary>.prom AFTER run() returns, so
// publishing here under "search-settings-drift" would have every run emit this gauge
// and then immediately overwrite it.
const textfileName = "freehire-search-settings.prom"

func main() { worker.Main(run) }

func run() int {
	// Gate before touching anything, the same order queue-metrics and llm-probe follow:
	// with nowhere to publish there is nothing worth a Meili round trip for.
	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" {
		log.Printf("search-settings-drift: %s is unset, nothing to publish", worker.PromTextfileDirEnv)
		return 0
	}

	cfg := config.Load()
	if cfg.MeiliKey == "" {
		log.Print("search-settings-drift: MEILI_MASTER_KEY is unset, nothing to check")
		return 0
	}

	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	drift, err := client.SettingsDrift(context.Background())
	if err != nil {
		log.Printf("search-settings-drift: %v", err)
		return 1
	}

	if err := worker.WriteTextfile(dir, textfileName, render(drift)); err != nil {
		log.Printf("search-settings-drift: %v", err)
		return 1
	}

	if len(drift) == 0 {
		log.Print("search-settings-drift: live settings match what this binary expects")
	} else {
		log.Printf("search-settings-drift: %d gap(s) between what this binary expects and what the live index declares:", len(drift))
		for _, d := range drift {
			log.Printf("search-settings-drift:   %s", d)
		}
	}

	// A gap here is not this worker's failure — it is the exact thing it exists to
	// report. Exiting non-zero for a gap it found would paint the unit red for
	// something it only observed, the same reasoning llm-probe documents for a failing
	// alias: a red unit that means "the thing I watch is broken" is indistinguishable
	// from one that means "I am broken".
	return 0
}
