**TriNet Hire traps** (`trinethire`; verified live 2026-10-24):

- **Only the numeric id addresses a company.** The board is the path segment `<id>-<slug>` as a posting link spells it, but `/companies/16150/jobs` and `/companies/16150-anything/jobs` serve the same page, so `boardIdentity` folds the slug off. The posting id is global across the platform.
- **The listing's location column is a NAME the employer chose, not a place** ("Automox Inc. HQ", "Remote in the Midwest", a recruiter's name). The place is the posting page's `City, ST` line, which takes wildcards (`*, NY`, `All, All`). The adapter keeps the address, drops a wildcard part, and lets the label contribute only the word "Remote".
- **The salary line prints a unit only for hourly pay** ("$45.00 - $50.00 per hour"). A line with no unit is the annual type, except where an employer typed an hourly figure into it ("$23 - $25") — so an annual figure under 1,000 is dropped.
- No posting date, no JSON, no schema.org markup; one unpaginated listing table (largest of 100 tenants: 30 rows).
