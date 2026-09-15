// Command search-ping announces the newest job pages to the external search engines
// that accept being told, instead of waiting to be crawled. One run sends one batch per
// configured engine and exits; schedule it hourly.
//
//	./search-ping                 # announce
//	./search-ping -dry-run        # print what would be sent, send nothing
//
// It exists because waiting does not work at this catalogue's size. Measured 2026-09-14
// on prod: Googlebot fetched 65 distinct job pages in a day against ~690k in the
// sitemap, and Search Console's URL Inspection answers "URL is unknown to Google" for
// postings sampled from it — pages Google has never once fetched, while the site
// publishes ~14k new technical postings a day.
//
// An engine with no credentials is not configured and is skipped without error, like
// the rest of this fleet; with none configured the run is a clean no-op that never
// opens the pool. SEARCH_PING_BATCH (default 200) bounds one run per engine, and for
// Google it is additionally bounded by what is left of the day's quota — which this
// worker reads from its own ledger, because the API offers no way to ask.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/searchping"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

var dryRun = flag.Bool("dry-run", false, "print the URLs each engine would be sent, send nothing")

// The HTTP timeout for one announcement. Generous: both APIs are somebody else's and a
// slow answer is not a wrong one, while a run that gives up early leaves budget unspent
// until the next hour.
const requestTimeout = 30 * time.Second

func main() {
	flag.Parse()
	worker.Main(run)
}

func run() int {
	batch, err := worker.EnvInt32("SEARCH_PING_BATCH", 200)
	if err != nil {
		log.Printf("search-ping: %v", err)
		return 1
	}

	// Gated BEFORE Bootstrap, like discord-sync and billing-sync: with no engine
	// configured there is nothing to announce, and an unconfigured deployment must run
	// without touching the database at all. This is what makes the rollback — clear the
	// two credentials — leave a green timer on a host that has no DATABASE_URL, which is
	// what this worker's unit, its entry in AGENTS.md and its pull request all promise.
	//
	// The gate reads the environment and nothing else. Building the engines needs the
	// signal-bound context Bootstrap has not created yet, so it waits until below.
	cfg := config.Load()
	if !anyEngineConfigured() {
		log.Printf("search-ping: no engine configured, nothing to do")
		return 0
	}

	// After the gate, not before it: an origin nobody can fetch matters only once there
	// is something to announce, and refusing on it first would turn an unconfigured
	// developer checkout into an hourly red unit.
	//
	// Every URL this worker sends is rooted at the origin, and a search engine told a
	// localhost address learns nothing and spends a call doing it. Unlike the rest of
	// the fleet this is not a local oddity but a wasted slice of a 200-a-day budget, so
	// an origin that is not the public site refuses the run rather than proceeding.
	if !*dryRun && !strings.HasPrefix(cfg.FrontendOrigin, "https://") {
		log.Printf("search-ping: FRONTEND_ORIGIN is %q, which would announce URLs nobody can fetch; refusing", cfg.FrontendOrigin)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Built with ctx, not context.Background(): Google's client refreshes its token on
	// that context, so a SIGTERM mid-refresh should end the run rather than wait out the
	// HTTP timeout. That is the whole reason this sits below Bootstrap and the gate above
	// reads only the environment.
	engines, err := configuredEngines(ctx, cfg.FrontendOrigin)
	if err != nil {
		log.Printf("search-ping: %v", err)
		return 1
	}
	if len(engines) == 0 {
		// Unreachable past the gate, and checked anyway: the day the two disagree, this
		// is a clean exit rather than a runner with nothing to drive.
		log.Printf("search-ping: no engine configured, nothing to do")
		return 0
	}

	runner := searchping.New(searchping.NewPostgresRepository(db.New(pool)), cfg.FrontendOrigin, engines...)

	if *dryRun {
		return report(runner.Preview(ctx, int(batch)))
	}
	return report(runner.Run(ctx, int(batch)))
}

// anyEngineConfigured reports whether this deployment has been given anything to
// announce with. It reads the environment and nothing else — no file, no network, no
// database — because it runs before Bootstrap, so that a host with neither credential
// never opens a pool it has no use for.
func anyEngineConfigured() bool {
	return strings.TrimSpace(os.Getenv("GOOGLE_INDEXING_KEY_FILE")) != "" ||
		strings.TrimSpace(os.Getenv("INDEXNOW_KEY")) != ""
}

// configuredEngines builds the engines whose configuration is present.
//
// A missing credential is not an error: it is how an engine ships turned off and how it
// is rolled back. A credential that is present but unusable IS an error — the difference
// between "not configured" and "misconfigured" is the whole point of checking.
func configuredEngines(ctx context.Context, origin string) ([]searchping.Engine, error) {
	var engines []searchping.Engine

	budget, err := worker.EnvInt32("GOOGLE_INDEXING_DAILY_BUDGET", 200)
	if err != nil {
		return nil, err
	}
	google, err := searchping.NewGoogleEngine(ctx, os.Getenv("GOOGLE_INDEXING_KEY_FILE"), int(budget), requestTimeout)
	if err != nil {
		return nil, err
	}
	if google != nil {
		engines = append(engines, google)
	}

	indexNow, err := searchping.NewIndexNowEngine(origin, os.Getenv("INDEXNOW_KEY"), requestTimeout)
	if err != nil {
		return nil, err
	}
	if indexNow != nil {
		// Checked once here rather than per send: IndexNow answers a key mismatch with a
		// 403 on every submission, which reads like a broken integration instead of two
		// copies of a public key that drifted apart.
		if err := indexNow.VerifyKey(ctx); err != nil {
			return nil, err
		}
		engines = append(engines, indexNow)
	}

	return engines, nil
}

// report logs one line per engine and decides the exit code. An engine that failed is
// worth a non-zero exit — unlike the permanently-red ingest units, this worker runs
// against two stable APIs, so a failure here is news.
func report(reports []searchping.Report) int {
	code := 0
	for _, r := range reports {
		// engine and event both, always: the passes share one allowance, so a line that
		// named only the engine would leave "which one spent the day" unanswerable.
		who := fmt.Sprintf("%s/%s", r.Engine, r.Kind)
		switch {
		case r.Err != nil:
			log.Printf("search-ping: %s offered=%d accepted=%d recorded=%d: %v",
				who, r.Offered, r.Accepted, r.Recorded, r.Err)
			code = 1
		case len(r.URLs) > 0:
			log.Printf("search-ping: %s would announce %d url(s), remaining=%s", who, len(r.URLs), budgetWord(r.Remaining))
			for _, u := range r.URLs {
				log.Printf("  %s", u)
			}
		case r.Offered == 0:
			log.Printf("search-ping: %s nothing to announce (remaining today: %s)", who, budgetWord(r.Remaining))
		default:
			log.Printf("search-ping: %s offered=%d accepted=%d recorded=%d remaining=%s",
				who, r.Offered, r.Accepted, r.Recorded, budgetWord(r.Remaining))
		}
	}
	return code
}

// budgetWord distinguishes "no budget left" from "no budget at all" — 0 and unbounded
// are the two ends of the same field, and printing 0 for both would hide an engine that
// has spent its day.
func budgetWord(remaining int) string {
	if remaining < 0 {
		return "unbounded"
	}
	return strconv.Itoa(remaining)
}
