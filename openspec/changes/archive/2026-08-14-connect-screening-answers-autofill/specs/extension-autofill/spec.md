## ADDED Requirements

### Requirement: The autofill profile also carries the candidate's screening answers

The autofill profile block SHALL, in addition to the identity contact fields, carry the
candidate's own screening answers (`internal/screeninganswers`): `authorized_countries`,
`visa_sponsorship_needed`, `desired_salary`, `notice_period`, `willing_to_relocate` and
`age_18_or_older`. A field the candidate has not stated SHALL be empty rather than guessed,
on the same terms as an unstated identity field.

These fields are independent of the identity contact block: there is exactly one
screening-answers store, so there is no multi-source precedence to resolve for them the way
there is for name/email/phone.

#### Scenario: Screening answers ride alongside the identity fields

- **WHEN** an authenticated caller reads the autofill profile and has stated their desired
  salary and their willingness to relocate, but no other screening answer
- **THEN** the response carries those two values and empty strings for the other four
  screening fields, alongside whatever identity fields the CV/résumé/account state

### Requirement: The deterministic fallback filler answers screening questions

When the browser extension's deterministic fallback filler is in use — the path taken when
the agent-driven autofill is unavailable — it SHALL recognize application-form questions
asking about work-authorized countries, visa sponsorship, desired salary, notice period,
willingness to relocate, and age (18+), and fill them from the caller's screening answers
using the same label-matching mechanism it already uses for identity fields. A question the
caller has not answered in their profile SHALL be left blank rather than filled with a
guess, matching the existing rule for identity fields.

#### Scenario: A notice-period question is filled from the profile

- **WHEN** the deterministic filler is planning fills for a form asking "What is your notice
  period?" and the caller's screening answers state a 30-day notice period
- **THEN** the plan fills that question with "30 days"

#### Scenario: An unanswered screening question is left blank

- **WHEN** the deterministic filler is planning fills for a form asking about desired salary
  and the caller has not stated one
- **THEN** the plan leaves that question unfilled

#### Scenario: An authorized-countries checkbox group is filled by country name, not by code

- **WHEN** a form asks "Which countries are you authorized to work in?" as a checkbox group
  offering full country names, and the caller's screening answers state authorization in the
  US and Germany
- **THEN** the plan checks the "United States" and "Germany" options, not a literal "US, DE"
  that would match neither

### Requirement: A boolean work-authorization question is not answered from the country list

A plain Yes/No "are you authorized to work [here]?" question SHALL NOT be matched to the
authorized-countries screening answer. That answer is a list of country codes, not a
boolean, and the profile carries no boolean field stating plain work authorization — filling
the question would put the wrong shape of value into a Yes/No control.

This does not apply to a question that itself asks *which* countries the candidate is
authorized to work in (a list question), which the previous requirement covers.

#### Scenario: A boolean work-authorization question stays unmatched

- **WHEN** an application form asks "Are you authorized to work in this country?" as a
  Yes/No question
- **THEN** the deterministic filler does not match it to any screening answer and leaves it
  for the caller
