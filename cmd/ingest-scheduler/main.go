// Command ingest-scheduler decides which provider crawls next and starts it.
//
// The unit name is built from the same boards row that selects the boards, so the provider
// name has no second spelling to drift from the catalog.
//
// Run it once a minute. It claims what is due within the fleet's free capacity, starts each
// run as a transient systemd unit, and exits, so it cannot stack on itself.
//
// SHADOW MODE: without INGEST_SCHEDULER_APPLY=1 it reports what it would have launched and
// launches nothing.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/ingestsched"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// lockKey serializes scheduler ticks across processes. The project's advisory-lock key
// list lives in internal/platform/migrate.
const lockKey = 0x66687363 // "fhsc" — freehire scheduler

func main() {
	worker.Main(run)
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Single-flight across PROCESSES: the concurrency cap is check-then-act (count in flight,
	// claim `cap - that`), so two overlapping runs would each claim the whole cap. Type=oneshot
	// does not stop a hand-run invocation. A run that cannot take the lock exits cleanly; all
	// state is in the database.
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("acquire lock connection: %v", err)
		return 1
	}
	defer lockConn.Release()

	var locked bool
	if err := lockConn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", int64(lockKey)).Scan(&locked); err != nil {
		log.Printf("scheduler lock: %v", err)
		return 1
	}
	if !locked {
		log.Print("scheduler: another tick holds the lock; skipping this one")
		return 0
	}
	defer func() {
		if _, err := lockConn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", int64(lockKey)); err != nil {
			log.Printf("scheduler unlock: %v", err)
		}
	}()

	cfg := config.LoadIngestScheduler()
	scheduler := ingestsched.Scheduler{
		Repo:     ingestsched.NewQueriesRepository(db.New(pool)),
		Launcher: ingestsched.NewSystemdLauncher(cfg.IngestBinary, cfg.WorkingDir, cfg.EnvFile, cfg.RunAs),
		Cap:      cfg.Cap,
		HeavyCap: cfg.HeavyCap,
		Grace:    cfg.Grace,
		Apply:    cfg.Apply,
		// The launched crawls are the `ingest` binary, so that job's switch decides.
		Paused: func(ctx context.Context) bool {
			return worker.Paused(ctx, config.Load().RedisURL, "ingest")
		},
	}

	result, err := scheduler.Tick(ctx)
	report(result, cfg.Apply)
	if err != nil {
		log.Printf("scheduler: %v", err)
		return 1
	}

	// A run that could not start must reach cron's alerting. So must a refused provider key:
	// the catalog holds a provider nothing can crawl.
	if len(result.Failed) > 0 || len(result.Refused) > 0 {
		return 1
	}
	return 0
}

// report prints one line per decision. Everything the tick chose NOT to do is named,
// because a fleet that quietly stops crawling looks identical to a healthy one.
func report(r ingestsched.TickResult, apply bool) {
	mode := "shadow"
	if apply {
		mode = "apply"
	}
	log.Printf("scheduler tick: mode=%s eligible=%d tracked=%d in_flight=%d reaped=%d launched=%d "+
		"would_launch=%d disabled=%d unmanaged=%d refused=%d failed=%d saturated=%t "+
		"paused=%t heavy_cap=%d light_cap=%d",
		mode, r.Eligible, r.Tracked, r.InFlight, r.Reaped, len(r.Launched),
		len(r.WouldLaunch), len(r.Disabled), len(r.Unmanaged), len(r.Refused), len(r.Failed),
		r.Saturated, r.Paused, r.Heavy.Cap, r.Light.Cap)

	// A pool with ZERO slots launches nothing while the line above reads healthy: launched=0
	// looks like "nothing was due" and saturated stays false. Say it in words.
	if r.Heavy.Cap == 0 {
		log.Printf("scheduler: HEAVY POOL STARVED — 0 of %d slots reserved for it; no sharded provider can be launched. Raise INGEST_SCHEDULER_HEAVY_CAP or INGEST_SCHEDULER_CAP",
			r.Heavy.Cap+r.Light.Cap)
	}
	if r.Light.Cap == 0 {
		log.Printf("scheduler: LIGHT POOL STARVED — the heavy reservation holds all %d slots; no unsharded provider can be launched. Raise INGEST_SCHEDULER_CAP or lower INGEST_SCHEDULER_HEAVY_CAP",
			r.Heavy.Cap+r.Light.Cap)
	}

	if r.Paused {
		log.Print("scheduler: ingest is paused; nothing claimed, every due run stays due for the next tick")
	}
	if r.Saturated {
		log.Printf("scheduler: fleet saturated at %d in flight; every due run stays claimable for the next tick", r.InFlight)
	}
	for _, run := range r.Launched {
		log.Printf("scheduler: launched %s", runLabel(run))
	}
	for _, run := range r.WouldLaunch {
		log.Printf("scheduler: would launch %s", runLabel(run))
	}
	for _, s := range r.Refused {
		log.Printf("scheduler: REFUSED %s: %s", s.Provider, s.Reason)
	}
	for _, s := range r.Failed {
		log.Printf("scheduler: FAILED to launch %s: %s", s.Provider, s.Reason)
	}
	for _, s := range r.Disabled {
		log.Printf("scheduler: disabled %s: %s", s.Provider, s.Reason)
	}
	// Unmanaged is one line, not one per provider: printing each would bury the rest every minute.
	if len(r.Unmanaged) > 0 {
		log.Printf("scheduler: %d providers still owned by their static timer: %s",
			len(r.Unmanaged), strings.Join(r.Unmanaged, " "))
	}
}

func runLabel(run ingestsched.Run) string {
	if run.Shards <= 1 {
		return run.Provider
	}
	return fmt.Sprintf("%s shard %d/%d", run.Provider, run.Shard, run.Shards)
}
