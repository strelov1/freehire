# JOIN

The board is the numeric company ID. Since 2026-11-15, the careers site's public
API is GraphQL at `https://join.com/candidate-api/graphql`. The old
`/api/public/companies/<id>/jobs` and `/api/public/jobs/<id>` routes return
`404 Cannot GET` even for JOIN's own live company (54); that is not board deletion.

POST `publicJobs(input: PublicJobsQueryInput!)` with `companyId` and
`paginationPage: {page, pageSize}`. Retain the UI's page size of five and the
existing 1.5 requests/second pace across listing and detail requests. Read every
declared page, require explicit `items` and nonnegative `pageInfo.rowCount` and
`pageCount`, and reject changing totals, repeated identities, malformed items or
an incomplete final count. GraphQL errors fail the operation, including HTTP 200
responses carrying partial data.

`publicJob(input: PublicJobInput!)` takes the native posting `id` and returns
`descriptionHtml` and `company.domain`. Sanitize the HTML directly. Display URLs
use the list's `idParam`, which differs from the native identity. Failed, null or
bodyless details retain an unreadable listed identity.

Live control: company 54 returns three postings; native job 14771122 returns a
complete HTML body. Company 176781 and unknown company 999999999 both return
`items: []`, `pageCount: 0`, `rowCount: 0`. Therefore JOIN grants full listing
coverage but no verified-empty marker. Neither the old 404 nor the new zero
response alone justifies retiring a catalog company.
