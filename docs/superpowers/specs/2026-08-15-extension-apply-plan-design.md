# Extension: a visible plan for filling an application form

Date: 2026-08-15
Status: approved

## Problem

Autofill is a black box. The user presses one button and gets a sentence: *"Autofilled 9
fields — review before submitting."* Nothing says which fields those were, what is still
empty, or whether the form is now ready to submit. On a real ATS form — twenty questions
across two screens — the user has to scroll the whole thing to find out, which is the work
autofill was supposed to remove.

Two things are missing: a **standing account of the form** (what it asks, what is answered)
and **visible progress** while the filling happens.

## Solution

The Match tab grows an application-form panel: the form's questions as a checklist, a
required-fields counter, and an autofill pass that walks the page field by field in view.

```
Application form                       6/7 · 86%
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░

Required
  ✓ First name          ✓ Email
  ✓ LinkedIn            – City
Optional
  – Website             – X (fka Twitter)
```

Three behaviours:

1. **The checklist stands on its own.** It appears whenever the open page shows an
   application form, before anything is filled, and it stays accurate as the user types
   into the page themselves.
2. **Autofill is a walk, not a batch.** Each field in turn: the page scrolls to it, the
   field is outlined, the value lands, the panel ticks it off and the counter moves. The
   Autofill button becomes **Stop** for the duration.
3. **The remainder is reachable.** A question autofill could not answer stays in the list
   as a live target: clicking it scrolls the page there and puts the cursor in it.

### Why a walk

A batch fill is faster in wall-clock and worse to watch: values appear off-screen, and the
user still has to audit the form afterwards because nothing showed them what changed. The
walk makes the audit happen as it goes — and it is the only version where "it filled the
wrong thing" is caught by the person watching rather than after submission.

The pause between fields is ~300 ms. It exists for the eye, and it is the whole reason a
walk reads as progress rather than as a flicker.

## Architecture

Nothing new crosses the network. The panel already reads the page's form and writes values
back through the same relay; this feature is the panel using them per-field instead of
once, plus a checklist rendered from what it reads.

```
sidepanel/App.svelte
  └─ ApplyPlan.svelte          the checklist + counter (new)
        ↑ questions              ↓ fill one / reveal one
  background.ts (relay, unchanged shape)
        ↕
  content.ts
     ├─ collectQuestions / extractForm      (existing — the read)
     ├─ fillByLabel                          (existing — the write)
     ├─ revealField                          (new — scroll + outline + focus)
     └─ form-change notifier                 (new — user edits reach the panel)
```

### The unit is the question

`internal` to this design: the list is built from `collectQuestions`, the same unit the
filler already works in, so a question rendered as 29 checkboxes is one line in the
checklist and one tick — not 29. The panel addresses a question by its label plus
`frame`/`form`, exactly as `LabelFill` already does.

### Components

**`ApplyPlan.svelte`** (panel). Renders a `PlanItem[]`: label, required, status
(`filled` | `empty` | `filling`). Owns no page access — it emits "reveal this one" and
renders what it is given. Splits Required and Optional; the counter counts required only,
because that is what gates submission.

**`applyPlan.ts`** (panel lib, pure). Turns `FramedField[]` into `PlanItem[]`: drops
questions that are not answerable (`hidden`, submit-like), decides `filled` from the
field's current `value`, and computes the counter. Pure over its input, so it is unit
tested without a DOM — the same discipline `scraper.ts` and `form.ts` keep.

**`revealField`** (content). Scrolls the question's first control into view
(`block: 'center'`), outlines it for ~600 ms, and optionally focuses it. The outline is
inline style on the element, restored afterwards — no stylesheet injected into a page we
do not own.

**Form-change notifier** (content). One delegated `input`/`change` listener on the
document, debounced 400 ms, posting `FORM_CHANGED` to the panel, which re-reads the form.
Delegation, not per-field listeners: an ATS form re-renders constantly, and per-field
listeners would leak with every re-render.

### Wire additions (`lib/protocol.ts`)

- `REVEAL_FIELD { label, frame, form, focus }` → `REVEAL_RESULT { found }`
- `FILL_BY_LABEL` gains `reveal?: boolean` — when set, each fill scrolls and outlines
  before writing. Absent for the agent's own tool-driven fills, which must stay silent.
- `FORM_CHANGED` — content → panel, no payload; the panel re-reads.

### The walk

The panel already gets its fill plan two ways — the agent's `/me/autofill/run` and the
deterministic fallback. Both end in a list of `(label, value, frame, form)`. The walk is a
loop over that list: one `FILL_BY_LABEL` per item with `reveal: true`, then a `300 ms`
pause, updating the checklist between steps. A `stop` flag checked between steps ends it.

The agent path currently fills the page itself, server-side, through the tool relay. It
keeps that behaviour: for the agent path the panel walks the **report** it gets back
(`{filled, deferred, unmapped}`, labels only) rather than re-filling — each reported label
is matched against the questions the panel just read, revealed, and ticked off. A label the
page no longer carries is skipped, not an error: the report describes a form that may have
re-rendered since.

The deterministic path walks its own plan and fills as it goes, where each step carries
`frame`/`form` and needs no matching.

## Error handling

- **No form on the page** — no panel, no counter. The Match card and Autofill button are
  unchanged; a page with no application is not a form with zero fields.
- **A question vanishes mid-walk** (the form re-rendered, or a step moved) — `fillByLabel`
  already answers `not_found`; the walk marks that item `empty`, does not stop, and the
  final line names what it missed.
- **A custom-widget dropdown** — already `deferred_combobox`; it stays in the list as an
  unfilled required item, reachable by click, and the closing line says how many.
- **The user navigates away mid-walk** — the panel's existing page-change handler resets
  the plan, and the walk's stop flag is set from the same place.

## Testing

- `applyPlan.ts` — unit tests over fixture `FramedField[]`: counter arithmetic, required
  vs optional split, a group counted once, an already-answered field counted as filled.
- `form.ts` `revealField` — jsdom test that it finds the question and restores the style it
  changed.
- The walk's stop/ordering logic — unit tested as a pure reducer over steps, not by
  driving a real page.

Existing suites (`form.test.ts`, `combobox.test.ts`) must stay green; the fill primitives
themselves do not change.

## Out of scope

- Asking the user for a missing answer inside the panel (considered, deferred: the
  remainder is reachable by click, which is enough to finish a form).
- Writing answers back into the profile for next time.
- Multi-page ATS flows ("Continue to the next page"): the plan describes the page in front
  of the user, not the application as a whole.
