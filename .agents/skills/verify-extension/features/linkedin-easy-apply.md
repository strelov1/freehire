# LinkedIn Easy Apply autofill

The panel recognises the Easy Apply dialog as an application and fills each step from the
candidate's profile. The candidate steps through and presses Submit themselves.

## Sub-features

- Dialog recognised as an application (no "doesn't look like the application form" refusal).
- Contact step: email select, phone country select, phone.
- Resume step: stored-resume picker left alone (panel reports no fields).
- Employer questions step: answers from the profile; LinkedIn's validation accepts them.
- Review step reached; `Submit application` present and NOT pressed.

## How to get to it (user POV)

Open a LinkedIn job with Easy Apply, press Easy Apply, press Autofill in the panel on each
step, Next, and Review.

## Driving it with cua_*

1. Job page and panel as in [match-card.md](match-card.md).
2. Press page `Easy Apply to this job` (query `Easy Apply`). The dialog `Apply to <company>`
   opens at `1/4 pages`.
3. Press panel `Autofill`. Proof: panel text `✓ Autofilled N field — review before submitting.`
4. Press dialog `Next` (query `Next`, the one under the page `AXWebArea`). At `2/4 pages`
   (Currículum) press `Autofill`. Proof: panel `No form fields found on this page.`
5. `Next` → `3/4 pages` (Preguntas adicionales). Press `Autofill`. Proof: the question's field
   holds the profile value and the panel checklist lists it as `answered`.
6. Press `Review`. Proof of success: `4/4 pages`, heading `Review your application`, and a
   `Submit application` button exists. STOP here.

## Gotchas

- Numeric employer questions: the salary question (`Cuál es tu expectativa salarial`, max 20
  chars) accepts digits only and says so only after Review (`Invalid input`). The filler now
  writes the profile's first amount as bare digits there ("5000 USD/month" → "5000"); proven
  live 2026-11-22. If a step will not advance, read the panel checklist: a field LinkedIn
  rejected shows `not answered`.
- The agent (server-side) autofill answers `HTTP 409 no fillable fields` on the resume step
  and the panel falls back to the basic filler; harmless.
- The panel's first read can race the dialog's spinner and show "N fields on this page — not
  enough to read as an application form"; it re-reads once the dialog renders.
- Draft state: LinkedIn keeps a started application. Re-opening Easy Apply resumes it.
