Every task runs the spec-driven-tdd micro-cycle: RED (a failing test first) → GREEN →
REFACTOR → simplify → review → only then `[x]`.

## 1. Membership: two states, and one permanent handle

- [x] 1.1 Migration `0148_talent_network_two_states.sql`. **RENUMBERED TWICE while this
      branch was open** — written as `0145`, which was free at the branch point and was
      taken by `0145_mentorship` before review, then `0146`/`0147` were taken too. There
      is no gate on the number and ordering falls to the alphabet, so **re-check the
      highest on `origin/main` immediately before opening the PR**, not when the file is
      written. References to APPLIED history (0085, 0086, 0117, 0118) must not move with
      it. Rewrite
      `talent_network_visibility = 'public'` to `'anonymous'`, then drop
      `users_talent_network_visibility_check` and add it back admitting only
      `off`/`anonymous`. `users` is hot: `migrate: no-transaction`, and the split
      `ADD ... NOT VALID` + `VALIDATE CONSTRAINT` shape 0085 documents. Run
      `pnpm check:sql`.
- [x] 1.2 `internal/platform/db/queries/users.sql`: the set-visibility query refuses
      anything but the two states; `make sqlc`, and confirm the pre-commit diff is clean.
- [x] 1.3 `internal/api/handler/me_talent_network.go`: `public` becomes an invalid input
      (400), the response still echoes the stored value. Extend
      `me_talent_network_test.go` — a request asking for `public` is refused and stores
      nothing.
- [x] 1.4 Migration `0149_users_talent_handle.sql`: add nullable `talent_handle text` to
      `users`. Nullable is the point — a non-member has no handle, and a default would mint
      one for every account that never joins.
- [x] 1.5 Migration `0150_users_talent_handle_uniq_idx.sql`: `CREATE UNIQUE INDEX
      CONCURRENTLY`, own `migrate: no-transaction` file, the shape 0086 uses. **Not `IF NOT
      EXISTS`** — it skips the `indisvalid = f` carcass a cancelled concurrent build leaves,
      which is what 0117 and 0118 existed to repair.
- [x] 1.6 Minting, in `internal/candidate/talentnetwork`: base from the category the most
      recent role title resolves to (`internal/dict/classify`), neutral base when it
      resolves to none, plus a random suffix. Pure function under test — same base yields
      different handles, and the output always satisfies the shape the route accepts.
      Collision resolves by re-minting against the store, the shape
      `internal/identity/accounts` already uses for usernames.
- [x] 1.7 Wire minting into `PutVisibility`: minted on the first join only, never
      recomputed. Tests — leaving and rejoining keeps the handle; a new CV in a different
      category keeps the handle; the minted handle never contains the account's `username`.
- [x] 1.8 Retire `talent_network_public_id`: drop the column and every read of it once the
      handle serves the route (task 4.2 and 5.3). Two public identifiers for one page is a
      drift, not a fallback.

## 2. The public projection

- [x] 2.1 `talentnetwork.ProjectCard(resumeextract.Structured) Card`. Placed in
      `talentnetwork`, NOT beside `Anonymous()` in `resumeextract`: the rule it encodes
      belongs to the catalogue, and `resumeextract`'s own projections are what this change
      is retiring. A whitelist per design.md: `total_years`, dictionary-resolved `skills`,
      and per role the classified seniority/category, the period and the resolved stack.
      **REVISED while implementing:** languages, certifications and education are OUT —
      all three are free text and the only text→level dictionary lives in
      `internal/job/jobfacts`, block `job`, layer 5, which `candidate` may not import.
- [x] 2.2 The invariant test, and it is the point of this change: over a fixture CV whose
      prose, titles, project names and institution all carry a distinctive employer token,
      assert that token appears **nowhere** in the marshalled `Catalog()` output. Cover
      separately: an employer named only in `summary`; only in a role's `highlights`; only
      in a title (`"Backend Engineer @ X"`); only in a project `name`; only in an
      education `institution`.
- [x] 2.3 A title that resolves to neither category nor seniority still yields a role
      entry carrying its period and stack. A skill outside the dictionary is dropped while
      its resolved neighbours survive.
- [x] 2.4 Delete `Structured.Public()`, its `Public` struct, and the handler branch that
      selected it — `public` no longer exists, so nothing may reach them. Confirm with
      `deadcode -test -tags=integration,llmlive ./...` that nothing else did.

## 3. The catalogue package

- [x] 3.1 Create `internal/candidate/talentnetwork/` and **add it to the block table in
      `internal/platform/arch/layering/blocks.go`** — a package in neither column fails
      both guards. Package doc states the in-memory-snapshot decision and the seam for
      when membership outgrows it (design.md, "The catalogue is projected in memory").
- [x] 3.2 The read: the membership + stamp-gate predicate as a sqlc query over `users`
      (+ `user_profiles` for the curated facets, LEFT JOIN — a member may have no profile
      row). `make sqlc`.
- [x] 3.3 The snapshot: project every read row through `Catalog()`, hold it immutably,
      refresh on a TTL. Test that a refresh replaces the snapshot atomically — a concurrent
      reader never sees a half-built one.
- [x] 3.4 Filters over the snapshot: category, seniority, skills, timezone region, city,
      years, language. Values within one filter are OR, different filters are AND, an
      absent filter equals an empty one. Test each of those three rules separately.
- [x] 3.5 Order and paging: freshness of the structured extract descending, tie-broken by
      the member's handle. Test that walking every page of a set containing a
      timestamp tie returns each member exactly once — a test that only checks the first
      page cannot see this bug.
