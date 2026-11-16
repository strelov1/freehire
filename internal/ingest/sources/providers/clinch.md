# ClinchTalent

Board is the career-site hostname. Read `/sitemap.xml`, retain `/jobs/` URLs and
use the UUID suffix or full slug as the external ID. Detail metadata does not change
identity, company ownership or sitemap timestamps.

On 2026-11-15, real Mader and Armaguard details exposed `.job-title` and
`.job-component-location` components. Prefer their text to slug guesses; preserve
punctuation, abbreviations and every explicit location. Other verified themes use
`.job-location` spans or `.job-component-location-group`; repeated responsive
copies are deduplicated. Physical locations take priority over location groups. A generic h1 can be page
chrome (Mader says `Careers`), and the document/OG title appends geography.
See the reduced real fixtures under `testdata/clinch/`.

Slug geography remains a best-effort fallback when explicit components are absent.
Do not adjust the location dictionary to compensate for ambiguous job-title words.
A detail challenge latches hydration off for the run; emit all sitemap jobs with
fallback metadata and empty bodies. Ordinary detail errors do not latch or drop
postings. This adapter does not declare `fullBoardListing` or `emptyBoardListing`;
a sitemap-only run does not establish readable-body coverage or prove an employer
has no vacancies. Do not use such a run to justify retiring its former PageUp board.
