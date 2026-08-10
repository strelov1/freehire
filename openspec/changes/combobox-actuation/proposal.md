## Why

Agent autofill ships, and on real forms it fills a minority of the fields. Measured
on two live Greenhouse postings the day `agent-autofill-wire` landed:

| Form | Filled | Reported as not-fillable |
|---|---|---|
| Stripe — Backend Engineer, AI Security | 4 | 13 comboboxes + a 30-checkbox country question |
| Scout Motors — Software Engineer | 6 | 27 comboboxes |

Everything the agent skips is a custom widget: a react-select dropdown, or a
question rendered as a group of checkboxes. These are not edge cases — they carry
the answers an application actually turns on (country, work authorization, visa
sponsorship, notice period, education). `fill_simple` deliberately refuses them,
because the spike showed that writing text into one commits whatever its listbox
happens to highlight — "Norfolk Island" for a question that wanted "No".

So the honest state today is: the user still fills most of the form by hand, and
the panel's report is dominated by what the agent could not do.

## What Changes

- **`combobox.*` primitives:** `open`, `options`, `select`, `verify` — the agent
  drives the widget the way a person does (open it, read what is actually
  offered, choose, confirm what got committed) instead of writing text at it.
- **Actuation without CDP first.** A widget is opened and its option clicked with
  ordinary events; `chrome.debugger` stays an unused seam. It is the reliable
  fallback, but it costs a permission and an undismissable "freehire is debugging
  this browser" banner, so it is not paid for until a widget is found that needs
  it.
- **Checkbox groups become one field.** A question rendered as N checkboxes under
  a shared `fieldset`/`legend` is reported as one field with N options, not as N
  fields — which is also what stops the report being swamped by 30 country names.
- **The agent loop:** for each widget the plan targets, open → read options →
  choose the one the profile justifies → verify → move on. A widget whose options
  contain nothing the profile supports is left alone and reported, exactly as
  today.
- **NOT in this change:** typeahead widgets that fetch options from the network as
  you type (Greenhouse's "Location (City)" is one); file upload; anything that
  submits the form.

## Capabilities

### New Capabilities
- `combobox-actuation`: the extension opens a custom-widget dropdown, reports the
  options it offers, selects one, and verifies what was committed — the primitives
  the agent composes.
- `field-grouping`: a question rendered as several controls (a checkbox group) is
  observed and filled as one field with options, rather than as separate fields.

### Modified Capabilities
- `browser-tool-wire`: `read_form` gains the shape a widget needs — the options a
  combobox offers, and the grouping of a multi-control question. The wire's
  framing and ownership rules do not change.
- `agent-autofill`: the agent may now fill custom widgets, and must justify a
  chosen option from the profile the same way it justifies a typed value.

## Impact

- **freehire-extension**: new `combobox.*` primitives beside `read_form` /
  `fill_simple`; `form.ts` gains widget observation (open state, option list) and
  group detection; the executor grows the new tools. No new permission.
- **hire**: the agent loop in `internal/autofillagent` becomes multi-step for
  widget fields — the grounding filter applies to a chosen option as it does to a
  typed value.
- **Dependencies**: none new, unless a widget forces the CDP fallback — which
  would add the `debugger` permission and is deliberately out of scope here.
- **Risk carried over**: this is the failure mode the spike named. Every widget
  write is verified after the fact, and an unverified write is reported as failed
  rather than assumed.
