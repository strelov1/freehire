# Bounded PageUp and Clinch completion, 2026-11-15

Integrated the unpublished dependency through `a6c3a1cab` before implementation.
This followup changes only PageUp 1023, Clinch detail routing and the existing
catalog recovery script. Previous provider repairs remain dependency commits.

## Implemented behavior

PageUp board 1023 uses the verified
`https://jobs.northmemorial.com/nmh/en-us/search/?page-items=1000` XHR endpoint.
Only this board recognizes `/nmh/en-us/job/` numeric IDs and only this host uses
Scrapling HTTP + proxy. Other tenants retain their canonical host/path and direct
client. Responsive duplicate links emit one posting; a reported count that differs
from recognized unique jobs fails before hydration, including truncated first pages
or a future catalogue exceeding the 1000-item request. Missing count on 1023 fails.
No pagination protocol was invented. Numeric external IDs and catalog company `1023`
are preserved; correcting that company label is separate catalog work.

Clinch retains direct sitemap reads. Its detail getter for the 38 verified destination hosts now uses the existing
Scrapling HTTP + proxy client beneath the existing shared 1500 ms, burst-one limiter.
The existing service timeout, no outer retry policy, upstream status handling,
challenge latch and sitemap fallback remain. Other Clinch hosts keep direct detail reads under that same limiter.
There is no new crawler or rate increase.
Without Scrapling configuration the existing direct fallback remains; readable
hydration is therefore a release configuration prerequisite.

The recovery script contains exactly the 36 targets in the
[PageUp ledger](2026-11-15-ingest-pageup.md), plus previously verified 893 and 784.
It generates SQL literals for all 38 origins and destinations, requires each exact
origin company/empty region/false hub/empty tenants, rejects conflicting live
identities before invoking any writer, skips matching existing destinations, and
uses `cmd/add-board` only. Repeated execution is idempotent. Concurrent additions
remain guarded by the catalog writer and unique index; a race can stop an apply
partway and a rerun reconciles matching additions. Old boards and jobs are preserved.
899, 1023, 1034 and 1245 are excluded from Clinch activation.

## Bounded read-only evidence

On operator, one exact North Memorial XHR search and one detail request through
Scrapling HTTP + proxy returned HTTP 200: 146491 and 13440 response bytes.
Search JSON count was 184. A temporary replay of that complete captured listing
recognized 184/184 unique jobs. A one-job `NewPageUp.Fetch` replay of captured detail
514883 retained company `1023` and produced a 5510-byte sanitized posting body.
No other North Memorial detail was fetched. Temporary replay code was removed.
Permanent tests cover custom URL/ID/company/body, responsive duplication, incomplete
counts and canonical/custom transport separation.

The exact Moreton Bay detail linked in the PageUp ledger was requested once each
operator-direct, Scrapling HTTP direct and Scrapling HTTP + proxy. All returned
200 this window (108508, 108507 and 108507 bytes). The earlier ledger's 30 direct
202 challenges still motivate the proxy route; this cold sample does not establish
sustained acceptance or a speed improvement. The production registry constructor
is permanently tested with this known URL and its captured real fixture through a
fake Scrapling service: HTTP mode, proxy flag, metadata/body, direct sitemap client
and unchanged shared limiter are asserted. Existing challenge/error tests remain.

Actual default script execution on core via operator exited zero and printed exactly:

```text
38 missing destinations; PageUp boards and jobs untouched
```

It performed the `BEGIN READ ONLY` preflight and printed 38 dry-run add commands.
No writer was invoked. Mixed existing targets, repeat execution, late conflicts,
changed origin metadata and SQL literal construction have behavioral tests.

## Release handoff and remaining blockers

No deployment, push, PR, production write, retirement, host change, cooldown reset
or full crawl was performed. After separately approved normal release of the
combined commit, with existing Scrapling service configuration present, run from
its repository root with the intended `DATABASE_URL` already loaded:

