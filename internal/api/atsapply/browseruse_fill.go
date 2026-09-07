package atsapply

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/platform/browseruse"
)

// browserUseProviders are the ATS platforms this fallback executes for — the ones whose
// schema-only Reconcile (mergedFromAPIOnly) already produces a fully-resolvable Plan
// today, but which fillProviders excludes because no chromedp DOM-fill path exists for
// their live page. Greenhouse is deliberately absent: it already has a fill path.
// White-label (unrecognized-layout) postings are absent too, for a structural reason, not
// a policy one — see openspec/changes/add-browseruse-atsapply-fallback/design.md: there
// is no MergedField schema to build a Plan from when chromedp's own DOM scan failed, so
// there is nothing for this executor to be handed in the first place.
//
// Recruitee is absent for the same structural reason, found during implementation:
// internal/ingest/applyform.Fetchers has no registered Fetcher for it at all (its form
// arrives free with the ingest crawl and is written directly — see fetch.go's own
// NeedsRequestCapture doc), so Client.fetchSchema already parks a Recruitee attempt with
// errNoSchemaFetcher/reasonSubmissionNotImplemented long before Submit ever reaches this
// executor. Reaching Recruitee would need reading its already-captured form from storage
// instead of applyform.Fetcher — a real gap, not a decision, and out of scope here.
var browserUseProviders = map[string]bool{
	"ashby":    true,
	"workable": true,
}

// browserUseOutcome is the terminal signal buildTask's own instruction requires the
// agent's report to end with — parseOutcome extracts it.
type browserUseOutcome string

const (
	outcomeConfirmed   browserUseOutcome = "CONFIRMED"
	outcomeUnconfirmed browserUseOutcome = "UNCONFIRMED"
	outcomeParked      browserUseOutcome = "PARKED"
)

// browserUseEligible reports whether claimed's provider and an already-resolved plan
// qualify for this fallback: an eligible provider, and no file-kind field anywhere in the
// plan. Résumé/CV upload is out of scope for this executor (design.md's Non-Goals) — a
// fully-resolved plan that happens to include a résumé field (because the attempt carries
// an approved tailored CV) still falls through to the ordinary "no fill path" park,
// exactly as it would if this executor did not exist.
func browserUseEligible(provider string, plan Plan) bool {
	if !browserUseProviders[provider] {
		return false
	}
	for _, f := range plan.Fields {
		if f.Kind == "file" {
			return false
		}
	}
	return true
}

// buildTask builds the natural-language fill instruction browser-use executes for one
// resolved plan — the same shape a live spike verified: every value stated verbatim, an
// explicit prohibition on touching or guessing anything else, and a requirement to end
// the report with exactly one machine-parseable outcome marker. merged supplies each
// field's human-readable label (Plan.Fields carries only the opaque id) so the agent can
// find the right widget on the live page.
func buildTask(plan Plan, merged []MergedField, applyURL string) string {
	labelByID := make(map[string]string, len(merged))
	for _, f := range merged {
		labelByID[f.ID] = f.Label
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Open %s . Fill in EXACTLY these fields with EXACTLY these values, and do not modify, guess, or invent anything else:\n", applyURL)
	for _, f := range plan.Fields {
		label := labelByID[f.ID]
		if label == "" {
			label = f.ID
		}
		fmt.Fprintf(&b, "- %q (id %q): %s\n", label, f.ID, f.Value)
	}
	b.WriteString("\nDo not touch, select, or fill any field not listed above, under any circumstances — leave every other field exactly as it starts. Do not guess an answer for anything, including any field whose value you cannot find above. Only click the final submit control if every field listed above was accepted as given; if the page will not let you submit without touching a field not listed above, stop instead of touching it.\n\n")
	b.WriteString("End your final answer with exactly one of these three lines, verbatim, as the LAST line of your response and nothing after it:\n")
	b.WriteString(string(outcomeConfirmed) + ": <the exact confirmation text or message you saw after submitting>\n")
	b.WriteString(string(outcomeUnconfirmed) + "\n")
	b.WriteString(string(outcomeParked) + ": <why you could not submit>\n")
	return b.String()
}

// parseOutcome extracts buildTask's required outcome marker from the agent's final
// report. Only the last non-empty line is ever considered a marker — one buried earlier
// in a longer report does not count, since the instruction asks for it as the report's
// last line. A report with no recognizable marker there is treated as unconfirmed, the
// same "ambiguous means not confirmed" rule chromedp's own fillAndSubmit already follows.
func parseOutcome(report string) (outcome browserUseOutcome, detail string) {
	lines := strings.Split(strings.TrimSpace(report), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, string(outcomeConfirmed)+":"):
			return outcomeConfirmed, strings.TrimSpace(strings.TrimPrefix(line, string(outcomeConfirmed)+":"))
		case line == string(outcomeUnconfirmed):
			return outcomeUnconfirmed, ""
		case strings.HasPrefix(line, string(outcomeParked)+":"):
			return outcomeParked, strings.TrimSpace(strings.TrimPrefix(line, string(outcomeParked)+":"))
		}
		break
	}
	return outcomeUnconfirmed, ""
}

// browserUseEnforce gates whether the fallback actually executes or only logs what it
// would have attempted. Ships OFF (shadow) by default, mirroring PLAN_ENFORCE and
// add-auto-apply-eligibility-gate's own rollout — a false positive here spends real money
// against a real employer's form. A plain env read on every call, not memoized: see
// add-auto-apply-eligibility-gate's own fix for the sync.OnceValue footgun (it freezes
// whichever value the first call happened to see, breaking t.Setenv-based tests).
func browserUseEnforce() bool {
	return os.Getenv("AUTO_APPLY_BROWSERUSE_ENFORCE") == "1"
}

