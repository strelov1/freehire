## 1. Schema & wire shape

- [ ] 1.1 Add migration `0024_job_collections.sql`: `companies.collections TEXT[] NOT NULL DEFAULT '{}'` and `jobs.collections TEXT[] NOT NULL DEFAULT '{}'`
- [ ] 1.2 Add `Collections []string json:"collections"` to `internal/jobview` and populate it from the `jobs.collections` column in `FromRow` (dict-only, like skills)
- [ ] 1.3 Update `internal/db/queries/*.sql` so reads/writes carry the new columns and run `make sqlc`; update `UpsertJob`/company queries as needed

## 2. Collections registry & matching (`internal/collections`)

- [ ] 2.1 Define the registry type `{Slug, Title, Description, Resolver}` and the fixed v1 set (`yc`, `bigtech`) with a lookup-by-slug; unit-test the registry shape
- [ ] 2.2 Implement the `bigtech` resolver as a hand-coded slug list; unit-test it returns exactly the listed slugs
- [ ] 2.3 Implement the `yc` resolver: parse a `yc-oss` dataset payload and match entries to company slugs via `normalize.Slug`, returning matched slugs + unmatched entries; unit-test matching, omission of unmatched, and dedup (use a fixture payload)

## 3. Search facet plumbing

- [ ] 3.1 Add `collections` to the search document and to `FilterableAttributes` in `internal/search/client.go`
- [ ] 3.2 Add `collections` to the query-param→filter map in `internal/search/query_filter.go`; unit-test that `collections=yc` produces the expected Meili filter
- [ ] 3.3 Add an integration-tagged search test asserting a job with `collections=[yc]` is returned by a `collections = "yc"` filter and reported in the `collections` facet distribution

## 4. Import worker (`cmd/import-collections`)

- [ ] 4.1 Add a propagation query `UPDATE jobs SET collections = c.collections FROM companies c WHERE jobs.company_slug = c.slug AND jobs.collections IS DISTINCT FROM c.collections` (in `queries/*.sql`, regenerate sqlc); integration-test it copies membership onto jobs
- [ ] 4.2 Implement `cmd/import-collections`: for each registry collection resolve members, write `companies.collections` (idempotent, only managed tags), run propagation, log unmatched, print a reindex reminder
- [ ] 4.3 Add `import-collections` to the Dockerfile build + COPY list

## 5. Web pages

- [ ] 5.1 Mirror the collection registry as a small web contracts list (slug/title/description); note the gen-contracts seam
- [ ] 5.2 Add `/collections` index route: SSR the registry with per-collection open-job counts from the `collections` facet distribution
- [ ] 5.3 Add `/collections/[slug]` route: reuse the faceted job feed with `collections=<slug>` locked on; unknown slug → not found; SSR first page
- [ ] 5.4 Add a `/collections` link to the site nav

## 6. Verification

- [ ] 6.1 `go build ./... && go vet ./... && go test ./...` green; integration tests for db/search compile and pass
- [ ] 6.2 `svelte-check` + lint on `web/`; manual smoke of `/collections` and `/collections/yc` against a seeded DB