- [x] 3.6 The single-card read: by handle, re-checking membership against the database
      rather than the snapshot, so a departure takes effect immediately.

## 4. The public API

- [x] 4.1 `GET /api/v1/talent` in `internal/api/handler/`: unauthenticated, the list
      envelope (`data` + `meta`), `meta.total` behind the same predicate as the page.
      Unread parameters reported in `meta.ignored_params` — this endpoint owns its own
      vocabulary, like `/companies` does, and must not borrow `search.UnknownParams`.
- [x] 4.2 `GET /api/v1/talent/{handle}`: 404 with an identical body for a non-member, a
      handle nobody holds, and a malformed handle. Test all three answer the same, so the
      route cannot be used to probe for accounts — including a request that spells an
      account's `username`.
- [x] 4.3 Attach `internal/api/ratelimit` to both routes, and assert in a test that the
      limiter is on these paths — the guard that already exists for "limiters on REAL
      routes" is the pattern to follow.
- [x] 4.4 The card response sets a short `Cache-Control` and `X-Robots-Tag: noindex`.
      Documented in `web/src/lib/docs/api-spec.ts` (the source `docs/API.md` is generated
      from) and regenerated. **`web/static/openapi.yaml` deliberately NOT touched:** its
      own description scopes it to job search and says it is what the custom GPT imports
      as an Action, so adding a catalogue of people there would hand every GPT user a
      bulk reader of candidate profiles — the exact extraction the rate limit exists to
      slow down. Publishing it belongs with a decision about who may read it in bulk, not
      with this change.
- [x] 4.5 GENERATED, not hand-written: `cmd/gen-contracts` gained a `talentnetwork`
      entry over `card.go` alone (`catalogue.go` holds the serving machinery, none of
      which crosses the wire). The types are renamed `CandidateCard` / `CandidateRole` /
      `CatalogueMember` because every contract lands in ONE TypeScript file and `Card` was
      already taken by jobview's job card — a second `export interface Card` is a build
      error at best and a silently shadowed type at worst.

## 5. The web surface

- [x] 5.1 Port from `/Users/i_strelov/Projects/freehire-recruit/web/src/lib/`:
      `timezoneCountry.ts` (+ its test — it carries the six renamed tzdata aliases,
      including `Asia/Calcutta`, the largest single group), `candidateQuery.ts` (+ test),
      `CandidateCard.svelte`. Adapt each to this repo's `$lib/ui` and the catalogue's
      narrower card, and keep the tests.
- [x] 5.2 `web/src/routes/talent/` — the list. Filters read from and write back to
      `page.url`, never to local state; paging as real `<a href>` links, not the design
      system's `Pager` (its own doc comment says it does not touch the URL). Empty state
      and skeleton from the design system.
- [x] 5.3 Replace `web/src/routes/talent-network/[publicId]/` with `web/src/routes/talent/[handle]/`:
      the catalogue card — no name, no company names, no prose. Its `+page.server.ts` keeps
      the 404-on-non-member behaviour. The old route goes; nothing has linked to it.
- [x] 5.4 `web/src/routes/my/talent-network/+page.svelte`: the three-option picker becomes
      one toggle. Keep the echoed-value behaviour (trust the PUT response, not the click)
      and keep the "a link you have shared cannot be unshared" warning.
- [x] 5.5 Add `{ href: '/my/talent-network', label: 'Talent Network' }` to
      `web/src/lib/accountNav.ts` with its icon in `accountNavIcons.ts`, and the invitation
      block on `/my/profile` — non-member sees what joining publishes and withholds, member
      sees their state and a link to their own card. **Without this task the catalogue can
      never have members.**
- [x] 5.6 SEO: the list page indexable, the card page `noindex`. Verify the card is not
      under `my/+layout.svelte`'s blanket `noindex` by accident — it must carry its own.

## 6. Documentation and housekeeping

- [x] 6.1 `internal/candidate/talentnetwork/AGENTS.md`: what the package is, the projection
      rule (dictionary terms, numbers and dates only), the snapshot and its seam, and what
      it may import.
- [x] 6.2 Update the module table in the root `CLAUDE.md`/`AGENTS.md` with the new package,
      and check `pnpm check:links` passes — the table is the map an agent follows.
- [x] 6.3 Both stale changes archived with a SUPERSEDED banner naming what replaced them
      and what was never true even then (the overlay panel that shipped as a page).
      Archived `--skip-specs` deliberately: syncing their specs would write the retired
      tri-state into `openspec/specs/` only for this change to overwrite it, and a spec
      that is wrong for a day is a spec somebody reads. This change's own specs are what
      land there when it is archived. Their task lists are left as written — rewriting a
      completed change's record to match later code turns history into a quieter, second
      copy of the specs.

## 7. Verification

- [x] 7.1 `gofmt -l .` prints nothing; `go build ./...`, `go vet ./...`, `go test ./...`
      and `go vet -tags=integration ./...` all pass.
- [x] 7.2 `pnpm --dir web check` (0 errors), `lint`, `build` and the full vitest suite
      (1707 tests) pass. The fresh worktree did need `svelte-kit sync` first — without it
      even `gen:api-docs` fails, with a "Tsconfig not found" that names neither.
- [ ] 7.3 Manual browser pass against a running app: join from `/my/profile`, see yourself
      appear in `/talent` as a visitor in a logged-out window, confirm no employer name is
      anywhere on the page or in the API response, leave, and confirm the card 404s
      immediately. The equivalent task on the previous Talent Network change was left
      undone, which is how the unreachable toggle shipped.
