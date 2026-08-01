## Context

`internal/cvedit` is the only writer of a stored CV, and its `requireEvidence` is the
service-path enforcement of the tailoring capability's central rule. The enforcement is
correct. The *wiring* of the collaborator it needs is not:

```
internal/handler/cv.go:92        cvedit.NewEditor(cvedit.NewRepository(pool, queries), nil)
internal/handler/assistant.go:96 if cvH != nil && cvH.editor != nil { cvH.editor.WithEvidenceGate(bankGate{...}) }
internal/handler/cv_tailor.go:241 actor = cvedit.ActorAgent   // for any API-key caller
```

The gate is created nil, attached later, by a different feature's constructor, behind two
nil guards. `internal/cvedit/editor.go` justifies the mutator with "the bank is wired
later than the CV handlers" — but `handler.go:266` already builds an equivalent
`experience.Store` twenty-three lines before `newCVHandlers` at `:289`, and the store is
stateless. The justification is simply stale.

There are two independent fail-opens on the same rule:

| Site | Guard | Reachable in production? |
|---|---|---|
| `internal/cvedit/policy.go:157` | `if e.gate == nil { return nil }` | No — production attaches a gate |
| `internal/handler/assistant_cv_tools.go:356` | `if g.bank == nil { return nil }` | No — `g.bank` is always `experience.NewStore` |

Neither fires today. Both exist so that a wiring mistake reads as a permission.

## Goals / Non-Goals

**Goals:**

- Make the gate a value the editor is constructed with, so an editor that admits an
  agent's uncited claim cannot be assembled.
- Remove the fail-open that has no legitimate use (`bankGate` with no bank).
- Delete the post-construction mutator, so `newAssistantHandlers` stops reaching into a
  struct it was handed.

**Non-Goals:**

- Making `NewEditor` reject a nil gate, or introducing a `cvedit.NoGate{}` type.
  `requireEvidence` returns early for `ActorCandidate`, so a nil gate is legitimate for
  candidate-only editors; eleven test editors construct one, and stubbing all of them buys
  no invariant.
- Changing *what* the rule is, which paths it gates, or its messages. This change moves
  where the dependency is supplied, nothing else.
- Reconciling the three test fixtures that exercise the agent write path with a nil gate
  (see the proposal). They bypass `newCVHandlers` with a struct literal, so this change
  neither fixes nor breaks them; the reconciliation is a product question of its own.

## Decisions

### D1: `newCVHandlers` takes a `cvedit.EvidenceGate`, not the bank

The narrower type states exactly what the CV handlers need — an answer to "may this be
published?" — rather than handing them a whole experience bank they would only use to
build one. The assembly in `handler.go` constructs `bankGate{bank: bank}`, which is where
a composition decision belongs.

*Alternative considered:* pass `experienceBankTools` and let `newCVHandlers` wrap it.
Rejected — it widens the CV handlers' dependency to six bank methods to use one, and
`bankGate` is already the adapter for exactly this.

### D2: The bank is hoisted once and threaded to its three consumers

`experience.NewStore(experience.NewQueriesRepository(queries))` is currently built twice
(`handler.go:266` for the experience handlers, `assistant.go:75` for the assistant) and
would be built a third time for the gate. The store is stateless, so the duplication is
harmless in behaviour — but "the assistant owns the bank" is precisely the belief that put
the gate's wiring inside `newAssistantHandlers`. Hoisting it to the assembly removes the
belief along with the mutator.

### D3: `requireEvidence`'s nil-gate short-circuit stays; `bankGate`'s does not

They look like the same guard and are not. A nil *gate* means "this editor was built for
candidate-authored editing", which is a real configuration with eleven users in the test
suite. A `bankGate` with a nil *bank* means "an adapter was built around nothing" — there
is no configuration that wants it, and the field is never nil in production.

Deleting the first one would also be actively worse than it looks: with `if e.gate == nil`
removed, the very next statement to run for an agent write is `e.gate.Publishable(...)`,
so a candidate-only editor handed an agent change would panic rather than refuse. Fixing
that properly means fail-closed, which means the three agent-path fixtures must first be
given real gates — out of scope here by decision.

### D4: The mutator is deleted, not deprecated

`WithEvidenceGate` has exactly two callers: the production wiring being replaced, and one
test harness that re-does that wiring by hand. Both move to the constructor in the same
diff, so there is nothing to deprecate for.

## Risks / Trade-offs

**`newCVHandlers` gains an eleventh parameter.** → It gains the one dependency that is
security-relevant, and loses a mutator that made the dependency invisible. The alternative
— a config struct for the CV handlers — is a larger refactor than this finding warrants
and would obscure the very argument being made explicit.

**A future assembly could still pass a nil gate to `newCVHandlers`.** → True, and
deliberately not defended against: `NewEditor` keeps accepting nil for candidate-only
editors, so nil remains expressible. What changes is that the value must now be supplied
at the one call site that builds the production editor, where its absence is visible in
the same expression, rather than depending on a later mutator in another feature's
constructor.

**Collapsing three `experience.NewStore` constructions could couple otherwise independent
handlers.** → The store is stateless and holds only a repository over the shared
`*db.Queries`; sharing one value is equivalent to sharing three identical ones.

## Migration Plan

None. No schema, no API, no wire shape, no behaviour change in production — the gate is
attached today and stays attached, from a different line. Rollback is a plain revert.

## Open Questions

Must an API-key caller editing `PATCH /me/cvs/:id` as `ActorAgent` cite banked evidence?
Production says yes (the gate is attached). `cv_tailor_integration_test.go:276` records a
comment saying the CLI is "not a place to enforce provenance it cannot query", and its
fixture wires no gate. The two disagree. This change does not resolve it — it makes
production's answer unambiguous and leaves the fixture untouched — and the disagreement is
recorded as its own finding in the architecture review.
