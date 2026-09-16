## Context

This is a specification-only change. The behaviour it describes is already in production
and already tested; what is missing is the requirement that admits it.

`openspec/specs/error-tracking/spec.md` was written when the frontend reported everything
and the backend's exceptions were three. Since then #2892 added `web/src/lib/sentryNoise.ts`
and a `beforeSend` on both hooks, and moved `errNoRequirementList` to the non-reported side
of `classify()`. Two live scenarios now say, unconditionally, that an unhandled browser or
SSR error MUST be captured — and the shipped filter drops a subset of them.

A spec that contradicts working code is not a harmless stale document. It is an argument
for deleting the code, addressed to whoever reads it next.

## Goals / Non-Goals

**Goals:**

- The `error-tracking` requirements describe what ships.
- The backend's list of non-reported failures stops being a list.

**Non-Goals:**

- Any behaviour change. No runtime code, no tests, no configuration is touched.
- Revisiting WHICH conditions are filtered. That set is argued in `sentryNoise.ts` and
  pinned by its test; this change records the principle, not the membership.
- The Sentry quota itself, and the source-map credential — separate concerns, the second
  handled by `restore-sentry-sourcemaps`.

## Decisions

### Describe the rule, not the membership

The frontend requirement gains "report a defect, not a condition" and three shapes of
condition (transport, cancellation, a chunk the deploy replaced). It does not enumerate
message fragments. The fragments are an implementation detail that changes whenever a
browser rewords an error; a specification that listed them would be stale on the next
Chrome release and would invite the same drift this change is fixing.

### Turn the backend's enumeration into its rule

The live text reads "any `*fiber.Error` (4xx), `pgx.ErrNoRows` mapped to 404, and
foreign-key-violation mapped to 404". That was already incomplete when written —
`search.ErrBadQuery`, the `context.Canceled` 499 and `fitanalysis.ErrNoAnalysis` all predate
this work and none is listed — which is the evidence that an enumeration cannot hold. The
replacement names the rule ("an expected state, not a fault"), names `classify()` as the one
place that decides, and gives instances as instances.

*Alternative considered:* keep the enumeration and extend it. Rejected: it would be correct
for exactly as long as nobody adds a case, and the drift is silent — nothing checks a
specification against the function it describes.

### Say why the filter must fail open

The requirement states that an unreadable failure is reported rather than dropped. This is
the property most easily lost in a later "simplification": a filter that swallows what it
cannot parse looks tidier and produces silence, which is the failure mode the whole
capability exists to prevent.

## Risks / Trade-offs

- **A prose rule is looser than a list** → The looseness is deliberate, and the binding
  version is the test: `sentryNoise.test.ts` names each case after the production issue it
  was added for, so a fragment removed from the filter fails against the event it exists
  for. The specification says what kind of thing belongs; the test says which ones do.
- **Naming `classify()` puts an implementation symbol in a specification** → Accepted. The
  alternative is re-deriving "where is this decided" at every reader, and the enumeration
  that avoided naming it is exactly what went stale.

## Open Questions

None. The behaviour is shipped, measured and tested; this change only records it.
