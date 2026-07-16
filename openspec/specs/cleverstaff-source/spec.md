# cleverstaff-source Specification

## Purpose

Crawl one CleverStaff (cleverstaff.net) tenant's open vacancies into the catalogue via the
keyless `getAllOpenVacancy?alias=<board>` endpoint, mapping each object into a normalized `Job`
keyed by the tenant alias, with per-vacancy employer resolution for staffing-agency tenants.

## Requirements

### Requirement: CleverStaff per-tenant vacancy fetch

The system SHALL provide a `cleverstaff` source adapter that fetches one tenant's open
vacancies from `https://cleverstaff.net/hr/public/getAllOpenVacancy?alias=<board>`, where
`<board>` is the configured board file entry's `board` (the CleverStaff tenant alias). The
response is a JSON document `{"status":"ok","orgId":…,"objects":[…]}`; the adapter SHALL map
each element of `objects` to one `Job` — no per-vacancy detail request is made, because the
list already carries the full description.

The adapter is a **per-tenant ATS** keyed by `board`: it requires a board (it is neither
boardless nor an aggregator), stays a first-party source, and its postings SHALL NOT be
enrolled in the aggregator ATS-suppression pass.

#### Scenario: A tenant's vacancies map to jobs

- **WHEN** the adapter fetches a tenant whose `objects` array carries open vacancies
- **THEN** it yields one `Job` per element carrying the title (from `position`), description
  (from `descr`, sanitized), the `vacancyId` as `ExternalID`, the URL
  `https://cleverstaff.net/i/vacancy-<localId>`, work mode (mapped from `workCondition`),
  employment type (mapped from `employmentType`), and posted-at (from `dc`/`dm` epoch-ms)

#### Scenario: Non-ok payload is a board failure

- **WHEN** the fetch returns a document whose `status` is not `"ok"`, or the request fails
- **THEN** the adapter returns an error (so `board_health` cools the board) rather than
  yielding zero jobs silently

### Requirement: CleverStaff maps only cleanly-structured facets

The adapter SHALL set a structured facet only when CleverStaff states it unambiguously:
`workCondition` maps to `WorkMode` (remote/hybrid/onsite) and `employmentType` maps to a
recognized `EmploymentType` value, each emitting nothing for an unrecognized value. The
adapter SHALL NOT map `role` or `experience` to seniority, leaving those to the downstream
title dictionaries.

#### Scenario: Structured work mode and employment type are carried

- **WHEN** a vacancy has `workCondition: "remote"` and `employmentType: "fullEmployment"`
- **THEN** the yielded `Job` carries `WorkMode: "remote"` and `EmploymentType: "full_time"`

#### Scenario: Unrecognized structured values are left empty

- **WHEN** a vacancy's `workCondition` or `employmentType` is a value with no clean mapping
- **THEN** the corresponding `Job` field is left empty so the dictionaries decide

### Requirement: CleverStaff drops a vacancy it cannot key or address

The adapter SHALL drop an object that lacks a `vacancyId` (no dedup key), a `localId` (no
canonical URL), or a `position` (no title), rather than yielding an unusable `Job`. A vacancy
whose `status` is not open SHALL be filtered out. A single dropped object SHALL NOT abort the
rest of the tenant's mapping.

#### Scenario: Object without id, localId, or position is dropped

- **WHEN** an object in `objects` has no `vacancyId`, no `localId`, or no `position`
- **THEN** the adapter drops that object and continues mapping the rest of the tenant

#### Scenario: A non-open vacancy is excluded

- **WHEN** an object's `status` indicates the vacancy is not open
- **THEN** the adapter does not yield a `Job` for it

### Requirement: CleverStaff resolves the employer per Hub

For a tenant configured with `hub: true`, the adapter SHALL set each `Job.Company` from the
object's `clientName`, falling back to the configured company when `clientName` is blank. For
a tenant without `hub`, every `Job` SHALL keep the configured company regardless of
`clientName`.

#### Scenario: Agency tenant attributes vacancies to their client

- **WHEN** a `hub: true` tenant's vacancy carries `clientName: "Acme Corp"`
- **THEN** the yielded `Job.Company` is `"Acme Corp"`, not the tenant's configured company

#### Scenario: Ordinary tenant keeps its configured company

- **WHEN** a tenant without `hub` yields a vacancy carrying some `clientName`
- **THEN** the yielded `Job.Company` is the configured company from the board file