// browserUsePerRunCostCapUSD reads the per-run spend cap passed to the v4 API's own
// maxCostUsd field. A missing or unparseable value falls back to a conservative default
// rather than leaving the run uncapped — the spike's data:-URI test measured a single
// run reaching $0.07 on what should have been a cheap fill; an uncapped run has no ceiling
// at all if the agent gets stuck in a similar loop.
func browserUsePerRunCostCapUSD() float64 {
	const fallback = 0.25
	raw := os.Getenv("AUTO_APPLY_BROWSERUSE_MAX_COST_USD")
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

// dailySpendGuard bounds the fallback's AGGREGATE spend across one cmd/auto-apply run.
// outbox.RunPool may process attempts concurrently, so the running total is
// mutex-protected. Ships in shadow mode alongside browserUseEnforce: Allow always
// reports true (and logs) until AUTO_APPLY_BROWSERUSE_ENFORCE is set, so the threshold's
// effect on real traffic can be observed before it starts refusing anything.
type dailySpendGuard struct {
	mu       sync.Mutex
	spentUSD float64
	limitUSD float64 // <= 0 means unlimited
}

// newDailySpendGuardFromEnv reads AUTO_APPLY_BROWSERUSE_DAILY_CAP_USD once. Unset,
// empty, or unparseable leaves the guard unlimited (matches this executor's overall
// "absent config disables/uncaps the feature it configures" convention — see
// browserUseEnforce and the nil-executor checks in trySubmitViaBrowserUse).
func newDailySpendGuardFromEnv() *dailySpendGuard {
	raw := os.Getenv("AUTO_APPLY_BROWSERUSE_DAILY_CAP_USD")
	if raw == "" {
		return &dailySpendGuard{limitUSD: 0}
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return &dailySpendGuard{limitUSD: 0}
	}
	return &dailySpendGuard{limitUSD: v}
}

// allow reports whether a new execution may start, given spend recorded so far.
// enforced=false (shadow mode) always returns true; a would-be refusal is logged instead.
func (g *dailySpendGuard) allow(enforced bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.limitUSD <= 0 || g.spentUSD < g.limitUSD {
		return true
	}
	if !enforced {
		log.Printf("atsapply: browser-use daily spend guard would refuse (spent $%.4f of $%.4f cap) — shadow mode, allowing anyway", g.spentUSD, g.limitUSD)
		return true
	}
	return false
}

// record adds one execution's reported cost to the running total.
func (g *dailySpendGuard) record(costUSD float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.spentUSD += costUSD
}

// BrowserUseExecutor submits an already-resolved Plan through the browser-use cloud
// agent. It never resolves an answer itself — see buildTask's doc comment — and maps the
// agent's outcome onto the same autoapply.SidecarResult shape chromedp's own
// fillAndSubmit path returns, so internal/application/autoapply's runner needs no
// backend-specific handling.
type BrowserUseExecutor struct {
	client       *browseruse.Client
	spend        *dailySpendGuard
	perRunCapUSD float64
	pollInterval time.Duration
	timeout      time.Duration
}

// NewBrowserUseExecutor builds an executor. client is required (a nil client is the
// "not configured" state Client.browserUse itself represents — see client.go).
func NewBrowserUseExecutor(client *browseruse.Client) *BrowserUseExecutor {
	return &BrowserUseExecutor{
		client:       client,
		spend:        newDailySpendGuardFromEnv(),
		perRunCapUSD: browserUsePerRunCostCapUSD(),
		pollInterval: 5 * time.Second,
		timeout:      3 * time.Minute,
	}
}

// submit runs plan through browser-use against applyURL. handled reports whether this
// call decided the attempt's outcome at all; false (only when the daily spend guard
// refuses) means the caller falls back to its own unchanged StatusParked handling.
func (e *BrowserUseExecutor) submit(ctx context.Context, plan Plan, merged []MergedField, applyURL string) (result autoapply.SidecarResult, handled bool, err error) {
	if !e.spend.allow(browserUseEnforce()) {
		return autoapply.SidecarResult{}, false, nil
	}

	task := buildTask(plan, merged, applyURL)
	runID, err := e.client.CreateRun(ctx, task, e.perRunCapUSD)
	if err != nil {
		return autoapply.SidecarResult{}, true, fmt.Errorf("browser-use: create run: %w", err)
	}

	res, err := e.client.Wait(ctx, runID, e.pollInterval, e.timeout)
	if err != nil {
		return autoapply.SidecarResult{}, true, fmt.Errorf("browser-use: wait for run %s: %w", runID, err)
	}
	e.spend.record(res.TotalCostUSD)

	if res.Status != "completed" {
		// The run itself failed or was cancelled before ever producing a report. Treated
		// as unconfirmed rather than an ordinary retryable error: it may already have
		// interacted with the live form before failing, and — exactly like chromedp's own
		// StatusUnconfirmed — this package never risks a second real submission on an
		// outcome it cannot rule out as already-submitted.
		return autoapply.SidecarResult{Status: autoapply.StatusUnconfirmed}, true, nil
	}

	outcome, detail := parseOutcome(res.Result)
	switch outcome {
	case outcomeConfirmed:
		return autoapply.SidecarResult{Status: autoapply.StatusApplied}, true, nil
	case outcomeParked:
		return autoapply.SidecarResult{Status: autoapply.StatusParked, Reason: detail}, true, nil
	default:
		return autoapply.SidecarResult{Status: autoapply.StatusUnconfirmed}, true, nil
	}
}
