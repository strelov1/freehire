# Tailor and attach a CV

Tailors the CV for the job from the panel, shows the run's progress, and places the
tailored PDF into the application: a page's `input[type=file]`, or LinkedIn Easy Apply's
"Upload resume" picker.

## Sub-features

- `Tailor my CV` → confirm → a background freehire tab opens `/tailor/<slug>`; the panel shows
  `Tailoring your CV`, a progress bar `N of M requirements` and `Watch it work →`.
- When the run is over: `Open your tailored CV →` and, on an application, `Attach tailored CV`.
- Attach on an ATS form with a visible upload field (`DOM.setFileInputFiles` on that input).
- Attach in Easy Apply on the resume step: a trusted click on "Upload resume" with the file
  picker intercepted (`Page.setInterceptFileChooserDialog` → `Page.fileChooserOpened`).

## How to get to it (user POV)

On a job page press Tailor my CV and confirm; watch the bar fill or open the link. Then open
the application (Easy Apply: go to the resume step) and press Attach tailored CV.

## Driving it with cua_*

1. Job page with the panel open, no tailored CV for the job yet (the card shows `Tailor my CV`).
   Tailoring spends one of the owner's daily tailoring sessions: confirm with the owner first.
2. Press `Tailor my CV`, then the dialog's confirm (`Tailor my CV` or `Tailor anyway`).
   Proof: a new freehire tab exists; the panel shows `Tailoring your CV` and, within ~30 s,
   `N of M requirements` with N rising.
3. Wait until the panel shows `Open your tailored CV →` (all requirements reached, or two
   quiet minutes). Screenshot the bar mid-run and the end state.
4. Easy Apply: open the dialog, `Next` to `2/4 pages` (Currículum). Press `Attach tailored CV`.
   Proof: panel `Attached to this form`; the resume list gains `<job_slug>.pdf` and it is the
   selected radio. Continue to Review and check the Currículum section names that file.

## Gotchas

- The server starts the unattended run only for a signed-in browser (cookie), never for the
  extension's token, so the panel cannot start it itself: it opens the freehire tab, whose
  first visit to `/tailor/<slug>` creates the CV and auto-runs (`cold_start_running`).
- The run is detached from the tab's connection; closing the tab does not stop it.
- A job that already has a tailored CV opens without a new run; the panel shows
  `Open your tailored CV →` instead of the button.
- Attach on Easy Apply works only on the resume step; elsewhere it answers "open the resume
  step of Easy Apply first". Chrome allows one debugger client per tab: close DevTools first.
