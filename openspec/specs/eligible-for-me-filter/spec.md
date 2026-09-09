# eligible-for-me-filter Specification

## Purpose

Lets a job-search visitor narrow results to postings plausibly open to them
geographically, in one action, without hiding postings whose geography is
worldwide or was never resolved by the location dictionary.

## Requirements

### Requirement: The filter composes existing location facet values, never a new one

Turning the filter on SHALL stage only facet include values the search API already
supports (`countries`, `regions`, and the existing `regions=none` "unspecified"
sentinel) within the existing location facet group. It SHALL introduce no new
backend filter, query parameter, or stored column — the geography-based narrowing
it performs SHALL be fully expressible as a combination of values the search API
already accepts.

#### Scenario: A known country stages a country value plus the safety values

- **WHEN** the visitor's country is known (e.g. `id`) and the filter is turned on
- **THEN** the staged location group includes `countries=id`, `regions=global`, and
  `regions=none`, ORed together as one group

#### Scenario: A known region only (no specific country) stages a region value plus the safety values

- **WHEN** only the visitor's macro-region is known (e.g. `apac`) and the filter is
  turned on
- **THEN** the staged location group includes `regions=apac`, `regions=global`, and
  `regions=none`, ORed together as one group

### Requirement: The safety values are always included and never omitted

Whenever the filter stages a country or region value, it SHALL also stage
`regions=global` and `regions=none`, regardless of how the country/region was
sourced. This SHALL hold even though the two safety values are the same for every
visitor, because they are what keeps worldwide-remote postings and postings whose
location never resolved out of the "not eligible" bucket the filter narrows to.

#### Scenario: A posting with no resolved geography stays visible

- **WHEN** the filter is turned on
- **THEN** a posting whose derived `regions` and `countries` are both empty (no
  geography resolved) is still included in the results, because `regions=none`
  matches it

#### Scenario: A worldwide-remote posting stays visible regardless of the visitor's location

- **WHEN** the filter is turned on for a visitor in any country or region
- **THEN** a posting whose `regions` includes `global` is still included in the
  results

### Requirement: The country/region source follows a fixed precedence and degrades gracefully

The filter SHALL resolve the visitor's geography in this order, using the first
source that yields a value:
1. The signed-in candidate's profile `location_preferences.base.country`, when set
   (country-level).
2. The macro-region derived from the visitor's edge country header, via the
   existing `geo-default-scope` resolution (region-level) — available to signed-in
   and anonymous visitors alike.

When neither source yields a value (no profile country, and the edge header is
absent, reserved, or maps to no region — e.g. local development, a non-Cloudflare
path, or a Tor/unplaceable address), the filter SHALL have nothing to stage, and
SHALL be presented as unavailable rather than staging an empty or wrong narrowing.
No manual entry step SHALL be required from the visitor.

#### Scenario: A signed-in candidate with a profile country gets country-level precision

- **WHEN** the candidate is signed in and their profile has `location_preferences.base.country`
  set
- **THEN** turning the filter on stages that exact country, not just its region

#### Scenario: An anonymous visitor gets region-level precision from the edge header

- **WHEN** the visitor is not signed in (or is signed in without a profile country)
  and the edge country header resolves to a region
- **THEN** turning the filter on stages that region

#### Scenario: No source available leaves the filter unavailable

- **WHEN** neither the profile country nor the edge-derived region is available
- **THEN** the filter is presented as unavailable and stages nothing when interacted
  with

### Requirement: The filter is strictly opt-in

The filter SHALL never be applied automatically — not on first visit, not as a
default search scope, and not re-applied silently after being turned off. It SHALL
only take effect when the visitor explicitly turns it on, and SHALL remain off
until they do.

#### Scenario: A fresh search has the filter off by default

- **WHEN** a visitor opens the job search with no prior interaction with this filter
- **THEN** none of `countries`, `regions=global`, or `regions=none` are staged on
  their behalf by this filter

### Requirement: Turning the filter off removes exactly what it staged

Turning the filter off SHALL remove exactly the facet values it staged (the
resolved country or region, plus `regions=global` and `regions=none`), leaving any
other location selection the visitor made independently untouched.

#### Scenario: Turning off does not disturb an unrelated manual selection

- **WHEN** the visitor has separately selected a country pill by hand, then turns
  the "Eligible for me" filter on and back off
- **THEN** their manually-selected country pill remains staged, and only the
  filter's own values are removed
