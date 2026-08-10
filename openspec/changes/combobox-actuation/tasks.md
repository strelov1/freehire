## 1. Establish that widgets can be driven at all

- [x] 1.1 Throwaway probe, in a real Chrome with the extension loaded, against the live Stripe and Scout Motors forms: can a react-select be opened, its options read, and one committed with events an injected script can dispatch? Record what each widget responds to.
      **Yes.** `pointerdown`+`mousedown`+`mouseup`+`click` on the `[class*="-control"]` node (not the input) opens it; the same sequence on a `[role="option"]` node commits that option and closes the widget. Verified across Stripe's 13 and Scout's 27 widgets. Two caveats found: React 18 re-renders asynchronously, so opening must be *awaited*, not read back synchronously; and a widget's own listbox is addressed by its `aria-controls` id, which only exists while open — a page-wide `[role=option]` sweep would return a *neighbouring* widget's options.
- [x] 1.2 Find where the committed value is readable — the widget's own displayed value, not the input's `value` — so verification cannot report a success that did not happen.
      The widget's own `[class*="singleValue"]` node. `input.value` is the empty string after a successful commit on every widget probed, and no hidden input mirrors the value. Also: the committed text is not always the option text — the phone-code widget commits `+358` for the option `Åland Islands +358`, so verification compares by one-directional containment (**committed ⊆ selected option**). The reverse direction would let the spike's original failure pass, since `No` ⊆ `Norfolk Island`.
- [x] 1.3 Verdict before building: drivable → continue; not drivable → stop and bring the CDP/`debugger` trade-off back as an explicit decision.
      **VALIDATED** — drivable with ordinary events; the CDP seam stays unpaid. Out-of-scope typeahead widgets identify themselves: they open and offer zero `role=option` nodes (Stripe "Location (City)") or a single "Loading..." node (Scout "School"), so they need no special case.

## 2. Observation: options and grouping

- [x] 2.1 `form.ts`: read a combobox's offered options where they are exposed without opening; report them on the field.
      Read from the listbox the widget's own `aria-controls`/`aria-owns` names — resolved as the id *list* ARIA defines, so a widget pointing at a live-region node beside its listbox still reports its options. A page-wide `[role=option]` sweep would hand a closed question its neighbour's countries on a form of 27 widgets. Options the user cannot see are left out, on the same grounds as hidden controls. A react-select carries none here, having no listbox until opened.
- [x] 2.2 `form.ts`: group controls answering one question (shared `fieldset`/`legend` or equivalently labelled container) into one field carrying the question's label and its options; leave ungrouped anything whose structure is not clearly one question. Unit-test both directions.
      Grouping keys on the container *and* the controls' shared `name` and type: a "Demographic Questions" fieldset wrapping both a Yes/No radio pair and a country checklist asks two questions, and merging them would offer all the options at once and leave the second unanswerable. The "equivalently labelled container" is `role="group"`/`role="radiogroup"` carrying `aria-labelledby`/`aria-label`. Both directions tested, including the four ways grouping declines: text inputs sharing a fieldset, a lone checkbox, a legend-less fieldset, and an unlabelled container.
- [x] 2.3 `read_form` carries the new shape (widget options, grouped field); `fill_simple` fills a grouped field by option text and refuses an option the group does not offer.
      A group reports every already-chosen option as one comma-joined value and accepts that value back, matching the whole string as a single option before splitting it — "Korea, Republic of" is one country, not two the group has never heard of. An option the group does not offer leaves it wholly untouched. Filling ticks and never clears: a group may already carry an answer the user chose by hand.

## 3. `combobox.*` primitives

> Implemented and green, but the code-review pass over this group was cancelled
> before it reported — unlike group 2, these five carry no reviewed-clean stamp.

- [x] 3.1 `combobox.open` — reveal the listbox by label; report open / did-not-open / not-found.
      Presses the widget itself rather than an ancestor matched by class name, since the widget's handler sits on an ancestor and events bubble up to it — `[class*="-control"]` would also match Bootstrap's `form-control` and press the wrong node. Awaits the listbox instead of reading straight back.
- [x] 3.2 `combobox.options` — return the offered option texts; nothing when the widget is not open.
      A closed widget and an open one offering nothing are reported apart, because their remedies differ: the first needs opening, the second is the out-of-scope typeahead. `aria-expanded` outranks the mere presence of a listbox, so a library that keeps its list in the DOM behind `display:none` is not read as open.