```sh
python3 docs/investigations/pageup-clinch-recovery.py
# Separately approved catalog activation, after the repaired binary is installed:
python3 docs/investigations/pageup-clinch-recovery.py --apply
```

Use natural scheduling/cooldown expiry. Validate real bodies and coverage before
separately reviewing retirement; this script never retires the old PageUp boards.
The normal binary release entrypoint and full-SHA argument were read on core.
Once the final combined commit is published, CI passes and release is approved,
substitute its full 40-character SHA in this exact operator-to-core command:

```sh
ssh root@89.167.94.146 'ssh core /opt/freehire/bin/release.sh freehire <COMBINED_FULL_SHA>'
```

This is a handoff command, not an action performed here. The placeholder must be
replaced before executing it; do not use this followup's standalone SHA as a release
if it omits the combined dependency. The broader body-refresh release prerequisite
below must also be resolved by its owner.

True external blockers remain those individually established in the ledgers:
899 has no listed vacancy/detail; 1034 and 1245 need an accessible employer-scoped
I Work for NSW listing (security pages are not data). 1245 also needs reviewed
company identity. Existing Mindsight/OpenCATS and Bullhorn Flanders blockers in
the integration/ATS reports are not newly claimed repaired here.

Periodic ops body refresh remains disabled and release-blocked. No refresh policy
or slice budget is activated by this patch; the integration ledger's budgeting
prerequisite remains. Local compilation/tests do not establish production coverage.

## Verification

Checks and both completed CodeRabbit rounds are recorded below.

CodeRabbit round 1 completed (exit zero, nine files, one major question).
Its snapshot used the initial provider-wide detail route. The final route is limited
to the 38 verified public hostnames; an arbitrary private sitemap link stays on the
SSRF-guarded direct client. A behavior regression attempts a loopback detail and
asserts neither the endpoint nor proxy getter is reached. The Scrapling service also
validates initial destinations. Its existing redirect behavior trusts curated vendor
hosts; no arbitrary initial URL is admitted by this allowlist. Redirect policy is
unchanged shared transport behavior, not evidence that any measured vendor route
redirects internally. No service or host configuration was changed.

Final code checks passed with `GOMAXPROCS=2`: `go vet -p 2 ./...`,
`go test -p 2 ./...`, and `go vet -p 2 -tags=integration ./...`.
The final full sequence includes the private-link regression. The database-backed
full integration suite was not run; tagged vet establishes compilation only.
Targeted PageUp/Clinch/Scrapling regressions passed, including existing challenge
latch/error and real metadata fixtures. Seven Python tests, Ruff check/format,
gofmt, changed-code golangci-lint (zero issues), size/link/whitespace checks and
Gitleaks over the followup diff passed. Simplify retained existing APIs and moved
the board-specific parsing selection outside the per-link walk; no dependency,
new crawler, body-refresh activation or unrelated provider changes were added.

CodeRabbit round 2 completed (exit zero, all 12 files, three findings). Accepted:
adjacent responsive anchors before their shared metadata could keep the incomplete
first record. Deduplication now merges missing title/location/description into the
retained native ID; a regression covers exactly two adjacent anchors followed by
location and summary. Two major questions concerned PageUp production dispatch:
inspection confirms `ApplyScraplingEgress` installs `pageupCustomHTTP`, so no routing
fix was needed. Added a production-registry wire regression proving both 1023
search and detail use HTTP + proxy with the XHR header and yield a real body; a
canonical 889 read stays on the direct side without another service request.
No third review was run. Final vet/tests/tagged-vet are rerun after these fixes.

After round 2 fixes, a full test rerun hit local linker `errno=28` (disk full),
not an assertion failure. Removed about 5 GiB of disposable Go cache entries older
than this task, then reran the complete sequence successfully: full vet, full tests,
and integration-tag vet, all with `GOMAXPROCS=2` and `-p 2`. Final targeted tests,
Ruff/Python checks and changed-code golangci-lint also pass. The Go proxy allowlist
was compared directly with the Python destinations: exactly the same 38 hosts.
