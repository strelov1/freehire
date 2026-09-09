## 1. Cached tokens become readable

- [x] 1.1 Add `CachedInput` to `llm.Usage` (`internal/platform/llm/langfuse.go`) and read
      `GenerationInfo["PromptCachedTokens"]` in `UsageFrom` (`internal/platform/llm/llm.go`),
      using the existing `intFrom` coercion. Unit test: a choice carrying the key reports
      it; a choice carrying only the three existing keys still reports those and a zero
      cached count; a choice carrying no counts at all still returns nil.
- [x] 1.2 Carry `CachedInput` into the Langfuse usage body (`newGenerationBody`,
      `internal/platform/llm/langfuse.go`) and its wire type. Unit test: the encoded batch
      carries the cached count when set.
- [x] 1.3 Update `internal/platform/llm/AGENTS.md` to say what the cached count is and
      what it cannot distinguish.

## 2. The report's pure core

- [x] 2.1 Introduce the report row type and the run tally it is computed from (rounds,
      decode failures, token counts, first-token latency, terminal stop reason). Unit
      test the tally over a scripted sequence of events.
- [x] 2.2 Implement the cache verdict: a run whose rounds all report zero cached tokens is
      labelled "no cache observed"; a run with any non-zero count reports the total and
      its share of input tokens. Unit test both, plus the boundary of a single non-zero
      round.
- [x] 2.3 Implement cost from the price table: cached input priced at the cache-read rate
      where the table names one, and a model absent from the table reported with unknown
      cost rather than zero. Unit test each branch.
- [x] 2.4 Implement the ranking: order rows on `cvmatch` overall, with the ATS delta as
      the secondary key, and carry a failed run to the bottom rather than treating its
      absent score as a low one. Unit test the ordering and the failed-run placement.

## 3. The price table

- [x] 3.1 Add the generator that reads the gateway's model catalogue and writes the price
      table with its capture date. Unit test the parse against a recorded catalogue
      fixture, including a model with no cache-read price.
- [x] 3.2 Commit the generated table and document how to refresh it.

## 4. Fixtures

- [x] 4.1 Add the case fixture shape (a CV, a vacancy, the expected binding) and its
      loader, with the loader failing loudly on an unreadable or empty case set. Unit test
      the failure path.
- [x] 4.2 Export the profile fixture (CV, structured résumé, experience atoms) from
      production into `testdata/` once, and record in the change how it was captured.
- [x] 4.3 Add the seed helper that loads a case fixture into a fresh database. Verify by
      seeding and reading back through the same queries the tools use.

## 5. The bake-off

- [x] 5.1 Expose the seam the bake-off needs from `newAutopilotHarness` without changing
      what existing callers pass. Verify the existing autopilot integration tests still
      pass untouched.
- [x] 5.2 Write the `//go:build llmlive` bake-off: for each candidate model, a fresh
      database, the seeded fixtures, one autopilot run per case, the tally collected from
      the stream. Only the turn model varies; assert in the test that the fit model does
      not.
- [x] 5.3 Score each completed run with `cvmatch.Compute` and `atscheck.Compare` against
      the base report, and attach both to the row.
- [x] 5.4 Record a failed, cancelled or step-capped run as a row carrying that outcome and
      continue; end the bake-off only when the case set itself cannot be read.
- [x] 5.5 Emit the report: a table to the test log and the full rows as JSON under
      `.cache/`, naming the price table's capture date and the profile the cases were run
      against.
- [x] 5.6 Include each completed run's tailored CV text in the JSON report beside its
      vacancy and scores, and omit it for a failed run. Verify by reading a report back
      and finding one CV per completed (model, case).

## 6. Wrap-up

- [x] 6.1 Document the bake-off in `internal/ai/assistant/AGENTS.md`: what it measures,
      how to run it, and why rounds and cache rate matter more than the price page.
- [x] 6.2 Run the bake-off against at least two candidate models over several vacancies,
      record the first report, and read the tailored CVs it carries.
