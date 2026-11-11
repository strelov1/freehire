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
- A binary answer (an image) works: `@body photo: bytes` with a fixed `contentType`.
- Streams (SSE), websockets and redirects stay hand-written Fiber.
