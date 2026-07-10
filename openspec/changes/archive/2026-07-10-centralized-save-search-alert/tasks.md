## 1. Orchestration module (`web/src/lib/saveSearchAlert.ts`)

- [x] 1.1 `alertName(query)` — readable saved-search name from a filter query via the existing facet labels (`FACETS` options / `dynamicLabel`); capped at 100 chars; stable "Job alert" fallback when empty.
- [x] 1.2 `enableAlert(query, deps)` — the idempotent chain over injected store ports (DI, so it's unit-testable in plain-Node vitest): require auth (else record pending + open dialog); `ensureSaved` (reuse by `canonicalQuery` or `create(alertName)`); `notifications.ensureLoaded`; `!linked` → `need-link`; already-subscribed → success; else `subscribe`; 409 on create/subscribe → success.
- [x] 1.3 `alertStateFor(snapshot)` — pure render state `disabled` / `signed-out` / `idle` / `connecting` / `subscribed`; plus `matchedSavedSearch` (dedupe by canonical query).
- [x] 1.4 `pendingAlert` — best-effort `localStorage` (`hire.pendingAlert`) `setPendingAlert`/`consumePendingAlert` (read+clear once), mirroring `filterStorage.ts` guards.
- [x] 1.5 Unit tests (vitest): 20 tests — `alertName`, `matchedSavedSearch`, `alertStateFor`, `enableAlert` order with faked ports (signed-out→auth+pending; matched→reuse; unmatched→create; not-linked→need-link; already-subscribed; 409 create/subscribe→success + suffix retry), `pendingAlert` set/consume/clear + off-storage no-op.

## 2. Compact component (`web/src/lib/components/filters/SaveSearchAlert.svelte`)

- [x] 2.1 `query` + `variant: 'quick' | 'full'` (+ `autostart` for resume) props; renders states from `alertStateFor`: signed-out (primary CTA → `enableAlert` opens auth + records pending), idle (primary "Get new jobs in Telegram"), `connecting` ("tap Start → I've connected" recheck), `subscribed` (with off toggle in `full`), error with retry.
- [x] 2.2 Primary action → `enableAlert`; on `need-link` open `notifications.link()` deep link + set `connecting`, and `refreshTelegram()` recheck → re-run to subscribe. `disabled` (telegram off) hides the alert; `full` shows a plain "Save filter" (`ensureSaved`) instead.
- [x] 2.3 `full` variant adds the plain-save (telegram-off) and the turn-off toggle; `quick` is the one-tap offer. `autostart` runs the flow on mount (pending-alert resume).

## 3. Restore the sidebar "Save filter" button

- [x] 3.1 `FilterSummaryShell.svelte`: add a "Save filter" affordance beneath the "All filters" button (an inline expander), rendered only for the job summary (not company); accept an optional `saveQuery`/slot so the company summary opts out.
- [x] 3.2 `FilterSummary.svelte`: pass the live query and render `SaveSearchAlert query={current} variant="full"` in the expander.

## 4. Refactor the modal "My filters" tab (`SavedSearches.svelte`)

- [x] 4.1 Replace the inline save bar + Telegram notify block with `SaveSearchAlert query={current} variant="full"`; keep the saved-set list (apply / rename / delete) and the dirty "Update <name>" affordance.
- [x] 4.2 Remove the now-dead notify/link/save handlers moved into the shared module; ensure sign-out reset still clears both stores.

## 5. Onboarding banner + OAuth resume (`JobsView.svelte`, onboarding wiring)

- [x] 5.1 After `completeWizard(query)`, show an ephemeral post-onboarding banner hosting `SaveSearchAlert query variant="quick"` ("get these jobs in Telegram").
- [x] 5.2 On `/jobs` mount: if authenticated and `pendingAlert` has a query, consume it and replay `enableAlert` (feed already restored from `hire.jobFilters`).

## 6. Verify

- [x] 6.1 `npm run check` (svelte-check: 0 errors) and vitest (112 passed) green; `npm run lint` clean on changed files.
- [x] 6.2 Visual verification (throwaway route + headless Chrome, prod-proxied): the restored sidebar "Save filter" button renders under "All filters"; the onboarding alert banner and the sidebar/modal `full` control both render the shared component; signed-out shows the "Get new jobs in Telegram" CTA (→ auth + pending). The signed-in link→subscribe flow needs a real prod session + bot — to be exercised manually after deploy.
- [x] 6.3 Modal "My filters" refactor preserves apply / rename / delete / "Update <name>" logic verbatim and delegates save+notify to `SaveSearchAlert`; svelte-check clean. Authed e2e (save/apply/rename/delete/notify) to confirm manually post-deploy.

## 7. Post-review refinements (flow + surfaces)

- [x] 7.1 Reworked to **save-first**: saving is the primary standalone action; the Telegram alert is offered only once the search is saved (was: one tap implies save — confusing, esp. in the modal). Updated `alertStateFor` states (`unsaved`/`saved`/`idle`), `enableAlert` removed in favour of `ensureSaved` + a component-driven subscribe; spec + design D2 updated.
- [x] 7.2 Sidebar renders the shared control directly under "All filters" (no separate expander); its `unsaved` state is the "Save filter" button.
- [x] 7.3 Moved the FilterModal footer save-nudge below the "Show jobs" action row.
- [x] 7.4 Merged the separate `/my/notifications` page into the account saved-searches page ("Saved searches & alerts"): Telegram connection card on top + a per-saved-search alert toggle; `/my/notifications` 308-redirects; header menu shows one item; `NotificationsView` removed.
- [x] 7.5 Redesigned the Telegram connection card (round neutral badge + brand `ProviderIcon`, horizontal layout).
- [x] 7.6 Code-review pass (3 finder angles) + fixes: one-shot `onMount` autostart (was a self-invalidating `$effect` loop), `enableAlert`/subscribe gates on `telegram.enabled`, reactive pending-alert resume (covers in-dialog + OAuth), dropped duplicate store-reset (owned by `+layout`). svelte-check 0, tests green, lint clean.
