# internal/api

The HTTP edge — handlers, middleware, realtime transport, OG images. The only block nothing else may import.

**Layer 8 of 8.**

May import: `platform`, `dict`, `ai`, `identity`, `candidate`, `job`, `application`, `search`, `engage`, `ingest` — and itself.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`apigen` `atsapply` `candidateprofile` `handler` `ogimage` `realtime`

## The HTTP contract (TypeSpec → OpenAPI → ogen)

Routes are moving, one group at a time, to a contract in `typespec/`. `make api-contract`
compiles it to `openapi/api.openapi.json` and generates the ogen server in `apigen`
(never edit either by hand; CI's `api-contract` job regenerates and diffs). `handler`
implements `apigen.Handler`: a migrated group embeds its handlers in `contractHandlers`
(`handler/contract.go`), so a missing or mistyped operation does not compile.

A migrated route is still a Fiber route: it keeps its middleware chain and ends in the
`contract` handler, which runs the generated server through the net/http adaptor.
- The caller's id comes from `auth.UserIDFromContext(ctx)`: Fiber Locals are fasthttp
  user values, and the adaptor passes the RequestCtx on as the request context.
- Middleware that marks Fiber's user context (`mw.paidAI` → `plan.WithPaidAI`, the trace
  span) reaches the handler's `ctx`: the wrapper merges `c.UserContext()` into it. Do not
  re-mark inside a handler.
- Return errors as before (`fiber.NewError`, domain errors); `NewError` renders them
  through `classify`, and faults are reported with the Fiber route's attributes.
- Lists are `List<T>` and a single item that reads query params is `DataWithIgnored<T>`
  (`typespec/common.tsp`). A filter param the endpoint cannot read is not refused: declare
  it as a string, parse `contractQuery(ctx)` as before, and pass the report through
  `contractIgnoredParams`. Page params are strings too, read by `contractPageBounded` (the
  same clamp as `pageParamsBounded`); a list with no filter report uses `PageMeta`.
- The generated encoder leaves out only an unset `Opt*` and a nil slice. A Go field that
  was `omitempty` maps through `optString` / `nilIfEmpty`, or the answer changes shape.
- The wrapper hands the generated router the request as Fiber already accepted it: the
  matched route's own path (any letter case, one trailing slash) and repeated params cut to
  the first value (`fiberTarget`). An exploded array
  param would be cut too, so that function changes before one is declared.
- A patch field that was a Go pointer is `field?: T | null` in TypeSpec: absent and `null`
  both keep the stored value, as `json.Unmarshal` into a pointer did. `ptrOf` / `optNil`
  in `contract.go` convert between the generated `OptNil*` and the domain pointer.
- Timestamps keep their fraction (RFC3339Nano, as `encoding/json` wrote them):
  `make api-contract` runs `scripts/openapi-time-format.mjs`, which gives every date-time
  ogen's `x-ogen-time-format`. Without it ogen writes whole seconds.
- A binary answer (an image) works: `@body photo: bytes` with a fixed `contentType`.
- Streams (SSE), websockets and redirects stay hand-written Fiber.
- The contract is also the public API reference: `web/scripts/gen-api-docs.mjs` renders
  `/docs/api`, `/docs/api/internal` and `docs/API*.md` from `openapi/api.openapi.json`.
  So an operation carries its reader-facing text: `@summary`, a doc comment, doc comments
  on its params and body fields, and an `@opExample` (TypeSpec type-checks it against the
  operation, so a stale example fails `make api-contract`). The overview and tag
  descriptions live in `typespec/docs.tsp`.
- `@useAuth` states who may call an operation and must match the Fiber gate in front of
  it: the namespace default is the cookie (`mw.cookie`); `mw.key` is
  `@useAuth(SessionOrKey)`; a route with no gate or `mw.optional` is `@useAuth(NoAuth)`.
  It decides which reference documents the operation (a key-callable or public one is
  external, a cookie-only one internal). `@extension("x-audience", "moderator")` marks a
  moderator route, `"hidden"` one no client can use. ogen never sees `@useAuth`
  (`scripts/openapi-ogen-input.mjs` strips it), so it gates nothing.
- Shared mapping helpers live in `handler/contract.go` and nowhere else; reuse them, and put
  a new one there rather than in a group's file (groups moved in parallel wrote the same
  helper twice and broke the build when they met). Present: `optString`, `nilIfEmpty`,
  `ptrOf`, `intPtrOf`, `optNil`, `optNilInt`, `nilString`, `nilDateTime`, `contractQuery`,
  `contractIgnoredParams`, `contractIgnoredValues`; in `helpers.go` `requireUserIDFromContext`, `contractPathID`; in
  `handler.go` `contractPageBounded`, `contractPageWindowed`.
