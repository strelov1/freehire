## Context

The side panel already has both halves of what this needs: it reads a page's form
(`GET_FRAMED_FORM` → `FramedField[]`, carrying label, `required`, current `value`, `frame`
and `form`) and writes values back (`FILL_BY_LABEL` → `fillByLabel`), both addressing a
*question* rather than a control. What it lacks is any standing account of that form — the
result of an autofill is three sentences of notices, discarded on the next action.

The full design narrative, agreed with the user before this change was opened, is in
`docs/superpowers/specs/2026-08-15-extension-apply-plan-design.md`. This document records
the decisions that shape the implementation.

## Goals / Non-Goals

**Goals:**

- A checklist of the form's questions, standing on its own before anything is filled.
- Autofill visible as it happens: scroll, outline, tick off, advance.
- An unanswered question reachable from the panel in one action.
- A counter that stays honest while the user types on the page.

**Non-Goals:**

- Asking for a missing answer inside the panel (the click-to-reach remainder is enough to
  finish a form; the panel prompt is a later change if it earns itself).
- Persisting answers back into the profile.
- Modelling a multi-page ATS application. The plan describes the page in front of the user.
- Any change to what autofill may write, or to the server's autofill profile.

## Decisions

### The plan is a projection, computed in one pure function

`lib/applyPlan.ts` turns `FramedField[]` into `PlanItem[]` plus a counter. It is pure over
its input — no DOM, no messaging — so the counter arithmetic, the required/optional split
and "already answered" are unit tested directly, the same discipline `scraper.ts`,
`form.ts` and `combobox.ts` keep. `ApplyPlan.svelte` renders what it is given and emits
intent; it holds no page access of its own.

*Alternative rejected:* computing the plan inside the component. It would put the only
interesting logic in the one place this codebase cannot test without a browser.

### The walk lives in the panel, not in the page or the agent

The panel already owns the sequence: it has the plan, it can pause between steps, and it
renders the progress. Driving the walk from the content script would need a second copy of
the plan there; driving it from the agent would put a UI pace loop on the far side of the
network.

The agent path keeps filling server-side. The panel plays its report back: for each label
in `filled`, match against the questions it just read, reveal it, tick it off. A label the
page no longer carries is skipped — the report describes a form that may have re-rendered.

### Reveal is a page-side primitive, restored after itself

`revealField(doc, {label, form, focus})` scrolls the question's first control into view
(`block: 'center'`), sets an inline outline for ~600 ms, restores the previous inline style,
and focuses when asked. Inline style rather than an injected stylesheet: the page is not
ours, a class could collide with the ATS's own, and a stylesheet outlives the extension's
interest in the element.

`FILL_BY_LABEL` gains `reveal?: boolean` rather than a second message, so a fill and its
reveal cannot separate. The agent's own tool-driven fills leave it unset and stay silent.

### The page tells the panel when the form changes

One delegated `input`/`change` listener on the document, debounced 400 ms, posts
`FORM_CHANGED`; the panel re-reads the form and recomputes. Delegation, not per-control
listeners: an ATS form re-renders constantly and per-control listeners would be re-attached
(or leak) on every render. Polling was the alternative — cheaper to write, and it either
lags the user or re-walks the DOM on a timer forever.

### The checklist renders where the match card already is

The Match tab, above the pinned Autofill footer. It appears when the page shows an
application form (`looksLikeApplication`, already used to guard the deterministic filler),
so the panel does not decorate a job description with an empty account of a form that is
not there.

## Risks / Trade-offs

- **A long form makes a long checklist** → The list is grouped (required first) and scrolls
  with the panel's existing scroll container; no truncation, because hiding a required
  question is exactly the failure this change exists to remove.
- **`FORM_CHANGED` chatter on a busy ATS form** → Debounced at 400 ms and coalesced into one
  re-read; the read is a DOM walk the panel already does per page.
- **The 300 ms pause makes autofill slower** → Deliberate: the walk is the audit. A form
  where the user does not want to watch is the one they leave alone; the values still land
  in the same order.
- **`required` is only as good as the page's markup** → Some ATS forms mark required in the
  label text rather than the attribute. Those questions land in Optional and undercount the
  counter. Accepted for now; the alternative is parsing the asterisk out of label text,
  which guesses.

## Migration Plan

No server change, no schema, no permission. It ships with the next extension build; an older
panel against the same page keeps working because every addition to the wire is optional
(`reveal` absent = today's behaviour) and the new messages are ignored by a content script
that does not know them.

## Open Questions

None outstanding. The panel-prompt-for-missing-answers idea was considered and explicitly
deferred (see Non-Goals).
