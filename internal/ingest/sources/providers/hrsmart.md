**Deltek HRsmart traps** (`hrsmart`; public Gist tenant sampled 2026-11-16):

- Board is the entire tenant host, including regional instance:
  `gistworld.mua.hrdepartment.com`. A native identity is `board:3735`, not the
  LinkedIn discovery id. Canonical URLs use `/hrsmart/ats/Posting/view/<id>`.
- Read `/hrsmart/ats/JobSearch/viewAll`, the results table and explicit next-page
  control. Follow observed next links on the same host; require a stable reported total
  and exactly that many distinct ids at the disabled final control. Missing markup,
  partial pagination, cycles and count drift fail rather than authorize a close sweep.
- Detail fields are `job_details_ats_requisition_title`,
  `job_details_hua_location_id` and `job_details_ats_requisition_description`.
  Gist returned 50 jobs; posting 3735 was Senior Java Software Engineer in Basingstoke.
- HydratingSource skips seen bodies and allows link resolution to hydrate only its target.
  Transient detail failures emit Unreadable identities; an entirely unreadable result
  fails. A detail 404/410 is dropped as gone. The complete listing permits the ordinary
  ATS lifecycle; the adapter does not hard-delete postings or depend on LinkedIn liveness.
