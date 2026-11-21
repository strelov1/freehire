// Command search-ping announces the newest job pages to the external search engines
// that accept being told, instead of waiting to be crawled. One run sends one batch per
// configured engine and event and exits; schedule it hourly. Every posting the search
// index holds is eligible, technical or not; Google's scarce daily quota goes to the
// technical ones first.
//
//	./search-ping                 # announce
//	./search-ping -dry-run        # print what would be sent, send nothing
//
// It exists because waiting does not work at this catalogue's size: Google fetches far
// fewer job pages a day than the site publishes.
//
// An engine with no credentials is skipped without error; with none configured the run is
// a clean no-op that never opens the pool. SEARCH_PING_BATCH (default 200) bounds one run
// per engine, and for Google it is also bounded by what is left of the day's quota, which
// this worker reads from its own ledger because the API offers no way to ask.
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

// The HTTP timeout for one announcement. Generous: both APIs are somebody else's, and
// giving up early leaves budget unspent until the next hour.
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

	// Gated BEFORE Bootstrap, like billing-sync: with no engine configured
	// an unconfigured deployment must run without touching the database, so clearing the
	// credentials leaves a green timer on a host with no DATABASE_URL.
	//
	// The gate reads the environment only; building the engines needs the signal-bound
	// context Bootstrap creates, so it waits until below.
	cfg := config.Load()
	if !anyEngineConfigured() {
		log.Printf("search-ping: no engine configured, nothing to do")
		return worker.Skip("GOOGLE_INDEXING_KEY_FILE+INDEXNOW_KEY")
	}

	// After the gate, so an unconfigured developer checkout is not an hourly red unit.
	//
	// Every URL sent is rooted at the origin, and an engine told a localhost address spends
	// a slice of a small daily budget learning nothing, so a non-public origin refuses the
	// run.
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

	// Built with ctx, not context.Background(): Google's client refreshes its token on that
	// context, so a SIGTERM mid-refresh ends the run instead of waiting out the HTTP
	// timeout. That is why this sits below Bootstrap.
	engines, err := configuredEngines(ctx, cfg.FrontendOrigin)
	if err != nil {
		log.Printf("search-ping: %v", err)
		return 1
	}
	if len(engines) == 0 {
		// Unreachable past the gate; a clean exit if the two ever disagree.
		log.Printf("search-ping: no engine configured, nothing to do")
		return 0
	}

	runner := searchping.New(searchping.NewPostgresRepository(db.New(pool)), cfg.FrontendOrigin, engines...)

	if *dryRun {
		return report(runner.Preview(ctx, int(batch)))
	}
	return report(runner.Run(ctx, int(batch)))
}

// anyEngineConfigured reports whether this deployment has anything to announce with. It
// reads only the environment, because it runs before Bootstrap so a host with no
// credential never opens a pool.
func anyEngineConfigured() bool {
	return strings.TrimSpace(os.Getenv("GOOGLE_INDEXING_KEY_FILE")) != "" ||
		strings.TrimSpace(os.Getenv("INDEXNOW_KEY")) != ""
}

// configuredEngines builds the engines whose configuration is present. A missing
// credential is not an error (it is how an engine ships off and is rolled back); a present
// but unusable one IS.
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
		// Checked once, not per send: IndexNow answers a key mismatch with a 403 on every
		// submission, which looks like a broken integration rather than a drifted key.
		if err := indexNow.VerifyKey(ctx); err != nil {
			return nil, err
		}
		engines = append(engines, indexNow)
	}

	return engines, nil
}

// report logs one line per engine and decides the exit code. A failed engine is worth a
// non-zero exit: this worker runs against two stable APIs, so a failure is news.
func report(reports []searchping.Report) int {
	code := 0
	for _, r := range reports {
		// Name engine and event both: the passes share one allowance.
		who := fmt.Sprintf("%s/%s", r.Engine, r.Kind)
		if r.Tier != "" {
			who += "/" + string(r.Tier)
		}
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

// budgetWord distinguishes "no budget left" from "no budget at all": 0 and unbounded are
// the two ends of one field, and printing 0 for both would hide an engine that spent its day.
func budgetWord(remaining int) string {
	if remaining < 0 {
		return "unbounded"
	}
	return strconv.Itoa(remaining)
}
