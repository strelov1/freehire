// Command ingest-scheduler decides which provider crawls next and starts it.
//
// It replaces deploy/bin/gen-ingest-timers.sh and the ~279 static systemd units that
// script materialised. Those units named each provider a SECOND time, in a filename, and
// nothing reconciled that name against the boards table — so a provider could be crawled
// under a name no adapter answered to (habr_career, silent for a day) or kept crawling
// after its last board was retired (careerspage, empty since 18 July). Here the unit name
// is built from the same row that selects the boards, so there is no second spelling left
// to drift.
//
// Run it once a minute. It claims what is due within the fleet's free capacity, starts each
// run as a transient systemd unit, and exits — a one-shot like every other worker in cmd/,
// so it cannot stack on itself and a crash costs one minute rather than the fleet.
//
// It ships in SHADOW MODE: without INGEST_SCHEDULER_APPLY=1 it reports what it would have
// launched and launches nothing, so the first deployment cannot disturb a fleet the static
// timers still drive.
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

	// Single-flight across PROCESSES, not just across timer firings. The fleet's
	// concurrency cap is a check-then-act pair — count what is in flight, then claim
	// `cap - that` — and two overlapping invocations would each read zero and each claim
	// the whole cap, running 2x the ceiling the host is tuned for. `Type=oneshot` stops the
	// timer stacking on itself but says nothing about a hand-run invocation beside it, and
	// the flock semaphore this replaces WAS atomic. A run that cannot take the lock exits
	// cleanly: the next minute's tick loses nothing, because all state is in the database.
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
		Grace:    cfg.Grace,
		Apply:    cfg.Apply,
	}

	result, err := scheduler.Tick(ctx)
	report(result, cfg.Apply)
	if err != nil {
		log.Printf("scheduler: %v", err)
		return 1
	}

	// A run that could not be started is a real failure and must reach cron's alerting.
	// A refused provider key is too: it means the catalog holds a provider nothing can
	// crawl, which is precisely the condition that used to be silent.
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
		"would_launch=%d disabled=%d unmanaged=%d refused=%d failed=%d saturated=%t",
		mode, r.Eligible, r.Tracked, r.InFlight, r.Reaped, len(r.Launched),
		len(r.WouldLaunch), len(r.Disabled), len(r.Unmanaged), len(r.Refused), len(r.Failed),
		r.Saturated)

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
	// Unmanaged is one line, not one per provider: during cutover it holds ~226 names and
	// printing each would bury everything above it every single minute.
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
