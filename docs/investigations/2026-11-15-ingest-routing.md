# Ingest transport recovery, 2026-11-15

Follow-up to the aggregator investigation at commit
`a04e2607bddcb9c8235212c54edf01c98660e46b` (read with `git show`, not cherry-picked).
Scope: Gradcracker, InfoJobs Brasil and SpainJobs shared Scrapling routes only.

## Bounded live verification

A throwaway package-local Go test used `newScraplingClient` against the existing operator
Scrapling service, reached through a localhost-to-SSH forwarding harness. The harness read
service configuration on operator without printing credentials and forwarded the service's
JSON answer unchanged. The real Go listing/detail parsers processed those responses.
No full crawl, database access/write, cooldown reset, deployment or host configuration change
was performed. The temporary test and forwarding process were removed after verification.

`go test ./internal/ingest/sources -run TestRoutingLiveTmp -v -count=1` passed in 66.362 s.
There were ten upstream fetch operations (service proxy retries retain their normal bound):

- Gradcracker, HTTP direct: both
  `/search/computing-technology/jobs?order=dateAdded` and
  `/search/electronic-electrical/jobs?order=dateAdded` parsed 80 cards each, with reported
  totals 1,113 and 1,042. The former still redirects to the current technology/IT listing.
  `/hub/849/herbert-smith-freehills-kramer/work-placement-internship/82215/2027-london-spring-vacation-scheme`
  yielded 5,743 bytes from `gradcrackerBody`. All three requests used `pacedGradcracker`.
- InfoJobs Brasil, HTTP + proxy: `/empregos-em-acre.aspx` parsed 20 cards and total 2,036;
  `Client.CookieValue` confirmed the returned `ab_session_id` entered the Go jar (value never
  logged). Exact fragment endpoint:
  `/mf-publicarea/VacancyList/GetVacancyListFragment?url=` followed by the URL-encoded absolute
  listing URL with `?page=2` or `?page=3`. Each parsed 20 cards, `eof=false`, with first ids
  12174775 and 12107404, respectively; neither repeated the preceding page's first id.
  `/vaga-de-consultor-tecnico-comercial-em-acre__12107404.aspx` then parsed a live posting
  with a 4,451-byte description through `infojobsBRJob`.
- SpainJobs, HTTP + proxy: `/sitemap.xml` decoded 14 entries;
  `/sitemap/jobs-new.xml` decoded 2,450 posting URLs. The first URL passed
  `spainjobsParsePosting`, and `spainjobs.detail` parsed a 2,539-byte description from
  `/companies/vitartis/tecnicoa-calidad-area-proveedores-y-clientes-productos-elaborados-en-valladolid--1a519a8634`.

These are individual access/parse observations, not proof of sustained crawl capacity.

## Session and routing decisions

`services/scrapling-fetch/engines.py` drops identity/framing headers but forwards `Cookie`
in HTTP mode. `_result` returns a flat string header map, and Go's Scrapling transport rebuilds
those headers; Go's `http.Client` then stores and sends cookies through its attached jar.
Browser mode does not forward caller headers and is unsuitable for this session flow.
The flat response map is not a general multi-Set-Cookie guarantee: this change relies on the
live-verified InfoJobs bootstrap session cookie. Live fragment progression verifies the cookie
works across rotating exits.

The InfoJobs route explicitly attaches a fresh jar to the tier client; rebuilding the adapter
without one would discard the earlier registry's cookie client. A regression runs the real
Acre listing walk over a fake Scrapling wire endpoint, requires the session cookie on both
fragment requests, and verifies seven local postings across three pages. It fails before the
route exists. Route and pacing/budget tests also failed before implementation.

Gradcracker retains its shared ten-second limiter, burst one, and 200-new-posting budget.
InfoJobs retains its adapter-wide limiter, 80-new-per-board budget, fragment walk and existing
4500-second scheduler timeout. SpainJobs retains its one-second limiter, 1,500-new-posting
budget and five-minute deadline margin. No adapter parser, scheduler or sweep semantics change.
Without `SCRAPLING_FETCH_URL`, all three retain their earlier transports.

## Production follow-up

Through the normal release process, build and deploy the ingest binary containing this commit
with the existing Scrapling URL/token configuration. Keep all current pacing and budgets;
keep InfoJobs unsharded with its 4500-second timeout. Observe the next due runs rather than
resetting cooldowns. Check listing errors, hydrated body counts, fragment progression for
InfoJobs, and whether Gradcracker develops sustained 403/429 refusals despite ten-second pacing.
A failure should remain visible in board health. No service deployment is required.

## Local verification

- `gofmt -l .`: no output.
- `go test ./internal/ingest/sources -run 'Scrapling|Gradcracker|InfojobsBR|Spainjobs' -count=1`: passed.
- `go vet -p=2 ./...`: passed.
- `go vet -p=2 -tags=integration ./...`: passed.
- `mise exec -- golangci-lint run --new-from-rev=HEAD --allow-parallel-runners ./...`: passed, zero issues.
- `pnpm check:size`: passed.
- `pnpm check:links`: passed after repairing a wrapped Markdown destination.
- Simplify pass: kept the three route builders and provider-local cookie jar; no new helper
  or transport protocol was needed.
- CodeRabbit 0.9.5 first review completed over all six files: one minor broken-link finding,
  accepted and repaired. Second run completed with zero findings but explicitly reused its
  earlier review (no fresh file analysis); the repaired link was independently checked.
- `go test -p=2 ./...`: passed.

The first concurrent repository-wide attempts exhausted local disk space. Reclaimed only
rebuildable Go cache entries unused for at least one hour (8 GiB), then reran with `-p=2`.
No application or source files outside this task were removed.
