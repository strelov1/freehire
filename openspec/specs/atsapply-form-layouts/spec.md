# atsapply-form-layouts Specification

## Purpose

Says which employer application platforms auto-apply may drive a browser against, and what
it has to know about each one's page to do so without risking a submission it cannot take
back.

## Requirements

### Requirement: Only a platform this system has measured may be driven

The system SHALL drive a browser against an employer's application page only for a platform
whose page it holds a recorded description of. A platform with no such description SHALL
reach neither a browser nor a submit click, and its attempts SHALL be recorded as unresolved
for the reason that is true of them — that no fill path exists — never for an invented one.

The description SHALL consist of values read off a real posting, never inferred from the
page at run time. Locating the form by a general property (the element carrying the most
inputs) or the submit control by its text is prohibited: a submit click cannot be withdrawn,
and this system has twice acted on an inference about a page nobody had loaded.

#### Scenario: An unmeasured platform is never driven

- **WHEN** a queued attempt's platform has no recorded page description
- **THEN** no browser is launched and no submit control is clicked for it
- **AND** the attempt is recorded as unresolved because no fill path exists for that platform

#### Scenario: A platform this system can fill always has a description

- **WHEN** the set of platforms the system will fill and the set it holds descriptions for
  are compared
- **THEN** every platform in the first set appears in the second

### Requirement: A field is identified and selected by the same attribute

The system SHALL identify a scanned field and locate that field on the page by the SAME
attribute, decided per platform. Identifying by one attribute while selecting by another
SHALL NOT be possible.

This is a single decision because the two failing to agree produces no error: every field
resolves, the plan reports itself complete, and nothing is found on the page — after the
model spend and after the candidate has approved the application.

#### Scenario: A platform whose controls carry no id is addressed by name

- **WHEN** a platform's application form renders controls that carry no `id` attribute
- **THEN** each field is identified by its `name`
- **AND** the same `name` is what locates the field on the page

#### Scenario: A control that cannot be addressed is not offered as a field

- **WHEN** a scanned control carries neither the attribute its platform is addressed by nor
  any other means of locating it
- **THEN** it is omitted from the scanned field inventory
- **AND** it is never reported as a field the system will fill

### Requirement: An unconfirmed submission is never retried

The system SHALL treat a submit click that produces no confirmation as its own outcome,
distinct from both success and from a transient failure, and SHALL NOT retry it.

A retry cannot distinguish an application that never went through from one that did and
merely did not say so, and the cost of the two mistakes is not symmetric: a missing
application costs the candidate one opportunity, a duplicate costs them their credibility
with that employer.

#### Scenario: A submit click that is not acknowledged

- **WHEN** the system clicks a platform's submit control and no confirmation appears within
  the time it waits
- **THEN** the attempt is recorded as unconfirmed
- **AND** no further submission is attempted for that application
