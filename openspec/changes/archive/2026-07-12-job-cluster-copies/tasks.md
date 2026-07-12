## 1. Backend

- [x] 1.1 `ListRoleClusterCopies` query (open cluster members by location) + sqlc; integration test.
- [x] 1.2 `JobCopies` handler + `/jobs/:slug/copies` route.

## 2. Frontend

- [x] 2.1 `getJobCopies` API client + `JobCopy` type.
- [x] 2.2 Detail loader parallel fetch (degrade to []) + `JobCopies.svelte` section on the page.
