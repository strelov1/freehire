## 1. `hh` adapter: split listing and detail transports

- [x] 1.1 In `internal/ingest/sources/hhru.go`, change `hh` to carry two
      `HTMLGetter` fields (listing, detail), add
      `NewHHWithDetailGetter(listing, detail HTMLGetter) Source`, and redefine
      `NewHH(c HTMLGetter) Source` as `NewHHWithDetailGetter(c, c)` so every
      existing call site (`registry.go`, existing tests) is unchanged.
- [x] 1.2 Update `crawl()` to use the listing getter and `detail()` to use the
      detail getter.
- [x] 1.3 Write a failing test asserting a listing request goes to one fake
      `HTMLGetter` and a detail request goes to a DIFFERENT fake, via
      `NewHHWithDetailGetter` — then make it pass. Confirm the existing
      `NewHH`-based tests (`hhru_test.go`) still pass unchanged.

## 2. Wire `hh` onto the Firecrawl tier, off the proxy tier

- [x] 2.1 In `internal/ingest/sources/firecrawltier.go`, add `hh` to
      `firecrawlProviders`. **Revised during implementation** (design.md
      Decision 2 updated to match): the build function does NOT reuse the
      `direct` parameter `ApplyFirecrawlEgress` passes in — that value becomes
      the PROXIED client whenever `SOURCES_PROXY_URL` is set for ANY provider,
      which would put hh's listing back on the burned proxy. Instead:
      `func(hosted *firecrawlClient, _ HTTPClient) Source { return
      NewHHWithDetailGetter(NewClient(), hosted) }` — listing gets its own
      fresh, always-unproxied client.
- [x] 2.2 In `internal/ingest/sources/proxy.go`, remove `hh`'s entry from
      `proxiedProviders`; correct the stale "hh.ru's detail pages 403 the
      direct datacenter IP" comment (and a second stale mention in the
      `refusalRetryProviders` comment block) to describe the current, measured
      split (listing: direct; detail: Firecrawl) and why.
- [x] 2.3 In `internal/ingest/sources/pacer.go`, remove the now-unused
      `hhRequestInterval`/`hhRequestBurst` constants — no longer needed:
      Firecrawl's own client already handles vendor-side rate limiting, and
      listing's own loop is a handful of sequential requests. Confirmed no
      other references via build + full package test.
- [x] 2.4 Extended `TestHostedTierCarriesTheTwoUnreachableProviders` in
      `firecrawltier_test.go` to cover `hh`'s inclusion, and added
      `TestApplyProxyEgressLeavesHHOnTheDirectClient` in `proxy_test.go`
      confirming `hh` is no longer rewired by `ApplyProxyEgress`.
- [x] 2.5 Added `TestApplyFirecrawlEgressRewiresHHDetailWithAKey` and
      `TestHHKeepsItsCurrentTransportWithoutAKey` in `firecrawltier_test.go`:
      confirm `ApplyFirecrawlEgress` rewires hh with a key and leaves it alone
      without one. (The wantapply-style "listing keeps using `direct`" case
      does not apply to hh per the 2.1 revision — that split is instead
      covered at the adapter level by
      `TestHHWithDetailGetterSplitsListingAndDetailTransports` in
      `hhru_test.go`, task 1.3.)

## 3. Docs

- [x] 3.1 Updated `internal/ingest/sources/AGENTS.md`'s "hosted tier" section
      with an hh-specific callout: listing direct, detail via Firecrawl, and
      why hh can't reuse `ApplyFirecrawlEgress`'s shared `direct` transport.
- [x] 3.2 Updated `internal/platform/firecrawl/AGENTS.md` with a new "A
      different shape: hh is not IP-blocked at all" section. (No existing
      wantapply callout was actually in that doc to match — it's already
      stale on that count, pre-existing and out of scope for this change —
      so the new section stands on its own and cross-links to
      `internal/ingest/sources/AGENTS.md` for the wiring detail instead.)

## 4. Verify and land

- [x] 4.1 `gofmt -l .` clean, `go build ./...`, `go vet ./...`,
      `go test ./internal/ingest/sources/...` all green. Also ran the full
      `go test ./...` — no `FAIL` anywhere in the module.
- [x] 4.2 `go vet -tags=integration ./...` green (exit 0).
- [x] 4.3 Ran `simplify` skill pass on the diff — already clean and
      idiomatic, matches project conventions (dense narrative comments,
      per-file test fakes); no changes needed.
- [ ] 4.4 Request code review (`superpowers:requesting-code-review`); address
      feedback per `superpowers:receiving-code-review`.
- [ ] 4.5 Open PR; do not merge until task 5 (deploy-side prerequisites) is
      acknowledged, since merging without raising the Firecrawl budget on prod
      reproduces the same "hh: detail ... failed" symptom via
      `ErrBudgetSpent` instead of the CAPTCHA.

## 5. Deploy-side (outside this repo, host2 — manual, AFTER code deploys)

- [ ] 5.1 Confirm/upgrade the Firecrawl account plan for the added volume (see
      design.md's cost math — recommend at least the 1M-credit/mo "Scale"
      tier for headroom until hh's steady-state rate is known).
- [ ] 5.2 Raise `FIRECRAWL_MAX_PAGES_PER_RUN` in `/opt/freehire/.env` (recommend
      2000; see design.md Decision 4) and restart/allow the next
      `freehire-ingest@hh` timer firing to pick it up.
- [ ] 5.3 Watch the next 2-3 hourly `freehire-ingest@hh.service` runs'
      `journalctl` output for `hh: detail ... failed` lines dropping to near
      zero, and spot-check `/api/v1/jobs/search?source=hh` for real
      descriptions on newly-created rows (not just the salary paragraph).
- [ ] 5.4 Once the 7-day backlog window settles (~2026-09-10, per design.md
      Context), re-measure hh's steady-state detail-fetch rate and revisit
      whether the plan tier chosen in 5.1 is still the right fit.
