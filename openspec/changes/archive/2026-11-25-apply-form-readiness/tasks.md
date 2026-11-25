## 1. Spike: what identifies an application form

- [x] 1.1 Load the extension into Chrome and capture `read_form` output for a page from each of: Greenhouse embed (weareroku.com), Greenhouse board (job-boards.greenhouse.io), Greenhouse on a company site (careers.airbnb.com), Lever and Ashby — both before and after the application is opened.
- [x] 1.2 Judge which signal separates an application form from a newsletter/alert form. Verdict: **VALIDATED** — a visible file input, on all five boards. See the proposal.

## 2. Readiness

- [x] 2.1 Report the page's uploads as an observation: `extractUploads` in `form.ts`, carried per frame through `FORM` / `FRAMED_FORM` and folded in `readFramedForm`. File inputs stay out of the question list — their value cannot be set from script.
- [x] 2.2 `looksLikeApplication` decides over that observation, pure and tested.
- [x] 2.3 Panel: a not-ready page is told what is missing and filled only if the user presses "Fill it anyway".
- [x] 2.4 Wire: `read_form` returns `{fields, uploads}` so a harness can apply the same rule. Acting on it is a hire-side change (see the proposal).

## 3. Form scoping

- [x] 3.1 `FormField.form` carries the owning `<form>`; `scopeToApplication` keeps the `(frame, form)` group the upload sits in, falling back to every field when that group holds none.
- [x] 3.2 Tested with two same-labelled forms on one page and with a group living in an iframe.

## 4. Field matching

- [x] 4.1 `matchFieldKey` requires the synonym to open the label, at a word boundary.
- [x] 4.2 Regression tests with the live Roku question text and with Greenhouse's hash-suffixed labels.

## 5. Verification

- [x] 5.1 Live run, Roku: before Apply → 5 fields, 0 uploads, refused, nothing written. After Apply → 16 fields across groups `0/1` and `0/2`, scoped to 11, 6 filled, and only the application's controls (`form_*_2_4_*`) carry values — the job-alert form is left alone.
- [x] 5.2 Live run, Airbnb: before → 0 fields, refused. After → 21 fields in **frame 12**, scoped to 21, 5 filled.
