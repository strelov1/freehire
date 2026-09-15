## Purpose

Crawls Joppy (joppy.me), a Spain-only tech hiring platform with no search API, by walking its
public company directory each run and reading every company's open postings straight off that
company's own page.

## ADDED Requirements

### Requirement: Directory-wide crawl

The system SHALL discover every company Joppy publishes from the platform's own sitemap, then
fetch each company's own page and read that company's currently open postings from it, and SHALL
return one `Job` per open posting found across the whole directory.

#### Scenario: A crawl returns postings from every company that has any

- **WHEN** the adapter crawls the Joppy directory and some companies currently have open postings
  while others have none
- **THEN** it returns one `Job` for every open posting on every company that has at least one, and
  nothing for a company with none

#### Scenario: A company's page fetch fails without aborting the crawl

- **WHEN** one company's page cannot be fetched or read while the rest of the directory is
  reachable
- **THEN** the adapter skips that company and still returns the postings of every other company it
  could read

### Requirement: Boardless, multi-company source

The adapter SHALL be boardless: it needs no per-employer board configuration, and each returned
`Job`'s company SHALL come from the posting's own employer, not from a configured board.

#### Scenario: Two postings from different employers both crawl under one registry entry

- **WHEN** the directory carries open postings from more than one employer
- **THEN** a single crawl of the one `joppy` registry entry returns postings for all of them, each
  carrying its own employer's name as `Job.Company`

### Requirement: Posting normalization

For each posting the adapter SHALL map: the platform's own posting identifier to `ExternalID`; the
posting's public URL; the title; the HTML body to a sanitized `Description`; the work arrangement
the platform states (fully remote / hybrid / onsite) to `WorkMode`; and the required skills to
`Skills`, canonicalized through the skill dictionary. A posting stating no structured location
beyond its work-arrangement flags SHALL still be returned, with `Location` built from whatever
place information the posting does state. The adapter SHALL NOT map the platform's own required
language level onto `EnglishLevel`: Joppy states it on a 1-5 numeric scale with no authoritative
equivalence to freehire's CEFR-based vocabulary, so a required language and its stated level
SHALL instead be folded into the description text (see the "Facts without a structured field"
requirement below), never guessed into a CEFR bucket.

#### Scenario: A posting with full structured signal maps completely

- **WHEN** a posting states a title, a body, a work arrangement, and required skills
- **THEN** the resulting `Job` carries all of them, with skills canonicalized, `EnglishLevel` left
  unset, and the posting's required language level readable in the description text instead

#### Scenario: A posting missing an identity field is dropped

- **WHEN** a posting carries no stable posting identifier or no title
- **THEN** the adapter drops that posting rather than yielding a `Job` with no dedup key or no name

### Requirement: Salary published only when the employer opted in

The adapter SHALL set `SalaryMin`/`SalaryMax`/`SalaryCurrency`/`SalaryPeriod` only for a posting
the platform marks as publicly showing its salary range, and SHALL leave them unset otherwise —
even when the platform's own data carries a range internally for a non-public posting.

#### Scenario: A public salary range is published

- **WHEN** a posting is marked as publicly showing salary and states a minimum and a maximum
- **THEN** the resulting `Job` carries that range with its currency and period set

#### Scenario: A non-public salary is never published

- **WHEN** a posting is not marked as publicly showing salary
- **THEN** the resulting `Job` carries no salary range, regardless of what the platform's own data
  holds internally for that posting

### Requirement: Facts without a structured field are not silently dropped

For a posting fact the platform states but freehire's `Job` shape has no dedicated field for today
(visa sponsorship offered, a relocation package offered, EU-candidates-only eligibility, which
individual skills are must-haves versus nice-to-haves, and a required language's stated level),
the adapter SHALL fold that fact into the posting's description text rather than discarding it.

#### Scenario: Visa sponsorship is stated in the description

- **WHEN** a posting states that the employer sponsors a work visa
- **THEN** the resulting `Job`'s description includes that fact

#### Scenario: A required language's level is stated in the description

- **WHEN** a posting states a required language and a level on the platform's own scale
- **THEN** the resulting `Job`'s description includes that language and its stated level
