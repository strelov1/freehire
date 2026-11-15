# Candidate-controlled evidence for an application

Date: 2026-11-15
Status: Draft for review

## Problem

Freehire already stores candidate experience, retrieves relevant achievements and drafts cover
letters with citations. The candidate sees the sources after generation but cannot first decide
which achievements a particular application should use. Regenerating a letter can select a
different set without the candidate understanding why.

The intended experience is: review suggested facts, adjust the selection, prepare the application,
and inspect its sources. Selection must reduce repeated explanation without making every
application require a new questionnaire.

## Existing implementation

- `internal/candidate/experience` owns employments and achievement atoms. Only candidate-asserted
  provenance is publishable. Import accumulates facts and does not remove them.
- `internal/candidate/coverletter/Gather` retrieves evidence for `missing-have` requirements and
  falls back to the bank when retrieval yields nothing. Retrieval makes no model call.
- `coverletter.Analyzer` selects, drafts and audits a letter. Its service-level provenance gate
  and offered-ID citation sanitizer must remain in force.
- `internal/api/handler/cover_letter_drafter.go` is shared by the endpoint and assistant tool.
  Drafting requires an existing fit analysis, consumes a plan allowance through its caller,
  streams progress and stores a letter for a candidate and vacancy.
- `web/src/lib/tailor/CoverLetter.svelte` already displays the letter and resolved citations
  inside the Tailor workspace. It is the initial integration point.

The current citations identify achievements supporting the letter as a whole. They do not
establish a sentence-by-sentence verification guarantee.

## Product scope and delivery

The target is a candidate-controlled evidence set shared by artifacts for one vacancy. Deliver
it in two independently useful increments:

1. Review, save and use evidence for cover letters in the existing Tailor workspace.
2. Connect CV tailoring and screening answers to the same saved selection after their tools and
   prompts enforce it. Until then, the UI explicitly calls this selection cover-letter evidence.

The first increment does not claim that CV tailoring or screening answers respect the selection.
Sentence-level source links are a separate extension: the initial release retains the existing
letter-level source list rather than presenting unsupported precision.

## Interaction

### Before writing

The cover-letter tab offers “Review evidence” and the existing direct drafting action. Reviewing
is optional. With no saved selection, direct drafting preserves today's automatic behavior.
With a saved selection, both actions use that selection; bypassing review never discards it.

The review panel shows:

- The vacancy title and employer already available in the workspace.
- Suggested, preselected publishable achievements, ranked by the existing retrieval policy.
- Other publishable bank achievements, collapsed under “More from your experience”.
- Claim text and employment/project context where available, with an expanded view for long text.
- A selected count, “Save selection” and “Write using these facts”.
- A link to the existing Experience editor to add or correct a fact, followed by a refresh action.

Unchecking a fact removes it only from this vacancy's selected set. It does not delete or edit
bank data. No new folder structure or second experience editor is introduced.

An explicit empty selection can be saved, but generation is disabled with “Select at least one
confirmed experience”. It must never silently fall back to the whole bank.

Fit-analysis `missing-gap` requirements may be shown as “Not confirmed in your experience”. This
is an analysis result, not a new keyword-based Kubernetes detector. The user can add evidence
through the existing bank; the page does not silently promote a gap to a confirmed capability.

### After writing

Keep the letter, Copy action and existing “What it stands on” list. Add “Change evidence”.
Distinguish “Selected for this application” from “Cited in this letter”: a selected fact is
permitted input, not a promise that the final letter uses it.

Editing a saved selection does not rewrite an existing letter. Show that the selection changed
since that draft and offer explicit regeneration, with the normal allowance behavior. A failed
regeneration leaves the old letter intact.

## Ownership and persistence

Selection belongs to `(user_id, job_id)`, matching the existing letter's vacancy-level identity.
Two tailored CVs for the same user and vacancy see the same selection. The owner is always the
authenticated account; request bodies never choose it.

Add a small cover-letter-owned selection record containing the owner, vacancy, revision and
selected atom references. Absence means automatic mode; presence with zero references means
explicitly empty. Store references, not duplicate editable claims.

Do not cascade-delete the selection or silently remove its references when a bank atom is
removed. Resolution reports missing or no-longer-publishable selections and blocks generation
until the candidate reviews and saves a valid selection. Keep existing stored letters readable.
The implementation must use a new migration, sqlc queries and generated code, following the
repository's database and HTTP-contract conventions.

