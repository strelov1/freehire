## ADDED Requirements

### Requirement: Discussion tab
The company page's tab strip SHALL carry three tabs: **Jobs**, **Hiring** and
**Discussion** (`/companies/<slug>/discussion`). The Discussion tab's label SHALL carry
the company's open-thread count when it is non-zero (`Discussion · 3`). The discussion
index page SHALL render the company header and the tab strip above the thread list when
the company resolves under the SAME slug, and the header-less layout it had before when
the company is gone, unreachable, or merged into another (a merged company's discussion
stays at the retired slug, whose threads the canonical company's Discussion tab would not
show). The company header SHALL no longer carry its own Discussion link.

#### Scenario: Discussion of a listed company
- **WHEN** a visitor opens `/companies/acme/discussion` and `acme` resolves
- **THEN** the page shows Acme's header, the three tabs with Discussion selected, and the threads

#### Scenario: Discussion of a merged company
- **WHEN** `/companies/acme-inc/discussion` resolves to the canonical company `acme`
- **THEN** the page keeps the header-less layout naming Acme, without tabs

#### Scenario: Discussion that outlived its company
- **WHEN** the company behind the slug no longer exists
- **THEN** the page shows the threads under the existing "no longer listed" subject header, without tabs

### Requirement: Posting views on the Hiring tab
The hiring endpoint SHALL return `views` and `views_prev` from `insights_company_views`
(zeros when the company has no row). The Hiring tab SHALL show the 30-day views and their
change, labelled so a reader knows one visitor counts once per posting per day.

#### Scenario: Views served
- **WHEN** a company's rollup row has `views = 2114`, `views_prev = 1800`
- **THEN** the endpoint returns both and the card shows 2,114 with +314 against the previous 30 days

#### Scenario: No views
- **WHEN** a company has no row
- **THEN** the endpoint returns `views = 0`, `views_prev = 0`
