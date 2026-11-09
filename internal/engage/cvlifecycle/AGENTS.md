# internal/engage/cvlifecycle

Up to three letters to a verified FREE user whose CV parsed, until they pay or opt out, and
the paywall letter (below). `cmd/cv-lifecycle-mail` is the only caller. The block rules are in
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

## The paywall letter

A fourth letter (`StepPaywall`, step 4 of `cv_lifecycle_sends`, campaign `paywall-letter`)
answers a free user who pressed a paid AI button: about an hour after a `paywall_hits` row
(written by the paid-AI gate, see [internal/ai/plan/AGENTS.md](../../ai/plan/AGENTS.md)), it
says what they tried (the feature, and "for <job> at <company>" when the hit carries a job)
and that it is part of Pro. No offer, no model call: the list row carries all of it.

- **Once per account, ever** (the ledger's `(user_id, step)` key), however many hits and
  features: the letter names the earliest hit inside the window. Not once per hit or feature.
- **Due** `PaywallAfterMinutes` (60) after the hit and only for `PaywallWithinHours` (24): a
  hit older than that is no longer "what you just tried", so a letter held back by the daily
  claim for longer is dropped with it rather than sent late.
- **Every paid-AI label counts**, including the three unmetered ones (`ats_review`,
  `mail_recall`, `autofill`) and `ai_other`. `attempts` in `paywall.go` words each; a label
  without words gets a generic letter instead of none. A new paid route needs its words there.
- **Same gates as the CV letters**, repeated in `ListPaywallLetterDue` and re-checked in
  `ClaimPaywallLetterSend`: mailable address, news-email consent, free tier at send time (a
  granted tier counts as paid here), not in the holdout, the shared one-letter-a-day claim.
  It is NOT gated on enrollment or a CV: a user with no CV can hit a paywall. The holdout is
  the same `cv_lifecycle_holdout` hash enrollment stores, so an account is in one group for
  every letter.
- **It wins the day**: a pass sends the paywall letter before the CV letters (`passOrder`), so
  an account due for both gets the paywall letter and the CV letter waits for the next day.
  Other senders (onboarding, broadcast, limit nudge) run in their own commands and are not
  ordered against it; whoever claims the day first wins, as everywhere.
- It spends the same `MaxPerRun` budget, burns on a definite send failure like any step, and
  does not move the enrollment or the paying observation.

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

The paywall letter's own conversion is not in the enrollment table (the account may never
have enrolled). Read it from the ledger and the tier columns instead:

```sql
SELECT cv_lifecycle_holdout(h.user_id) AS holdout, count(DISTINCT h.user_id) AS hit,
       count(DISTINCT h.user_id) FILTER (WHERE s.user_id IS NOT NULL) AS mailed,
       count(DISTINCT h.user_id) FILTER (WHERE u.pro_until_stripe > h.created_at OR u.pro_until_revenuecat > h.created_at
                                           OR u.ultra_until_stripe > h.created_at OR u.ultra_until_revenuecat > h.created_at) AS paid
FROM paywall_hits h
JOIN users u ON u.id = h.user_id
LEFT JOIN cv_lifecycle_sends s ON s.user_id = h.user_id AND s.step = 4 AND s.error = ''
WHERE h.created_at < now() - interval '30 days'
GROUP BY 1;
```

Conversion by group:

```sql
SELECT holdout, count(*) AS enrolled, count(paid_at) AS paid
FROM cv_lifecycle_enrollments
WHERE enrolled_at < now() - interval '90 days'
GROUP BY holdout;
```

## Tests

Unit tests use fakes (`runner_test.go`, `content_test.go`, `mailer_test.go`,
`paywall_test.go`). The SQL is covered by `store_integration_test.go` and
`paywall_integration_test.go` (tag `integration`).
