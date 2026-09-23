## ADDED Requirements

### Requirement: The sidebar reports the match and offers no tailoring action

The job-detail sidebar's match block SHALL report what it knows and SHALL NOT carry a
tailoring call-to-action: `Tailor my CV` lives in the page's CTA row, where the reader
decides, and a second copy of the page's one primary button a screen below the first is what
this change removes.

The block SHALL keep exactly two things below the deterministic coverage bar:

- an `Upload a CV to analyse` prompt with its link to the profile, when the page has read
  that the reader has no stored CV;
- the cached fit-analysis card — the overall percentage, the verdict, the single biggest gap
  and a link through to the full analysis — when one is cached.

It SHALL NOT render a remaining-tailorings count nor a spent-allowance message. The
confirmation dialog raised by the CTA row's button already carries both, at the moment the
reader commits, and stating them in two places lets them disagree.

The block SHALL NOT issue a match-analysis request of its own. The page SHALL read that
response once and pass it in, so moving the button did not buy a second call to the same
endpoint.

#### Scenario: No tailoring button in the sidebar

- **WHEN** a signed-in reader with a stored CV opens a posting
- **THEN** the sidebar match block renders no `Tailor my CV` button

#### Scenario: The upload prompt survives

- **WHEN** the page has read that the signed-in reader has no stored CV
- **THEN** the sidebar match block shows the `Upload a CV to analyse` prompt with its profile link

#### Scenario: The cached analysis card survives

- **WHEN** a fit analysis is cached for the reader and the posting
- **THEN** the sidebar match block shows the percentage, the verdict, the top gap and the link to the full analysis

#### Scenario: The allowance is stated once, in the dialog

- **WHEN** the reader has tailoring sessions left today
- **THEN** the sidebar match block states no count
- **AND** the confirmation dialog raised from the CTA row states it

#### Scenario: One read of the match analysis per page

- **WHEN** the job detail page renders its CTA row and its sidebar match block
- **THEN** the match-analysis endpoint is called at most once for that posting

## REMOVED Requirements

### Requirement: The analysis offer is shown to a guest

**Reason**: The offer is no longer made from the sidebar, so a guest-only rendering of it
there has nothing to render. The CTA row's `Tailor my CV` button is now shown to an
unauthenticated reader for the same reason this requirement existed — the offer is what brings
them to sign in — and opens sign-in in place rather than navigating.

**Migration**: The guest behaviour is restated by "Tailoring the CV is the page's single
primary CTA" in `job-page-actions`, whose scenarios cover the sign-in-in-place rule and the
withholding of the button once the page knows the reader has no CV (which is what kept the
no-profile state from offering the same next step twice).
