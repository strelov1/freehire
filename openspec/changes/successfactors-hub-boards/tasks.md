## 1. The tenant key

- [x] 1.1 Add `sfTenant(loc string) string` to `internal/ingest/sources/successfactors.go`,
      returning the job URL's first path segment, and `""` when the path carries no tenant —
      the tenant-less `/job/<slug>/<id>/` shape, a bare host, or an unparseable URL. Cover
      each of those with table tests, including that the literal `job` is never a tenant.

## 2. The board-file field

- [x] 2.1 Add `Tenants map[string]string \`yaml:"tenants"\`` to `sources.CompanyEntry` beside
      `Region` and `Hub`, documented as an optional per-entry map from a hub's tenant key to
      the employer's display name, ignored by adapters that do not implement hub resolution.
      Prove it decodes from YAML with a loader test.

## 3. Hub resolution in the adapter

- [x] 3.1 In `successfactors.detail`, resolve the job's company through `e.Tenants` from
      `sfTenant(entry.Loc)` when `e.Hub` is set, falling back to `e.Company` on a missing or
      unmapped key. Adapter tests over a fixture sitemap and job pages: a mapped tenant, an
      unmapped tenant, a tenant-less URL, and a non-hub board whose company is unchanged.

## 4. The board

- [x] 4.1 Convert the existing `jobsearch.createyourowncareer.com` entry (today `company: Arvato`) into a hub entry in
      `sources/successfactors.yml` with `hub: true`, `company: Bertelsmann` and the 23
      curated tenant names, then run `go run ./cmd/validate-sources`.

## 5. Verify against the live board

- [x] 5.1 Run the adapter against the live board and confirm the employers land as intended:
      mapped tenants carry their own name, unmapped ones read `Bertelsmann`, and no job is
      attributed to a company called `job`.
