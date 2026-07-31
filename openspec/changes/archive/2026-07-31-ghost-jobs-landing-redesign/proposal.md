## Why

`/features/ghost-jobs` is the page a reader reaches from the "How this works" link in the
job page's ghost row — they arrive mid-question, having just seen a mark they did not
understand. What they find is eight stacked sections and roughly 2,500 words of uniform
small grey text, with no illustration of any kind: every mechanic the signal runs on is
prose. The two things that actually answer their question — which criteria fired, and why
the warning is not stronger — are the hardest things on the page to extract.

The feature is inherently quantitative and structural: four criteria, two tiers, two
independent gates, a published prevalence range. All of it is drawable, and none of it is
drawn.

## What Changes

- **Eight sections become five**, plus a collapsed FAQ and a shortened closing CTA. The
  "what you actually see" section and the tier-explaining paragraph are absorbed by the
  new gate section; the "line we do not cross" section is compressed from a full section
  into a two-line contrast in the hero, keeping its deliberate placement above the
  mechanics.
- **Six diagrams are added.** A 100-cell prevalence waffle showing the 18–27% range as a
  band rather than a fake-precise number; one diagram per criterion; and a 2×2 gate matrix
  that makes "structural evidence can never reach `likely`" geometric instead of a clause
  in a sentence.
- **The static preview becomes an interactive sandbox.** Four criterion toggles and a
  three-position contributor control drive the *real* `GhostBadge` and `GhostChecklist`,
  fed a `Ghost` assembled on the fly. The reader discovers the ceiling by failing to reach
  `likely` with structural criteria alone.
- **`ghostLevel()` joins `$lib/ghost.ts`**, and `CONVERGENCE` / `WITNESS_GATE` move there
  from `ghostSignals.ts` with a re-export left behind. The gate rule exists nowhere in the
  frontend today — the landing asserts it in English prose that no test can check. This
  trades an unverifiable claim for a unit-tested function and consolidates a mirror
  currently smeared across two modules.
- **`SignalExplainer` gains a `gist` field.** ~25 words visible per criterion; the existing
  `why`, which carries the load-bearing honesty caveats, moves under a disclosure rather
  than being cut. Visible copy drops from ~2,500 words to ~515, with ~1,000 more preserved
  in the DOM behind disclosures.
- **Twelve disclosures use native `<details>`** behind a thin styling wrapper, so the
  collapsed FAQ text stays in the DOM for `faqPageJsonLd` and works without client JS under
  SSR. `GhostChecklist` is deliberately not refactored onto it.
- No API, worker, or database change. No copy claim is strengthened: every caveat the page
  makes today it still makes, either visibly or one disclosure away.

## Capabilities

### New Capabilities

- `ghost-jobs-feature-landing`: the composition of the `/features/ghost-jobs` page — its
  section order, the six diagrams and what each is allowed to depict, the copy budget and
  disclosure discipline, the colour rule the diagrams inherit, and the page's accessibility
  treatment of decorative graphics.

### Modified Capabilities

- `ghost-job-signal`: the requirement *"The explaining page previews the signal with the
  components that render it"* grows. The preview becomes interactive, and the frontend gains
  a tested mirror of the gate rule so the page can demonstrate the level ladder rather than
  assert it. The existing constraint — render the real components, never a copy or a
  screenshot — is retained unchanged and now also binds the sandbox.

## Impact

**Frontend, `web/` only.**

- `web/src/lib/ghost.ts` — gains `ghostLevel()`, `CONVERGENCE`, `WITNESS_GATE`.
- `web/src/lib/ghostSignals.ts` — gains `gist` on `SignalExplainer`; its two constants
  become re-exports from `$lib/ghost`.
- `web/src/lib/components/GhostLandingView.svelte` — restructured; drops from 256 lines as
  markup moves into the new subdirectory.
- `web/src/lib/components/ghost/` — new: the six diagrams, the sandbox, the pure-geometry
  module for the waffle and matrix, and a `Disclosure.svelte` styling wrapper.
- Tests: `ghost.test.ts` and `ghostSignals.test.ts` extended; `ghostDiagrams.test.ts` and
  `ghostFaq.test.ts` added. The ghost FAQ has no test today despite feeding JSON-LD.

**Deliberately untouched:** `internal/ghost` and every worker; `GhostBadge.svelte` and
`GhostChecklist.svelte`, which are product components imported by the job card; the
`GHOST_FAQ` array itself, which stays intact so the visible block and the JSON-LD payload
cannot disagree; and the route file's `<Seo>` and structured-data payload.

**Risk to resolve first:** the "every criterion has a diagram" assertion must import a
registry that pulls in `.svelte` files, and this vitest setup tests pure `.ts`. If that
import does not resolve, the registry stays the single source and exposes its keys for the
test rather than a duplicated code list being introduced.
