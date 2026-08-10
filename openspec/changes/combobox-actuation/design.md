## Context

`agent-autofill-wire` shipped the wire and two primitives: `read_form` observes,
`fill_simple` writes text, checkboxes and native `<select>`s addressed by label.
It deliberately refuses custom widgets — the spike showed that writing text into a
react-select commits whatever its listbox highlights, so a question wanting "No"
got "Norfolk Island".

Measured on two live Greenhouse postings, that refusal covers most of the form:
Stripe filled 4 fields and deferred 13 comboboxes plus a 30-checkbox country
question; Scout Motors filled 6 and deferred 27. The widget cases are where the
substantive answers live.

## Goals / Non-Goals

**Goals:**
- Fill a custom-widget dropdown by choosing from the options it actually offers.
- Observe a multi-control question (a checkbox group) as one field with options.
- Never leave a widget in a state nobody asked for: every write is verified, and
  an unverified write is reported as failed.

**Non-Goals:**
- `chrome.debugger` / CDP. Kept as a named seam, not built.
- Typeahead widgets that fetch options over the network while you type
  (Greenhouse's "Location (City)"). They need a debounce-and-wait model of their
  own.
- File upload, and anything that submits a form.

## Decisions

**Drive the widget, don't write to it.** The primitives mirror what a person
does: `combobox.open` reveals the listbox, `combobox.options` reports what is
offered, `combobox.select` clicks one, `combobox.verify` reads back what the
control now holds. The agent composes them. This is the same "primitives, not a
smart filler" line the previous change drew, extended one step: the option list
is *evidence*, so the agent chooses among real values instead of guessing a
string and hoping the widget agrees.

**Ordinary events first; CDP is a seam, not a dependency.** A react-select opens
on a real `mousedown`/`click` and commits on a click over the rendered option —
both of which an injected script can dispatch. `chrome.debugger` would give
genuinely trusted input, but it costs the `debugger` permission and an
undismissable "freehire is debugging this browser" banner for as long as it is
attached. That is a product cost, and it is not paid until a widget is found that
demonstrably needs it. If one is, the seam is `combobox.select` — one primitive
swaps its actuation, nothing else moves.

**Verify by reading back, not by assuming.** After a select, the committed value
is read from the control and compared to what was asked for. This is what makes
the spike's failure mode *detectable* rather than silent: a mismatch is reported,
never counted as filled. Both the report and the grounding filter treat an
unverified write as not written.

**A checkbox group is one field.** Grouping is by the DOM structure the question
already has — a shared `fieldset`, or a common labelled container — so "which
country do you reside in" is one field offering 30 options, not 30 fields. This
is the same observation fix that stops the panel's report being swamped, so the
grouping belongs to `read_form`, not to the report.

**Grounding still applies to the choice.** A chosen option must be justified by
the profile exactly as a typed value is. "Germany" is chosen because the profile
says Berlin, Germany; "Yes, I require sponsorship" is not chosen because nothing
in the profile answers it. The existing `groundedIn` filter is the place, and its
input becomes the option text.

**The loop lives with the agent, in hire.** Same reason as last time: the harness
end is already there, the profile is there, and the wire stays a dumb conduit.

## Risks / Trade-offs

- [A widget that ignores synthetic clicks] → The whole change rests on this. It is
  the first thing to establish, on the real Stripe and Scout forms, before the
  primitives are built out; if it fails, the CDP seam is the answer and the
  permission cost comes back on the table as an explicit decision.
- [Verification reads the wrong thing] → A react-select's committed value lives in
  its own markup, not in the input's `value`. `combobox.verify` has to read what
  the widget displays; getting this wrong would report success on a failed write,
  which is worse than not filling at all.
- [Grouping heuristics misfire] → Grouping controls that are not one question
  would hide fields from the agent. It keys on explicit structure (`fieldset`,
  labelled container) and leaves ungrouped anything it cannot read confidently.
- [More steps per field] → Each widget costs open + read + select + verify over
  the wire, against one call for a text field. A form with 27 of them is a lot of
  round trips; batching is available later if it proves slow, and is not designed
  for up front.

## Open Questions

- Does one option list serve every widget, or do the common ATS widgets
  (react-select, Workable's, Ashby's) differ enough to need per-widget reading?
  Resolve on real forms during task group 1.
