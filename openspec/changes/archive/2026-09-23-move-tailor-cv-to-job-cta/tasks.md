## 1. The CTA rank plan

- [x] 1.1 In `web/src/lib/autoApplyButton.test.ts`, rewrite the `jobCtaPlan` cases against the
  new shape: `external` carries only `label`, and `autoApply` carries `leads` in place of
  `primary` — true only for `idle`, false for every quiet state and absent where auto-apply is
  hidden. Cover all six `AutoApplyButtonState` kinds. Tests fail to compile (RED).
- [x] 1.2 In `web/src/lib/autoApplyButton.ts`, drop `primary` from `JobCtaPlan['external']`,
  rename `autoApply.primary` to `leads`, and rewrite the doc comments on `JobCtaPlan`,
  `quiet()` and `jobCtaPlan` so they describe "the offered way to apply" rather than the brand
  fill, naming `Tailor my CV` as the page's one primary CTA. GREEN.

## 2. The sidebar block stops offering the action

- [x] 2.1 Add `web/src/lib/components/MatchSummary.spec.ts` (mounted, the `JobMatch.spec.ts`
  idiom): given a `matchAnalysis` prop it renders the cached-analysis card; given
  `has_cv: false` it renders the `Upload a CV to analyse` prompt; in no case does it render a
  `Tailor my CV` button, a remaining-tailorings count or a spent-allowance message; and it
  issues no `getMatchAnalysis` call. RED.
- [x] 2.2 Rewrite `web/src/lib/components/MatchSummary.svelte`: take `matchAnalysis` as a prop
  instead of fetching, delete the button, the `remaining(...)` caption and the
  `allowanceRefused` branch, and drop the imports they orphan (`goto`, `askConfirmTailor`,
  `promptSignIn`, `isAuthenticated`, `api`, `PlanLimitLink`, `$lib/allowance`, `SquarePen`).
  Replace the component's header comment — it currently explains the guest button. GREEN.

## 3. The sidebar passes the read through

- [x] 3.1 Extend `web/src/lib/components/JobMatch.spec.ts`: the guest state renders no
  `Tailor my CV` button and no empty analysis section, and the component no longer calls
  `getMatchAnalysis` itself. RED.
- [x] 3.2 In `web/src/lib/components/JobMatch.svelte`, add the `matchAnalysis` prop and pass it
  to `MatchSummary`; delete the guest-state `<MatchSummary>` render at the former line 350-357
  together with the comment explaining it. GREEN.

## 4. The CTA row takes the button

- [x] 4.1 Extend `web/src/lib/components/jobActionStrip.test.ts` (the source-text audit) with
  the CTA-row composition it does not yet cover: `ctaGroup`, the pinned header's button row and
  the phone's sticky bar each render `tailorCta`; `tailorCta` is the only snippet passing
  `variant="primary"`; `applyCta` passes `variant="outline"` unconditionally; the sticky bar
  renders exactly one apply control beside it. RED.
- [x] 4.2 In `web/src/lib/components/JobView.svelte`, add the `tailorCta(size, className)`
  snippet, the `startTailoring()` handler and the guest→`promptSignIn` branch moved from
  `MatchSummary`, plus the slug-guarded `getMatchAnalysis` effect and the `hasCv` derivation
  that withholds the button. GREEN.
- [x] 4.3 Render `tailorCta` last in `ctaGroup`, last in the pinned header's row, and first in
  the phone's sticky bar beside the single leading apply control; switch `applyCta` to a fixed
  `outline` variant, `autoApplyCta` to a fixed `secondary` variant, and the two
  `cta.autoApply?.primary` reads (the sticky bar's choice and the quiet strip's phone-only
  origin link) to `cta.autoApply?.leads`. Pass `matchAnalysis` to `<JobMatch>`. GREEN.
- [x] 4.4 Update the block comments the change falsifies: `applyCta`'s and `autoApplyCta`'s
  ("how loud it is comes from the CTA plan"), `ctaGroup`'s, the pinned header's "carries the
  SAME pair", and the sticky bar's "carries whichever of the two controls the plan made
  primary".

## 5. Verification

- [x] 5.1 `cd web && pnpm test` — the whole vitest suite, both projects.
- [x] 5.2 `cd web && pnpm check` and `pnpm lint` over the changed files.
- [x] 5.3 Run the app and look at a real job page: the three CTA positions at `lg`, the
  pinned header after scrolling, and the phone width's sticky bar. Done at 1440px and 390px
  against the production API through the dev proxy — order, treatments and bar height are as
  designed. **Signed-in (has CV / no CV) and the auto-apply button were NOT reached**: that
  needs a session the dev proxy cannot carry to localhost, and no local backend was running.
  Those branches rest on the tests and the type checker alone; the PR says so, and they are
  the first thing to look at after deploy.
