package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// cliArgs is the parsed command line (with the INGEST_PROVIDER / SHARD fallbacks applied).
type cliArgs struct {
	provider, shardSpec, boardFilter, regionFilter string
	reconcileLinkedIn                              bool
	reconcileAfter                                 int64
}

// errReconcileConfig marks a bad ownership-reconciliation request; run exits 2 for it, 1 for
// every other argument error.
var errReconcileConfig = errors.New("ownership reconciliation requires linkedin without board, region or shard filters")

// parseArgs reads the provider (first positional or INGEST_PROVIDER), --shard=i/n (or SHARD),
// --board=, --region= and the reconcile flags. An unknown flag or a second positional is an
// error, not a silent full crawl: a mistyped --board must not look like a legitimate run.
func parseArgs(args []string, getenv func(string) string) (cliArgs, error) {
	var c cliArgs
	for _, a := range args {
		switch {
		case a == "--reconcile-linkedin":
			c.reconcileLinkedIn = true
		case strings.HasPrefix(a, "--reconcile-after="):
			var err error
			c.reconcileAfter, err = strconv.ParseInt(strings.TrimPrefix(a, "--reconcile-after="), 10, 64)
			if err != nil || c.reconcileAfter < 0 {
				return c, fmt.Errorf("--reconcile-after needs a non-negative posting id: %w", errReconcileConfig)
			}
		case strings.HasPrefix(a, "--shard="):
			c.shardSpec = strings.TrimPrefix(a, "--shard=")
		case a == "--shard":
			// The value must be attached; a space-separated form would swallow the provider name.
			return c, errors.New("--shard needs an attached value, e.g. --shard=2/6")
		case strings.HasPrefix(a, "--board="):
			c.boardFilter = strings.TrimPrefix(a, "--board=")
		case a == "--board":
			return c, errors.New(`--board needs an attached value, e.g. --board="Frontend Developer"`)
		case strings.HasPrefix(a, "--region="):
			c.regionFilter = strings.TrimPrefix(a, "--region=")
		case a == "--region":
			return c, errors.New("--region needs an attached value, e.g. --region=IT")
		case a == "":
		case strings.HasPrefix(a, "-"):
			return c, fmt.Errorf("unknown argument %q", a)
		case c.provider == "":
			c.provider = a
		default:
			return c, fmt.Errorf("unexpected second provider %q (already have %q)", a, c.provider)
		}
	}
	if c.provider == "" {
		c.provider = getenv("INGEST_PROVIDER")
	}
	if c.shardSpec == "" {
		c.shardSpec = getenv("SHARD")
	}
	if c.provider == "" {
		return c, errors.New("no provider given (pass it as an argument or set INGEST_PROVIDER)")
	}
	if (c.reconcileLinkedIn && (c.provider != "linkedin" || c.shardSpec != "" || c.boardFilter != "" || c.regionFilter != "")) ||
		(!c.reconcileLinkedIn && c.reconcileAfter != 0) {
		return c, errReconcileConfig
	}
	return c, nil
}

// argsExitCode is the exit code for a parseArgs error.
func argsExitCode(err error) int {
	if errors.Is(err, errReconcileConfig) {
		return 2
	}
	return 1
}

// runExitCode is the crawl run's exit code: broken when it reached nothing (crawlReachedNothing),
// was cut by its deadline, or a sweep close failed.
func runExitCode(total pipeline.Stats, truncated bool, sweepFailed int) int {
	failed := sweepFailed
	if crawlReachedNothing(total) {
		failed++
	}
	// A truncated run did not crawl what it was asked to: degraded however cleanly it stopped.
	if truncated {
		failed++
	}
	return worker.ExitCode(failed, 0)
}

// reconcileExitCode is exit 1 for a failed or unfinished (report.More) reconciliation batch, so
// the operator re-runs it.
func reconcileExitCode(report ownershipReport, err error) int {
	if err != nil || report.More {
		return 1
	}
	return 0
}