Read and write operations are owner-scoped. A selection update uses an expected revision;
concurrent edits return a conflict instead of overwriting another tab. The first save must
also detect another tab creating the record.

## Server behavior

Expose owner-authorized read and update operations under the existing CV cover-letter route
group. Resolve the CV to its vacancy through the same authorization path as drafting.

The read response returns selection mode, revision, selected IDs, resolved publishable bank
facts, suggested IDs, and invalid selected references. Suggestion and availability failure must
be reported as an error rather than displayed as an empty bank. Reading and saving selection
make no model call and consume no AI allowance.

On update, validate the whole request before writing:

- IDs are valid and unique, with an explicit bounded request size.
- Every selected atom belongs to the caller and is publishable.
- The expected revision matches.

Use the bank's provenance predicate; do not duplicate its list of permitted labels. Bound the
selection to 50 atoms initially, returning a clear validation error rather than truncating.

At generation time, the shared drafter resolves the saved selection again. Automatic mode uses
existing Gather behavior. Explicit mode offers only selected, currently publishable atoms.
Invalid or empty explicit selections fail before any allowance is consumed or stream opens.
The assistant tool follows the same selection policy through the shared drafter.

Freeze the resolved facts and selection revision for the duration of an accepted generation.
A concurrent selection change does not change a running model request. Store the generation's
selection revision alongside its letter so the read view can report a newer selection.

A selected atom can also be edited without changing its ID. Include a deterministic fingerprint
of the selected claims and provenance in the generation metadata, and compare it on reads to
report changed evidence. Do not call a draft current merely because its selection revision
matches. Existing letters without this metadata remain readable with freshness unknown.

## Model input and guarantees

The selected set must constrain all candidate-specific achievement inputs, including the
professional profile projection. Filtering only the atoms while passing unselected achievements
through `candidateProfileFrom` would defeat the control. Preserve necessary role chronology,
education and other structured background, but omit unselected achievement claims from that
projection in explicit mode. Test the actual prompt input for this boundary.

Keep the existing select/draft/audit chain: it may use a subset of the offered facts. The audit
receives the same frozen evidence. Sanitization refuses citations outside that set.

This provides control over offered facts and cited IDs, not a proof that model prose cannot
hallucinate. Product copy must avoid absolute guarantees such as “nothing here is invented”.
Use “Drafted from your selected experience. Review before sending.”

## Failures and accessibility

- No fit analysis: offer the existing fit-analysis action; do not trigger a paid analysis.
- No publishable facts: explain how to add or confirm experience; do not draft from inferred facts.
- Save conflict: keep local choices visible, fetch the latest selection and ask for review.
- Save failure: retain local choices and do not start generation with an older saved selection.
- Missing/changed evidence: identify affected facts and require review where publication is invalid.
- Vacancy navigation: reset local state; stale responses cannot replace the new vacancy's selection.
- Stream disconnection: retain today's warning that the server may have saved the letter.
- Use labeled native checkboxes, keyboard-accessible expansion and announced save/error states.

## Alternatives considered

A browser-only selection is smaller but disappears across sessions and cannot constrain the
assistant path. A separate dossier workspace duplicates the bank and adds unnecessary upkeep.
A persisted selection in the existing workspace gives consistent behavior with limited new UI.

## Verification and acceptance

- Only owner-scoped, publishable facts can be read or selected; forged IDs never reach a prompt.
- Omitted selection preserves automatic behavior; explicit empty selection never falls back.
- Unselected achievements are absent from both evidence and professional-profile prompt inputs.
- Endpoint and assistant generation both honor the same selection.
- Selection updates survive reload and detect concurrent saves, including first-save races.
- Removed/inferred selected facts block generation before charging; edited claims mark a draft changed.
- A running generation uses its initial snapshot; a newer selection is shown after completion.
- Failed save/generation preserves local choices and the existing letter respectively.
- UI tests cover selection, optional review, empty state, conflict, refresh and vacancy navigation.
- Integration tests cover ownership, persistence, revision conflicts and charge ordering.
- Run affected frontend checks and repository-required Go checks, including integration-tag vet.

## Deferred work

CV and screening integration, sentence-level source spans, near-duplicate suggestions and writing
style examples are outside the first increment. No dossier folders, new semantic retrieval,
automatic employer submission or extra model-based ranking is required.
