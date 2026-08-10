## 1. Client

- [x] 1.1 Add `resolveJob(url, token)` to `lib/freehire.ts` over
  `POST /api/v1/jobs/resolve`, returning the `{ public_slug, status }` envelope.
- [x] 1.2 Add the pure `resolveNotice(status)` mapping the three outcomes to the sentence
  the panel shows; unit-test it (`lib/freehire.test.ts`), including an unknown status.

## 2. Panel

- [x] 2.1 Show "Add to freehire" when signed in and the current page resolved to no
  catalog job (text-matched card or empty state), hidden while a curated card is showing.
- [x] 2.2 On success with a slug, reload the match so the curated card replaces the ad-hoc
  one; on `queued`, notice only; on failure, notice and leave the card intact. Disable the
  button while in flight.

## 3. Verification

- [x] 3.1 `npm test` (66 tests) and `npm run check` (0 errors) green.
- [ ] 3.2 Load the built extension, open a vacancy freehire does not carry, press the
  button, and confirm the reported outcome matches what the catalog did. **Needs the
  server side deployed — not doable until `/api/v1/jobs/resolve` is live.**
