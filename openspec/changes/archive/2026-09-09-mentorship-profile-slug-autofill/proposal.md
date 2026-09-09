## Why

Submitting a new mentor profile requires typing two raw identifiers by hand with no
guidance: the public URL slug (`internal/engage/mentorship/profile.go:255-256` rejects
anything with an underscore or other character outside `[a-z0-9-]`, which a mentor typing
their own account-style handle will not guess) and the company slug (a raw foreign key
into the company catalogue, typed with no lookup, that 404s on any typo). A mentor hit
this directly: submitting `i_strelov` as the URL fails with an opaque "is not a usable
profile address" error, and there is no indication anywhere in the form what characters
are allowed or how to find the exact company slug.

## What Changes

- On creating a mentor profile, an empty "Your URL" submission is no longer refused: the
  system derives a slug from the display name (the same sanitisation rule an account
  username already uses), and resolves a collision on the derived slug with a numeric
  suffix (`base-2`, `base-3`, ...) automatically. An explicitly supplied slug is still
  validated and still refused outright on a collision — only a system-derived slug gets
  the silent suffix retry.
- The "Your URL" field in the profile creation form live-previews the derived slug as the
  mentor types their name, stays editable, and shows the allowed character format.
- The "Company slug" free-text input in the profile creation form is replaced by the
  existing `CompanyPicker` typeahead component (already used in the referral-offer form),
  so a mentor searches their employer by name instead of typing its internal slug.
- Editing an existing profile is unaffected: both the URL and the company remain
  immutable after creation, exactly as `internal/engage/mentorship/profile.go:95-97`
  already documents (a stable URL for shared links; a company change is a new claim that
  would need re-moderation, which this change does not add).

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `mentor-profile`: submitting a profile with no explicit URL slug SHALL succeed with a
  system-derived, unique slug instead of being refused; an explicitly supplied slug's
  validation and collision handling are unchanged.

## Impact

- `internal/engage/mentorship/profile.go` (`Service.SubmitProfile`, `validateProfile`):
  derive and retry the slug when blank.
- `internal/engage/mentorship/profile_test.go`: new unit tests for derivation, collision
  suffixing, and the fallback for a name with no latin/digit characters.
- `web/src/lib/components/MentorProfileEditor.svelte`: live slug preview, format hint, and
  swapping the company input for `CompanyPicker` (create mode only; edit mode keeps both
  fields as read-only display, unchanged from today's disabled inputs).
- No database migration, no API request/response shape change (both fields already exist
  and already accept an empty or a valid `slug` string).
