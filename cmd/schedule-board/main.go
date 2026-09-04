// Command schedule-board is how a curator reads and edits the ingest schedule — the
// database-backed replacement for editing the constants in deploy/bin/gen-ingest-timers.sh
// and re-running it over ssh.
//
// It reports by default and writes only under --apply, like cmd/add-board.
//
// The roster is the boards table; this command edits OVERRIDES. A provider with no row is
// scheduled on documented defaults and shows as "default" in the report. Turning a
// provider OFF requires a reason, enforced by the table itself: "nobody configured it" and
// "we decided not to crawl it" must not be the same state, because that is precisely what
// hid two dead providers in production for weeks.
//
// Usage:
//
//	go run ./cmd/schedule-board                                           # report everything
//	go run ./cmd/schedule-board --provider=paylocity --shards=24 --timeout=4500 \
//	    --notes="~10.42s/board measured" --apply
//	go run ./cmd/schedule-board --provider=reed --cadence=6h --apply
//	go run ./cmd/schedule-board --provider=bayt --disable \
//	    --reason="fingerprint client has no proxy support" --apply
//	go run ./cmd/schedule-board --provider=greenhouse --manage --apply
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/strelov1/freehire/internal/ingest/ingestsched"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// maxShards mirrors the CHECK on ingest_schedule.shards. Both exist: the constraint stops
// a hand-written psql UPDATE, this stops the round trip.
const maxShards = 64

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false, "actually write; without it the run only reports")
	provider := flag.String("provider", "", "provider key to edit; omit to report the whole schedule")
	shards := flag.Int("shards", 0, "partition the provider across N runs (0 = leave alone)")
	cadence := flag.Duration("cadence", 0, "how often the provider crawls, e.g. 3h (0 = leave alone)")
	timeout := flag.Duration("timeout", 0, "per-run budget, e.g. 4500s (0 = leave alone)")
	notes := flag.String("notes", "", "what was MEASURED to justify these numbers")
	disable := flag.Bool("disable", false, "stop scheduling this provider; requires --reason")
	enable := flag.Bool("enable", false, "resume scheduling this provider")
	reason := flag.String("reason", "", "why the provider is disabled (required with --disable)")
	manage := flag.Bool("manage", false, "hand this provider to the scheduler (rollout only)")
	unmanage := flag.Bool("unmanage", false, "hand this provider back to its static timer (rollout only)")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	repo := ingestsched.NewQueriesRepository(db.New(pool))

	if *provider == "" {
		return report(ctx, repo)
	}

	in, err := edit(*provider, editFlags{
		shards: *shards, cadence: *cadence, timeout: *timeout, notes: *notes,
		disable: *disable, enable: *enable, reason: *reason,
		manage: *manage, unmanage: *unmanage,
	})
	if err != nil {
		log.Printf("schedule-board: %v", err)
		return 2
	}

	describe(in)
	if !*apply {
		log.Print("schedule-board: dry run; pass --apply to write")
		return 0
	}
	if err := repo.SaveOverride(ctx, in); err != nil {
		log.Printf("schedule-board: %v", err)
		return 1
	}
	log.Printf("schedule-board: wrote %s", in.Provider)
	return 0
}

type editFlags struct {
	shards           int
	cadence          time.Duration
	timeout          time.Duration
	notes            string
	disable, enable  bool
	reason           string
	manage, unmanage bool
}

// edit turns the flags into a partial override. It is split from run so it can be tested
// without a database — every check here is about the flags, not about the store.
func edit(provider string, f editFlags) (ingestsched.OverrideInput, error) {
	in := ingestsched.OverrideInput{Provider: provider}

	if err := ingestsched.ValidateProviderKey(provider); err != nil {
		// Refusing here rather than at the next scheduler tick: a typo written into the
		// table would otherwise sit there being reported as refused every minute, and the
		// person who typed it would be gone.
		return in, err
	}
	if f.disable && f.enable {
		return in, fmt.Errorf("--disable and --enable are contradictory")
	}
	if f.manage && f.unmanage {
		return in, fmt.Errorf("--manage and --unmanage are contradictory")
	}
	if f.disable && strings.TrimSpace(f.reason) == "" {
		return in, fmt.Errorf("--disable requires --reason: an unexplained disable is the silence this table exists to remove")
	}
	// The table bounds this too. Refusing here as well is what turns a fat-fingered zero
	// into a message rather than a constraint name — and the constraint is the only thing
	// standing between a typo and a generate_series that outlives the scheduler's timeout
	// on every tick.
	if f.shards > maxShards {
		return in, fmt.Errorf("--shards=%d exceeds the maximum of %d; the largest real value is paylocity's 24", f.shards, maxShards)
	}

	if f.shards > 0 {
		in.Shards = &f.shards
	}
	if f.cadence > 0 {
		in.Cadence = &f.cadence
	}
	if f.timeout > 0 {
		in.RunTimeout = &f.timeout
	}
	if f.notes != "" {
		in.Notes = &f.notes
	}
	if f.reason != "" {
		in.DisabledReason = &f.reason
	}
	switch {
	case f.disable:
		off := false
		in.Enabled = &off
	case f.enable:
		on := true
		in.Enabled = &on
	}
	switch {
	case f.manage:
		on := true
		in.Managed = &on
	case f.unmanage:
		off := false
		in.Managed = &off
	}
	return in, nil
}

