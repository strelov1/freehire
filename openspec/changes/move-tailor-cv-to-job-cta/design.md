## Context

The job page has two independent CTA stories today and they were written apart.

`internal`-side there is nothing to change: this is entirely `web/`. The pieces are

- `web/src/lib/autoApplyButton.ts` — a pure function, `jobCtaPlan(state)`, that ranks the
  two buttons. It is where the rule "never two primaries; one wherever an action remains"
  is written down and unit-tested without mounting Svelte.
- `web/src/lib/components/JobView.svelte` (~1100 lines) — renders the CTA row three times
  (title row, pinned header, phone's sticky bar) through two snippets, `applyCta` and
  `autoApplyCta`, so one link never reads at two ranks on one page.
- `web/src/lib/components/MatchSummary.svelte` — the sidebar's fit-analysis block. It
  fetches `GET /api/v1/jobs/:slug/match-analysis` itself, and renders one of four branches:
  an upload prompt, a cached-analysis card, a spent-allowance message, or the
  `Tailor my CV` button with a remaining-count caption under it.
- `web/src/lib/components/JobMatch.svelte` — the sidebar's deterministic coverage block. It
  is the only consumer of `MatchSummary`, and renders it twice: once inside its guest state
  (purely to show the guest the offer) and once in its real-match state. It is itself
  rendered from THREE places, not one: the job page, the tracking drawer's `fit` tab, and
  the swipe deck's card.
- `web/src/lib/components/ConfirmTailorDialog.svelte` — the pre-flight dialog, mounted once
  in the root layout, driven by `askConfirmTailor(slug)` in
  `web/src/lib/confirmTailorDialog.svelte.ts`. It fetches the deterministic job match AND
  the tailoring allowance itself, and already states the cost, the spent case and the
  not-in-plan case with an upgrade link.

Two facts decided most of this design. First, the dialog already carries every word the
sidebar says about the allowance — so moving the button does not orphan that information,
it removes a duplicate. Second, only ONE of `JobMatch`'s three callers has a match analysis
to hand it — the job page — so the data can travel down a prop rather than through a new
store, and the other two pass `null` and say in a comment why.

(The design first claimed `JobMatch` had a single caller. It has three; the two extra ones
were found by `pnpm check` after the prop was made required, not by reading. The claim came
from a `grep … | head`, whose truncation is invisible.)

## Goals / Non-Goals

**Goals:**

- `Tailor my CV` renders in all three CTA positions and is the page's only brand-fill button.
- The auto-apply rank machinery keeps working: the phone's sticky bar and the
  `Apply`/`Show origin` wording are still decided by whether auto-apply can be started.
- The page makes no more network calls **per render** than it does today — one read of the
  match analysis, not two. It does make that one read on strictly more page loads; see the
  decision below, which is a cost this change accepts rather than avoids.
- The behaviours the sidebar button had — confirm-before-navigate, guest sign-in, withheld
  without a CV — survive the move unchanged.

**Non-Goals:**

- Changing what tailoring does, what the workspace does, or the plan limits behind it.
- Redesigning the confirmation dialog, including its no-CV copy.
- Touching the deterministic coverage block (`JobMatch`'s teaser, chips, claim and avoid
  flows) — only its render of `MatchSummary` changes.
- Any backend, API or wire-shape change. `MatchAnalysisResponse.tailor_allowance` stays on
  the response; the sidebar simply stops reading it.

## Decisions

### `primary` is split into two questions, not merely negated

`JobCtaPlan` today carries `primary: boolean` on both controls, and that flag answers two
different questions at once: *is this button brand-filled* and *is auto-apply the offered way
to apply*. The second reading is load-bearing in two places that have nothing to do with
colour — `JobView.svelte`'s phone sticky bar picks its button with `cta.autoApply?.primary`,
and the quiet strip shows the phone-only origin link under the same condition.

If `primary` were simply set to `false` everywhere, both would silently pick the wrong
control: the phone would lose auto-apply entirely (it has no other home below `lg`) and the
origin link would vanish with it.

So: `external.primary` is deleted (the link is outline in every state, so the field would
only ever be `false`), and `autoApply.primary` becomes `autoApply.leads`, documented as "is
auto-apply the offered way to apply". Its truth table is unchanged — true only in `idle` —
which is why the mobile bar and the strip keep working by renaming a reference.

*Alternative considered:* keep `primary` and add `tailor: {primary: true}` to the plan.
Rejected: it leaves the ambiguity in place and gives `jobCtaPlan` a third entry that is a
constant, which a reader would reasonably expect to vary.

### The tailoring action moves to `JobView`, the fetch moves with it

`startTailoring()` (await `askConfirmTailor`, then `goto('/tailor/[slug]')`) and the
guest→`promptSignIn` branch move from `MatchSummary` to `JobView`, beside the button they
now belong to. `JobDrawer.svelte` keeps its own copy of the same three lines, as it does
today — it is a different surface with a different label, and the shared part is already
factored out into `askConfirmTailor`.

The button's one data dependency is `has_cv`. Rather than a second
`getMatchAnalysis` call, `JobView` takes over the fetch that `MatchSummary` does today —
same slug-guarded `$effect`, same `isAuthenticated()` short-circuit — and passes the
response down: `<JobMatch {job} {matchAnalysis} />` → `<MatchSummary {matchAnalysis} />`.
`MatchSummary` becomes render-only.

*Alternative considered:* a per-slug `.svelte.ts` store, the shape
`confirmTailorDialog.svelte.ts` uses. Rejected for now: that shape exists because a dialog
mounted once in the root layout has no props path to its callers. Here there is one, exactly
one level deep, and a global keyed by slug would need lifecycle care that a prop does not.
`JobMatch` passing through a prop it does not read is the visible cost, and it is one line.

*Alternative considered:* drop the `has_cv` gate and let the backend's 409 ("add a résumé
first", per `cv-tailoring`'s own requirement) answer it. Rejected: that 409 surfaces on the
tailoring workspace after a navigation, which is a worse way to learn the same thing than a
prompt the sidebar is already showing.

### The read now fires on every signed-in page load, and that is a real cost

`MatchSummary` mounted only in `JobMatch`'s `ready` state — signed in, profile loaded,
profile carrying skills, the job carrying a `skills` facet, and the deterministic match
already back (`resolveMatchState`). Its own effect then short-circuited for a guest. In
`JobView` the same effect fires for **every signed-in reader on every posting**: a reader
with no profile skills, or a non-technical posting with no `skills` facet, used to cost zero
calls to `GET /jobs/:slug/match-analysis` and now costs one.

That endpoint is not free — `GetJobBySlug`, the CV timestamp, the language read, the cached
analysis and two allowance computations — and this host's bottleneck is already its crawl
fleet. It is accepted rather than avoided because the alternative is worse in kind: the CTA
gate needs `has_cv`, and the conditions that used to suppress the read are all conditions
about the SIDEBAR's content, none of which say anything about whether the reader has a CV.
Gating the page's primary CTA on whether the sidebar happened to render would make the
button appear and disappear for reasons no reader could connect to it.

### The fit-analysis allowance no longer pre-empts the button, deliberately

The old block suppressed the button when **either** allowance would refuse —
`refuses(allowance) || tailorRefused` — and named whichever did. `ConfirmTailorDialog` reads
only the tailoring one. So a reader whose *fit-analysis* allowance is spent while their
tailoring allowance is not now gets the button, the dialog, and the workspace, where the old
sidebar told them their plan does not include job analyses.

That is the correct outcome, not a gap left behind. The button spends a TAILORING session;
the fit analysis is a different feature, and the tailoring workspace reads the cached
analysis best-effort (`api.getMatchAnalysis(slug).catch(() => null)`) and proceeds without
one. Hiding the page's single primary call to action because an unrelated feature's daily
count is spent would refuse somebody an action that works. The old conflation made sense
only while the button lived inside a block whose subject was the fit analysis.

What the reader loses is the explanation: the sidebar used to name the refused feature and
link the plan page. Tailoring still works, so there is nothing to explain at the moment of
the click — and `/my/plan` is where a spent allowance is reported in full.

### The sidebar loses both allowance branches, not just the button

With the button gone, `MatchSummary`'s spent-allowance branch and its
`N of today's CV tailorings left` caption describe a control that is no longer there. Both
are already said by `ConfirmTailorDialog` — including the not-in-plan case and the upgrade
link — at the moment the reader commits, which is the better place for them. They are deleted
rather than followed up to the CTA row: a caption under a button in a horizontal row has
nowhere to go, and a count stated in two places is a count that can disagree.

This also means `MatchSummary` stops reading `allowance` and `tailor_allowance`, and drops
its imports of `$lib/allowance` and `PlanLimitLink`.

### The guest render of `MatchSummary` is deleted, not emptied

`JobMatch.svelte`'s guest branch renders `<MatchSummary>` for one reason: to put the offer
in front of a guest. `MatchSummary` never fetches for a guest, so with the button gone that
render produces an empty bordered section. The branch goes; the CTA row's button carries the
guest offer now, and more prominently than the sidebar did.

### Order in the row: brand fill last

`[Auto-apply] [Apply] [Tailor my CV]`, with `Tailor my CV` rightmost in all three positions.
The apply link is where the eye already expects it, and the page's loudest button lands at
the end of the row rather than in its middle. The phone's bar carries two controls side by
side, each taking half the width: `Tailor my CV` first, then whichever apply control leads.

## Risks / Trade-offs

- **Three buttons plus Save crowd the pinned header at `lg`.** → The pinned header's own
  comment already commits it to carrying the same pair as the title row, so the third button
  belongs there by that rule rather than by preference. Save is already icon-only there. If
  it still overflows at exactly `lg`, the fix is the pinned header's own layout, not a
  different set of buttons — a header that disagrees with the title row is the failure this
  avoids.
- **The button is now reachable by a reader whose CV read has not landed yet.** → The window
  is one request, and the confirmation dialog stands between the click and any navigation;
  its no-match branch already says "add a CV to your profile".
  The row still jumps for that reader — the button renders and is then REMOVED when the
  response says there is no CV. Withholding it until the fetch resolves would move the jump
  onto everybody instead, which is the wrong trade: almost every signed-in reader has a CV,
  and a primary CTA that fades in on every page load is worse than one that leaves on the
  rare page load where it was never going to work. The sidebar has the same shape of gap —
  its `{#if noCv || analysis}` gate means the block appears after the read rather than
  showing a skeleton.
- **Two full-width buttons make the phone's sticky bar taller.** → They share one row at half
  width each, not two stacked rows, so the bar's height is unchanged.
- **The visual hierarchy change is not A/B tested.** → It is a deliberate product decision
  (tailoring is the metered feature; applying sends the reader away), taken with the user.
  Rollback is a revert of one commit: no data migrates and no wire shape moves.

## Migration Plan

Pure frontend. Deploy is the ordinary `release.sh` blue/green flip; there is no migration,
no Meilisearch settings change and no worker involved. Rollback is a revert.

## Open Questions

None outstanding. Hierarchy, sidebar contents and phone placement were settled with the user
before this change was written.
