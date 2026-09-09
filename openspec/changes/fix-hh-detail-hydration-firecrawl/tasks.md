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
- [x] 4.4 Requested code review (commit b6bf0c76 vs base a3d79c13).
      Assessment: "Ready to merge: with fixes" — no Critical/Important
      issues. Two Minor doc-comment issues fixed (commit 87076c7d):
      `hhru.go`'s type comment pointed at the wrong file for the
      DDoS-Guard rationale (registry.go → firecrawltier.go), and
      overclaimed the split as unconditional when it only holds with
      `FIRECRAWL_API_KEY` set. Reviewer independently verified the
      `ApplyFirecrawlEgress`-`direct`-parameter deviation (task 2.1) is
      real and sound, ran the full verification suite itself (all green),
      and confirmed all markdown links resolve.
- [x] 4.5 Opened PR #2662, CI green (all checks passed), merged (squash) to
      main as 146e51fe. User explicitly directed proceeding to deploy and
      verify (task 5) immediately after — the merge/deploy gate was
      acknowledged and crossed deliberately, not skipped.

## 5. Deploy-side (outside this repo, host2 — manual, AFTER code deploys)

**Status as of 2026-09-08 17:40 UTC**: code merged (#2662, `146e51fe`) and will
reach prod on the next `freehire-autodeploy` cycle. Checked the live account
(`GET /v1/team/credit-usage`): it is currently on Firecrawl's **Free plan —
1000 credits/month, 891 remaining this period** (resets 2026-09-29), nowhere
near hh's measured volume. User's explicit decision: **wait on the plan
upgrade** rather than raise `FIRECRAWL_MAX_PAGES_PER_RUN` or run a live
verification now — until the plan is upgraded, `hh` will keep hydrating
almost entirely list-only in practice (the existing shared 25-page/run cap
exhausts almost immediately once `hh` starts drawing on it too), which is a
safe, low-cost holding state: no behavior regression, no meaningful spend,
and `bayt`/`gulftalent`'s own small allocation isn't starved either. Resume
5.1-5.4 once the plan is upgraded.

- [ ] 5.1 Confirm/upgrade the Firecrawl account plan for the added volume (see
      design.md's cost math — recommend at least the 1M-credit/mo "Scale"
      tier for headroom until hh's steady-state rate is known). **Requires the
      Firecrawl web dashboard (billing) — outside SSH/API reach.**
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
