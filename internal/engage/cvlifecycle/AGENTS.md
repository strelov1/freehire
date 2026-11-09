# internal/engage/cvlifecycle

Up to three letters to a verified FREE user whose CV parsed, until they pay or opt out.
`cmd/cv-lifecycle-mail` is the only caller. The block rules are in
[internal/engage/AGENTS.md](../AGENTS.md); the shared one-letter-a-day claim is
`internal/engage/maildaily`.

## The letters

| Step | Day after the upload | Says |
|---|---|---|
| 1 | 1 | the ATS score and the top three gaps |
| 2 | 6 | the score the CV could reach after tailoring |
| 3 | 13 | the offer: a discount on the first month of Pro |

The score is `atscheck.Score` with the inputs the site's ATS report uses (the stored CV text,
the skills found in it, the role's top skills from the search index). No model is called, so
the number in the mail is the number behind the link. Letter 1 needs a gap or a keyword to
name; letter 2 needs potential above the current score. Otherwise the step is skipped.

Letter 3 may be an account's FIRST letter: an account enrolled late (the window is 14 days)
whose upload is already 13 days old has steps 1 and 2 dropped as superseded. That is the
spec, not a bug; do not "fix" it by sending the earlier steps late.

## Enrollment and the holdout

One row per account in `cv_lifecycle_enrollments`, ever. `uploaded_at` is the clock anchor and
is never moved by a re-upload, so the offsets cannot restart. An account is enrolled only if it
passes `mail_recipient`, its CV is parsed (`cv_parsed(...)`: extract status `ok` AND the
structured stamp equals the current upload), it is on the free tier and it uploaded inside
`EnrollWindowDays`. Consent is NOT checked at enrollment: an account that had already opted out
is enrolled and then marked `unsubscribed`, so the conversion read still has it.

`holdout` is a stable tenth of accounts, a hash of the user id with a fixed salt
(`cv_lifecycle_holdout`, migration 0248; the salt and the bit mask are part of the definition,
changing either reshuffles the groups). Holdout rows are enrolled and observed but never mailed.

## Claim, send, exit

For each due step the runner does, in this order:

1. compose the letter (nothing written);
2. `ClaimCVLifecycleSend` writes the step row in ONE statement that re-checks the whole
   eligibility predicate (the list may be stale by now);
3. the daily claim (`maildaily`) for the UTC day;
4. SES.

Outcomes: a definite send failure BURNS the step (row kept, with `error`) and releases the day;
a crash keeps both claims, which fails toward fewer letters; a skip, a lost day or a paused
SES gives the claim back and leaves the step pending; a step whose successor is already due is
dropped, so a missed day never becomes a burst. Re-arming a burned step is a deliberate
`DELETE FROM cv_lifecycle_sends`. Only an unsubscribe (`news_email_enabled = false`, written by
the one-click link) or a live paid tier ends the sequence; a lapsed grant does not.

`ListCVLifecycleDue` is keyset-paged by user id and `MaxPerRun` counts only letters
attempted, so accounts that are permanently skipped cannot fill the head of every page.

## What "started paying" means

`paid_at` is stamped, and status becomes `paid`, when an enrolled account's Stripe or RevenueCat
Pro or Ultra tier (`*_until_stripe`, `*_until_revenuecat`) is live. The observation runs on EVERY
pass, for `ObserveWindowDays` (90) after enrollment, and includes holdouts and unsubscribed
accounts, since the comparison is the point.

A tier that is only GRANTED (`*_until_granted`: by hand, or an invite reward) is not a payment:
status becomes `entitled`, `paid_at` stays NULL and it is not a conversion. `entitled` is not
final: the account is still mailed while a grant lapses, and moves to `paid` if it later pays.
Conversion rate for a group: `count(paid_at) / count(*)` by `holdout`, over rows enrolled at
least `ObserveWindowDays` ago.

## The offer

The code comes from `CV_LETTER_PROMO_CODE` and lives in memory only: it is not stored, logged
or put in an error. `promo.Preview` runs at send time and its percent is what the copy says.
Letter 3 is skipped (stays pending) when the code is unset or refused, already redeemed by the
account, or the account holds a pending invite discount (`Discount().Percent > 0`: checkout
admits one coupon). The offer is described as the first month, never a recurring price.

## Privacy

A `Letter` holds scores, fixed check wording and dictionary skill names, never CV text, a name,
an address or a contact detail. Rendering uses `html/template` data fields; the tracking tags are
`Campaign = cv-letter-N`, `Group = GroupNews` and a one-click `UnsubscribeURL`.

## Reports

More than one optional letter to an account in a UTC day, across the four ledgers (should
return no rows; a row is a sender that bypassed `maildaily`):

```sql
SELECT user_id, day, count(*)
FROM (
    SELECT user_id, (sent_at AT TIME ZONE 'UTC')::date AS day FROM onboarding_emails WHERE error = ''
    UNION ALL
    SELECT user_id, (sent_at AT TIME ZONE 'UTC')::date FROM broadcast_emails WHERE error = ''
    UNION ALL
    SELECT user_id, (sent_at AT TIME ZONE 'UTC')::date FROM cv_lifecycle_sends WHERE error = ''
    UNION ALL
    SELECT id, (limit_nudge_sent_at AT TIME ZONE 'UTC')::date FROM users WHERE limit_nudge_sent_at IS NOT NULL
) s
GROUP BY user_id, day
HAVING count(*) > 1;
```

Do the letters cause payments? `go run ./cmd/report-cv-letters` (hand-run, read-only, needs only
`DATABASE_URL`; `report.go` here). Per letter it prints sent, delivered, clicked, bounced and
complained (SES, from `email_events` plus `email_event_daily`) and how many recipients started
paying within `PaidWindowDays` (14) of the letter, next to the holdout, then the whole-sequence
rates from enrollment. Reading it:

- **The control is on the letter's clock.** A letter's own control is the holdout accounts still
  unpaid when it fell due (`uploaded_at` + the step's `AfterDays`), counted over the 14 days after.
  Letter 3 goes to people who did not pay in two weeks, so a rate counted from enrollment would
  be a different stretch of an account's life. The enrollment rows (holdout against every
  non-holdout account, letter or not) are the comparison that cannot be flattered.
- **Only closed windows are read.** An account whose 14 days are still open is not a non-payer;
  `sent` counts every letter, the paid rate only matured ones. Run it 14+ days after the last
  letter of the cohort you care about, and it needs a few hundred accounts per group to say
  anything.
- **Letters overlap.** Letters are 5 and 7 days apart, so one payer can sit inside two letters'
  windows; the per-letter paid counts do not add up.
- **No opens.** The report prints n/a: the configuration set publishes no Open event (see
  [notifications](../../../docs/agents/notifications.md#engagement-tracking)); reopening that is the
  owner's decision, not a column to fill.
- **Mobile buyers are missing** until app-store purchases reach the backend; `paid_at` is a Stripe or
  RevenueCat tier first seen live by the hourly observation.
- `email_events` has no user id, so SES figures cannot be cut by holdout or by payer; they say how a
  letter fared, and the ledger says who paid.

Conversion by group:

```sql
SELECT holdout, count(*) AS enrolled, count(paid_at) AS paid
FROM cv_lifecycle_enrollments
WHERE enrolled_at < now() - interval '90 days'
GROUP BY holdout;
```

## Tests

Unit tests use fakes (`runner_test.go`, `content_test.go`, `mailer_test.go`). The SQL is
covered by `store_integration_test.go` (tag `integration`).
