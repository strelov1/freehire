## Why

The experience bank's job forms (add and edit) ask for a free-text "Company" name with no help finding the right spelling, even though the site already has a company-search typeahead (`CompanyPicker`, used on the Referrals and Mentor-profile forms) that solves exactly this. Separately, the schema and API already carry a `current` ("still working here") flag on an employment — it is rendered ("Present") wherever an employment's dates are shown — but no form exposes a way to set it, so a candidate can only leave the end date blank, which is not the same value on the wire and is easy to miss.

## What Changes

- Generalize `CompanyPicker.svelte` so its typed text is an optional `$bindable` `value` prop (in addition to the existing `onSelect` callback), so a consumer that needs free text — not just a resolved catalogue slug — can use it too. Existing consumers (`ReferralsView`, `MentorProfileEditor`) are unaffected since they do not bind `value`.
- Replace the plain text "Company" input in the experience bank's add-job form (`ExperienceBankView.svelte`) with `CompanyPicker`: picking a suggestion fills the canonical company name into the same free-text field; typing a company not in the catalogue keeps working exactly as before.
- Replace the equivalent "Company" input in the per-employment edit form (`ExperienceEmploymentCard.svelte`) with `CompanyPicker` for job-kind entries only; project-kind entries keep their plain text "Project name" input unchanged.
- Add an "I currently work here" checkbox to the End-date area of both the add-job and edit-job forms (job kind only). Checking it hides/clears the End date field and submits `current: true` with no end date; unchecking it restores the ordinary End-date input and submits `current: false`.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `experience-bank`: the job-entry forms (add and edit) gain a company-catalogue-backed autocomplete on the Company field (free text remains valid) and a way to mark a job-kind employment as current, both previously unsupported by any UI even though the underlying data model already carries them.

## Impact

- Frontend only, no backend/API changes: `internal/candidate/experience`'s `current` field and `createExperienceEmployment`/`updateExperienceEmployment` already accept it.
- Affected files: `web/src/lib/components/CompanyPicker.svelte`, `web/src/lib/components/ExperienceBankView.svelte`, `web/src/lib/components/ExperienceEmploymentCard.svelte`.