func describe(in ingestsched.OverrideInput) {
	var parts []string
	if in.Shards != nil {
		parts = append(parts, fmt.Sprintf("shards=%d", *in.Shards))
	}
	if in.Cadence != nil {
		parts = append(parts, "cadence="+in.Cadence.String())
	}
	if in.RunTimeout != nil {
		parts = append(parts, "timeout="+in.RunTimeout.String())
	}
	if in.Enabled != nil {
		parts = append(parts, fmt.Sprintf("enabled=%t", *in.Enabled))
	}
	if in.Managed != nil {
		parts = append(parts, fmt.Sprintf("managed=%t", *in.Managed))
	}
	if in.DisabledReason != nil {
		parts = append(parts, "reason="+*in.DisabledReason)
	}
	if in.Notes != nil {
		parts = append(parts, "notes="+*in.Notes)
	}
	if len(parts) == 0 {
		log.Printf("schedule-board: %s — nothing to change", in.Provider)
		return
	}
	log.Printf("schedule-board: %s <- %s", in.Provider, strings.Join(parts, " "))
}

func report(ctx context.Context, repo *ingestsched.QueriesRepository) int {
	rows, err := repo.Report(ctx)
	if err != nil {
		log.Printf("schedule-board: %v", err)
		return 1
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	// A tabwriter buffers until Flush, so these writes cannot fail on their own; the
	// Flush below is where a real write error surfaces, and it IS checked.
	_, _ = fmt.Fprintln(w, "PROVIDER\tSOURCE\tSHARDS\tCADENCE\tTIMEOUT\tSTATE\tDUE\tLAST RUN\tNOTE")
	for _, r := range rows {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Provider, source(r), shardsColumn(r), r.Cadence, r.RunTimeout,
			state(r), when(r.NextDueAt), when(r.LastFinishedAt), note(r))
	}
	if err := w.Flush(); err != nil {
		log.Printf("schedule-board: write report: %v", err)
		return 1
	}
	log.Printf("schedule-board: %d providers", len(rows))
	return 0
}

// source is the distinction the spec asks the report to draw: a provider running on
// defaults versus one somebody configured.
func source(r ingestsched.ProviderReport) string {
	if r.Overridden {
		return "override"
	}
	return "default"
}

// shardsColumn shows the intended count and, when they differ, what run state actually
// holds — a shard-count change that has not been reconciled is exactly the drift this
// whole change is about, so it must be visible rather than averaged away.
func shardsColumn(r ingestsched.ProviderReport) string {
	if r.ShardsInState == r.Shards {
		return fmt.Sprintf("%d", r.Shards)
	}
	return fmt.Sprintf("%d (state:%d)", r.Shards, r.ShardsInState)
}

func state(r ingestsched.ProviderReport) string {
	switch {
	case !r.Enabled:
		return "disabled"
	case !r.Managed:
		// "not-managed", not "static-timer": between §9 (the generated units are deleted)
		// and §8.5 (the column is dropped) a provider in this state is scheduled by
		// NOTHING, and a label asserting a timer that no longer exists would read as
		// healthy.
		return "not-managed"
	case r.InFlight > 0:
		return fmt.Sprintf("running:%d", r.InFlight)
	default:
		return "scheduled"
	}
}

func when(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04")
}

func note(r ingestsched.ProviderReport) string {
	if !r.Enabled && r.DisabledReason != "" {
		return r.DisabledReason
	}
	return r.Notes
}