- [x] 3.3 `combobox.select` — commit an offered option by acting on the option; refuse a value the widget does not offer.
      Acts on the option node from the widget's own listbox, and waits for the widget to close so a following `verify` reads the committed value rather than the state before it landed.
- [x] 3.4 `combobox.verify` — read back the widget's committed value; a mismatch is a failure, never a fill.
      Compares by containment in one direction — committed ⊆ chosen option — for the reason recorded in 1.2. Reads by walking up from the widget and stopping the moment the subtree holds a second combobox, which makes reporting a *neighbour's* value impossible rather than merely unlikely.
- [x] 3.5 Unit-test the four against a react-select-shaped fixture, including the mismatch path.
      The fixture opens and commits on a *later tick*, so a primitive that merely dispatches-and-reads fails it. Covers the mismatch path with the spike's own case (a question wanting "No" left holding "Norfolk Island"), the shortened-display path ("+358" for "Åland Islands +358"), a widget deaf to dispatched events, and a hidden persistent listbox.

## 4. Agent loop

> Done in `hire` on `feat/combobox-agent-loop` (unpushed). Like group 3, no
> reviewed-clean stamp: the review pass was cancelled before it reported.
>
> **Scope note.** This group puts the loop in hire, per design.md's "the loop
> lives with the agent, in hire". That decision was since revisited: the user also
> wants a *local* harness, connected with `freehire-cli`'s API key, able to drive
> the filling itself. Both paths are wanted, so this group stands as the hosted
> path and the harness end is a separate change. The wire needs nothing either
> way — the relay forwards frames verbatim, so the primitives groups 1-3 shipped
> already serve any harness that connects.

- [x] 4.1 For each widget field in the plan: open → options → choose the option the profile justifies → select → verify. An unverified write is reported as not filled.
      Choosing had to become a *second* model call rather than part of the plan: no widget renders options until opened (measured — not one of the two forms' 40 comboboxes carries a list while closed), so the plan cannot name a value it has never seen. It names which widgets are worth opening; the choice is made against the list read from the page. Only `verified` counts as filled.
- [x] 4.2 Grounding applies to the chosen option exactly as to a typed value; a question the profile does not answer selects nothing.
      Applying the *existing* filter unchanged turned out to be wrong: it compared raw substrings both ways, and a widget's options are short — `no` sits inside `norway`, so a candidate living in Oslo would have "No" grounded for any question at all. That is this change's own failure mode one layer down. Grounding now matches whole words, in both directions, which still accepts "Germany" from "Berlin, Germany" and no longer accepts "No" from Norway.
- [x] 4.3 The report names each grouped question once, and distinguishes filled / not-yet-fillable / left-for-you.
      A grouped question arrives from `read_form` as one field, so it is named once for free. The three-way split is now a named function: filled (a confirmed commit), not-fillable-yet (would not open, committed something else, or offers its options only over the network), and left-for-you (the profile does not answer it, or the answer it suggests is not on offer).

## 5. Verify

- [ ] 5.1 Extension: `vitest run` + `svelte-check` + `wxt build`; hire: agent tests green.
      Extension green: 109 tests / 11 files, `svelte-check` 999 files 0 errors, `wxt build` 77.88 kB. Hire's agent tests are task group 4's, deliberately out of this session's scope.
- [ ] 5.2 Live end-to-end on the same two Greenhouse forms used as the baseline (Stripe: 4 filled / 13 widgets + 1 group; Scout Motors: 6 filled / 27 widgets). Record the new numbers; nothing is submitted.
      **Observation half measured** by injecting this change's own `form.ts` into both live forms (read-only; nothing filled, nothing submitted). Stripe: 59 controls → **31 questions**, the 29-country question now one field carrying 29 options and `required: true`. Scout Motors: 61 controls → 61 questions, 27 widgets, no groups — it has no checkbox groups to collapse. On both forms **no** widget carries options while closed, so the agent must go through `combobox.open` + `combobox.options`, exactly as designed.
      **Fill half still outstanding**: it needs the agent loop (task group 4, in hire) plus a signed-in panel, so the filled/not-fillable counts cannot be recorded yet.
- [ ] 5.3 Confirm no new permission: the built manifest asks for no `debugger`, and Chrome shows no debugging banner while autofill runs.
      Built manifest asks for `storage`, `tabs`, `sidePanel`, `scripting`, `activeTab`, `identity` — no `debugger`. The banner half needs a live autofill run, so it waits on 5.2's fill half.
