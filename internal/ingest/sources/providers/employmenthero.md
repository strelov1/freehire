# Employment Hero

Employer pages use `employmenthero.com/jobs/organisations/<slug>/`. The company-page jobs component reads `services.employmenthero.com/ats/api/v1/career_page/organisations/<slug>/jobs` with `page_index` and `item_per_page`. The jobs listing includes the description and a stable `job_id`. The public URL uses the separate `friendly_id` under `/jobs/position/`.

The Employment Hero company returned 70 postings with full descriptions across two pages at 50 per page on 2026-11-22. Its submitted position named the organisation page through its hiring organisation metadata. The posting slug by itself does not identify the employer.
