# Match card on a job page

On a page the catalogue knows, the Match tab shows the job, the profile match percentage and
the skills the candidate has and misses.

## Sub-features

- Job recognised from the page URL (`/jobs/find`).
- Profile match with "You have" / "Missing" skills.
- Analyze match, Tailor my CV (paid AI; do not trigger without the owner's ok).

## How to get to it (user POV)

Open a job posting, open the freehire panel, read the Match tab.

## Driving it with cua_*

1. `cua_launch_app {bundle_id:"com.google.Chrome", urls:["https://www.linkedin.com/jobs/view/4479789544/"]}`
   (Realnaut, Desarrollador/a Senior Java; any open LinkedIn job works).
2. Open the panel (toolbar `freehire`), screenshot.
3. Proof: the card names the company and title of the page, shows `PROFILE MATCH` with a
   percentage, and an `Autofill` button at the bottom.

## Gotchas

- A LinkedIn search URL (`/jobs/search/?currentJobId=…`) is recognised only once the
  currentJobId canonicalisation is deployed to production; until then use `/jobs/view/<id>/`.
- `Tailor my CV` and `Analyze match` spend the owner's daily AI allowance.
