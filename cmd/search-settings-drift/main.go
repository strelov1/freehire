// Command search-settings-drift checks whether the LIVE Meilisearch jobs and companies
// indexes carry every sortable attribute, filterable attribute, and embedder the
// deployed binary may ask for, and publishes what it finds as a Prometheus gauge
// through the node_exporter textfile collector. Schedule it every few minutes.
//
// It exists because nothing checked this. A binary rolled out before its settings patch
// reaches the live index turns every request for the new attribute or embedder into a
// Meili 400 (a 500 for callers), for every request using that sort, filter or match
// ranking, since a settings patch replaces the attribute lists wholesale
// (internal/search/search/AGENTS.md). This worker does not fix the ordering; it makes the
// drift visible on a schedule.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// textfileName is the collector file this worker owns. Like queue-metrics and llm-probe it
// does NOT follow the name-after-the-binary convention: worker.Main writes the run outcome
// to <binary>.prom AFTER run() returns and would overwrite this gauge.
const textfileName = "freehire-search-settings.prom"

func main() { worker.Main(run) }

func run() int {
	// Gate before touching anything, as queue-metrics and llm-probe do: with nowhere to
	// publish there is nothing worth a Meili round trip.
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

	// A gap is what this worker exists to report, not its failure: exiting non-zero would
	// make a red unit mean both "the thing I watch is broken" and "I am broken" (see
	// llm-probe).
	return 0
}
