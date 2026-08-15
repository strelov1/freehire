## 1. The plan projection

- [x] 1.1 `extension/lib/applyPlan.ts`: `PlanItem` + `buildPlan(fields: FramedField[]): ApplyPlan` — one item per question, required/optional split, `answered` from the field's current value, and the required counter (count, total, percent). Pure; no DOM, no messaging.
- [x] 1.2 `applyPlan.test.ts`: a grouped question counts once, an already-answered field counts answered, the counter counts required only, a form with no required questions yields no counter, an empty form yields an empty plan.

## 2. Page-side primitives

- [x] 2.1 `extension/lib/protocol.ts`: `REVEAL_FIELD { label, frame, form, focus }` / `REVEAL_RESULT { found }` / `FORM_CHANGED`, and an optional `reveal` on `FILL_BY_LABEL`. Document each in the wire contract's own idiom.
- [x] 2.2 `extension/lib/form.ts`: `revealField(doc, {label, form, focus})` — find the question (reusing `findQuestion`), scroll its first control into view centred, outline it inline for ~600 ms, restore the previous inline style, focus when asked. Returns whether it found the question.
- [x] 2.3 `form.test.ts`: reveal finds a question by label, restores the inline style it changed, focuses only when asked, and reports `found: false` for a label the document does not carry.
- [x] 2.4 `extension/entrypoints/content.ts`: handle `REVEAL_FIELD`; honour `reveal` on `FILL_BY_LABEL` (reveal, then fill).
- [x] 2.5 `extension/entrypoints/content.ts`: one delegated `input`/`change` listener, debounced 400 ms, posting `FORM_CHANGED` to the panel. Extract the debounce so it is testable.

## 3. The checklist in the panel

- [x] 3.1 `extension/entrypoints/sidepanel/ApplyPlan.svelte`: renders an `ApplyPlan` — counter + progress bar (hidden when there are no required questions), Required and Optional groups, per-item answered/unanswered/filling state, and an action per item that emits "reveal this one". Design-system primitives only.
- [x] 3.2 `App.svelte`: read the page's form when the Match tab opens and on `FORM_CHANGED`, build the plan, render `ApplyPlan` when the page shows an application form (`looksLikeApplication`), and clear it on page change.
- [x] 3.3 `App.svelte`: item action → `REVEAL_FIELD` with focus; a `found: false` answer says so in the panel rather than doing nothing.

## 4. The walk

- [x] 4.1 `extension/lib/walk.ts`: the pure step reducer — given a plan and a list of labels to work through, yield the next step, apply an outcome, and honour a stop. No timers, no messaging.
- [x] 4.2 `walk.test.ts`: order is preserved, a stop ends it with values kept, a label absent from the plan is skipped without ending the walk, and the counter advances per applied step.
- [x] 4.3 `App.svelte`: drive the deterministic filler through the walk — one `FILL_BY_LABEL` with `reveal` per step, ~300 ms between steps, ticking the checklist as it goes.
- [x] 4.4 `App.svelte`: play the agent's report back through the same walk — match each reported label against the questions just read, reveal, tick; skip a label the page no longer carries.
- [x] 4.5 `App.svelte`: the Autofill button becomes Stop while a walk runs; stopping ends the walk and leaves written values in place. The closing summary names what could not be answered.

## 5. Verification

- [x] 5.1 `npm run test` (extension), `npx svelte-check`, `npm run lint`, `npm run build` — all clean.
- [x] 5.2 Load the built extension and walk a real ATS application form end to end (Greenhouse or Ashby): checklist appears, walk scrolls and ticks, stop works, an unanswered item scrolls and focuses, typing on the page moves the counter.
- [x] 5.3 Update `extension/AGENTS.md` — the panel's layout list and the wire contract now include the plan, the walk and the three new messages.
