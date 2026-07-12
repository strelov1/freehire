## 1. Tabbed related section

- [x] 1.1 `JobRelated.svelte` — Similar/Other-locations tabs; locations tab shows 10 + "View all" link. Replace `JobCopies` usage on the detail page; remove `JobCopies.svelte`.
- [x] 1.2 Detail loader fetches a 10-copy preview; `getJobCopies` gains `limit`/`offset`.

## 2. Full-list page

- [x] 2.1 `/jobs/[slug]/copies` route (+page.server.ts loader with offset pagination + +page.svelte list). SEO/title from the anchor job.

## 3. Verify

- [x] 3.1 `npm run check` + `npm run lint` clean; visual-verify the tabs + full page render.
