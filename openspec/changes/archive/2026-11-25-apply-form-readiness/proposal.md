## Why

Autofill assumes the page it is pointed at asks the application's questions. On a
real careers page that is often false, and the panel has no way to say so.

Measured on `weareroku.com` (Greenhouse, `gh_jid=8022897`) with the extension
loaded in Chrome:

- **Before the user clicks "Apply":** the application form sits in a
  `display:none` container (`div.block-wrapper.ApplyCTA`), so `read_form`
  reports **5** fields — and all five belong to a *job-alert subscription* form
  (`First Name`, `Last Name`, `Email`, `Departments`, `Locations`).
- **After the click:** **16** fields, the application's own.

So Autofill on an unopened page silently fills a marketing signup and reports
"✓ Autofilled 3 fields". Nothing is wrong from the code's point of view; the page
simply was not ready, and neither the panel nor the agent noticed.

Two adjacent findings from the same page, worth resolving here rather than
separately:

- **Both forms get filled.** With the application open, the page carries two
  `First Name` / `Last Name` / `Email` questions. `fillByLabel` answers the first
  question carrying a label, but Greenhouse suffixes each label with a random
  hash (`"First Name (required) e85441b6"`), so the two forms read as distinct
  questions and both are written. Confirmed live: `form_*_2_4_*` (application)
  **and** `form_*_2_1_*` (job alert) both received the profile.
- **`matchFieldKey` matches on a bare substring.** `country: ['country']` makes
  *"Are you authorized to lawfully work for Roku in the **country** to which you
  are applying?"* resolve to the `country` key. It is inert only by accident —
  `profileToValues` ships no `country` value, so the fill is dropped. Adding a
  country to the profile would start writing "India" into a Yes/No radio group.

## What Changes

- **Readiness signal:** decide whether the page is showing an application form
  before filling it, and tell the user when it is not ("open the application
  form first") instead of filling whatever is visible.
- **Form scoping:** fill one form — the application — rather than every question
  on the page that happens to carry a matching label.
- **Tighten `matchFieldKey`:** stop a generic word inside a long question from
  claiming a profile key.

## Non-goals

- Clicking "Apply" on the user's behalf. Opening an application is the user's
  action, not autofill's.
- The frame fan-out and the API error surfacing — already fixed (see
  `background.ts` `readTopFrameSnapshot`, `freehire.ts` `apiErrorMessage`).

## Spike verdict: VALIDATED

The extension was loaded into a real Chrome and `read_form` captured on five
boards, before and after the application was opened:

| board | not showing an application | showing one |
|---|---|---|
| Greenhouse board (Discord) | — always open | 34 fields, **2 uploads**, frame 0 |
| Greenhouse embed (Roku) | 5 fields, 3 required, **0 uploads** | 18 fields, **2 uploads**, frame 0 |
| Greenhouse on a site (Airbnb) | 0 fields | 24 fields, **2 uploads**, **frame 18** |
| Lever (Match Group) | 0 fields | 53 fields, **1 upload**, frame 0 |
| Ashby (Linear) | 0 fields | 13 fields, **2 uploads**, frame 0 |

**A visible file input separates the two states on all five boards**, including
the only hard case: Roku's job-alert signup is indistinguishable from a short
application by field count or required count, and is correctly rejected because
it takes no CV.

The spike also settled a non-goal empirically: an early pass clicked a button
reading "Submit Application" on Ashby and **submitted the form**. The extension
must not press buttons on the user's behalf.

## Decisions

- **A not-ready page is a refusal, not a warning.** Filling a marketing signup
  silently and reporting "✓ Autofilled 3 fields" is the worse failure, because it
  is invisible. A refusal is visible and recoverable, and carries a "Fill it
  anyway" button for the rare application that asks for no CV.
- **Scoping keys on `(frame, form)`**, not the form alone — an ATS iframe numbers
  its own forms from zero, so the frame is part of the identity.
- **`matchFieldKey` anchors at the front of the label** instead of matching a
  substring anywhere in it.

## Out of this repo

The agent path (`/me/autofill/run`) should refuse a not-ready page on the same
grounds. The extension now ships `uploads` alongside `fields` in the `read_form`
result, which is backwards compatible; acting on it is a change in **hire**
(`internal/autofillagent`), tracked separately.
