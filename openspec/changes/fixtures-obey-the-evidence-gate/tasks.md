## 1. Answer the product question before touching a fixture

- [x] 1.1 Read what production actually assembles — `Register` passes `bankGate{bank}`
      unconditionally, so the answer is already "yes" and aligning the fixtures changes no
      product behaviour.
- [x] 1.2 Record that the other reading (a per-actor policy) is a product change and stays a
      separate proposal.

## 2. Make the fixtures describe production

- [x] 2.1 The integration fixture builds the real bank the way `Register` does, from its own
      queries — an integration test with a live database has no reason to stub it.
- [x] 2.2 The uncited PATCH is 403; a cited one lands. Both, because a gate that only ever
      refuses would pass a one-sided test.
- [x] 2.3 The tool fixtures: an empty bank rather than a nil gate, and the two cases that expect
      a write to land bank an atom and cite it.
- [x] 2.4 Eight fixtures I1 does not name also had nil gates. Check whether they reach the agent
      path (they do not) before deciding, then give them the real gate anyway so the rule can be
      enforced.

## 3. Keep it

- [x] 3.1 A rule test: no file constructs an editor with a nil gate. Verify it fires by restoring
      one.
- [x] 3.2 `go test ./...` AND `go test -tags=integration ./...`.
- [x] 3.3 Mark I1 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
