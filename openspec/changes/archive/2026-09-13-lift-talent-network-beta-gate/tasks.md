Every task runs the spec-driven-tdd micro-cycle: RED (a failing test first) → GREEN →
REFACTOR → simplify → review → only then `[x]`.

## 1. Server: lift the join gate

- [x] 1.1 `internal/api/handler/me_talent_network_test.go`: rewrote
      `TestPutTalentNetwork_JoiningIsBetaOnly` into `TestPutTalentNetwork_NonBetaAccountCanJoin`
      and deleted `TestPutTalentNetwork_LeavingIsNeverBetaGated`. Confirmed RED (403
      instead of 200) against the pre-change handler.
- [x] 1.2 `internal/api/handler/me_talent_network.go`: removed the `IsBetaTester` check and
      its 403 branch from `PutVisibility`, and dropped `IsBetaTester` from the
      `talentNetworkStore` interface. `talentnetwork.Join` is now unconditional on the
      non-`off` branch.
- [x] 1.3 `internal/api/handler/me_talent_network_test.go`: removed the `beta` field and
      `IsBetaTester` method from `fakeTalentNetworkStore`, and every `beta: true`/`beta:
      false` literal. **Deviation from the plan**: `TestPutTalentNetwork_NonBetaAccountCanJoin`
      (added in 1.1) became an exact duplicate of the pre-existing
      `TestPutTalentNetwork_MintsAHandleOnFirstJoin` once the fake could no longer express
      "beta" as a concept at all — there is nothing left to distinguish a "non-beta"
      account by. Deleted it during this refactor rather than keep a redundant test; the
      remaining join tests already cover "any account can join" once beta drops out.
- [x] 1.4 `go build ./...`, `go vet ./...`, `go test ./internal/api/handler/...` green.

## 2. Frontend: the profile invitation and the nav entry

- [x] 2.1 `web/src/lib/components/profile/TalentNetworkInvite.svelte`: removed the `beta`
      derived value and its use in the `{#if beta && status === 'ready'}` guard — the
      card now shows to every signed-in candidate once `status === 'ready'`. Updated the
      comment above it. Added `TalentNetworkInvite.spec.ts` (no test file existed before);
      confirmed RED against the pre-change component with both a beta-tester and a
      non-beta-tester case. **Post-review simplification**: once GREEN, the component no
      longer reads `currentUser`/`beta_tester` at all, so the two cases were an
      unmocked-effect duplicate of each other (same finding shape as 1.3's Go test).
      Collapsed to one test and dropped the now-pointless `$lib/auth.svelte` mock
      (flagged as a Minor nit by code review).
- [x] 2.2 `web/src/lib/accountNav.ts`: removed the
      `{ href: '/my/talent-network', label: 'Talent Network', betaOnly: true }` entry and
      its preceding comment block. Leave the Mentorship `betaOnly` entry and the
      `visibleAccountNav`/`betaOnly` mechanism itself untouched. **Also**
      `web/src/lib/accountNavIcons.ts`: its icon map is keyed by
      `Record<AccountNavItem['href'], LucideIcon>`, so removing the href turns the
      `'/my/talent-network': Radar` entry into a type error — remove that entry and the
      now-unused `Radar` import (found by `pnpm check`, not listed when this task was
      written). **Also** `web/src/lib/i18n/shell.ts`: drop the `/my/talent-network` key
      from both the English `navItems` and the Russian `ru.navItems` override — a stale
      key here failed `shell.test.ts`'s own coverage assertion (found by the full vitest
      run, not listed when this task was written).
- [x] 2.3 `web/src/lib/accountNav.test.ts`: replaced `'hides the Talent Network outside the
      beta group'` with `'carries no Talent Network entry, for anyone'`, asserting
      `/my/talent-network` is absent from `visibleAccountNav`'s output for every
      combination of `isModerator`/`isBetaTester`. Also updated the `toHaveLength(20)`
      section-count assertion to 19.
- [x] 2.4 `web/src/routes/talent/+page.svelte`: removed the `<meta name="robots"
      content="noindex">` tag and its explanatory comment — the list is indexable now
      (matches `talent-network-catalog`'s existing "The list is indexable, a card is not"
      requirement). The card page's own `noindex` (`/talent/[handle]/+page.svelte`) is
      untouched.
- [x] 2.5 `pnpm --dir web check` (0 errors), `lint` (0 errors, pre-existing warnings only),
      `build`, and the full vitest suite (1929 tests) all green.

## 3. Documentation and spec sync

- [x] 3.1 Grepped the touched files and `internal/candidate/talentnetwork/` — no stale
      beta-gate comment remains about the Talent Network. The remaining `beta`/`betaOnly`
      hits are all Mentorship's own (unrelated, out of scope).
- [x] 3.2 `openspec validate lift-talent-network-beta-gate --strict` passes.
      **Deviation**: the validator refused a MODIFIED requirement that silently dropped
      the original spec's "A signed-in candidate opens their account" scenario — a
      MODIFIED block replaces the requirement wholesale, so archive would otherwise lose
      it with no record. Fixed by keeping that scenario's name and updating its WHEN/THEN
      to state the new behavior (no Talent Network nav section), rather than introducing
      a differently-named scenario that reads as an addition instead of a change.

## 4. Verification

- [x] 4.1 `gofmt -l .` prints nothing; `go build ./...`, `go vet ./...`, and
      `go vet -tags=integration ./...` all pass. `go test ./...` passes except a
      pre-existing, unrelated failure — `cmd/billing-sync`'s
      `TestTheStoreProviderAloneKeepsTheWorkerRunning` (no billing provider configured in
      this environment), already documented as pre-existing by the sibling
      `talent-network-education-certifications` change's own verification task.
- [x] 4.2 `pnpm --dir web check` (0 errors), `lint` (0 errors), `build`, and the full
      vitest suite (1929 tests) all pass.
- [ ] 4.3 Manual browser pass: as a non-beta account, open `/my/profile` and confirm the
      Talent Network invitation shows and joining succeeds; confirm `/my` navigation
      carries no Talent Network entry for any account; view page source of `/talent` and
      confirm no `noindex` meta tag. **Not done automatically**, same reason the sibling
      `talent-network-education-certifications` change recorded: this machine's shared
      Docker host port 5432 is already bound by another concurrent worktree's Postgres
      (`mentor-directory-filters-db-1`), so `make up` would conflict. The unit/component
      equivalents cover the same guarantees at a lower level: `TestPutTalentNetwork_*`
      (server join behavior), `TalentNetworkInvite.spec.ts` (the invitation shows
      regardless of `beta_tester`), and `accountNav.test.ts`'s "carries no Talent Network
      entry, for anyone". Run this step by hand once a free `make up` stack is available.
