# Salary extraction

Reads a compensation range out of a posting's description, with no model call.
`jobview.FromDomain` serves it only when the enrichment payload states neither bound.
A manual salary and an ATS structured salary (`salary_*_source`) win over it, in that
order — the same precedence `SetJobEnrichment` already applies once the model has run.

## Always true

- **A label, or nothing.** The line must start with `Salary:`, `Pay:`, `Compensation:`
  or `Remuneration:`; `Salary Range:` is also accepted. A standalone label may
  precede its value on the next non-empty line, which must start with an amount.
  A next line whose only amount is a benefit (a sign-on bonus) is not that
  value; a line that still states pay is.
  A number anywhere else in the posting is not a salary. SEEK and
  WhatJobs already put the source's own salary field on such a line; "Rates Negotiable"
  carries the label and no number, and yields nothing.
- **One or two amounts.** A third figure on the line (a level, a bonus percent) is
  ignored only when it is not itself a money token. Three money tokens is a refusal.
- **`from` is a floor, `up to` is a ceiling, a single figure is both bounds.** A
  trailing `+` on the only figure is a floor. `+ Bonus` and `+ 17%` are not amounts.
- **No period is allowed only when the figure is at least 1000.** `$140+` with no
  period is as likely an hourly rate as a yearly one, and inventing the period is the
  guess this package exists to avoid. `£15.59 per hr` is 16, period `hour`.
- **A bare `$` takes its currency from the posting's countries, and only when those
  dollar countries agree.** `us` is USD, `au` is AUD, and the same for `ca`, `nz`,
  `sg`, `hk`. `us` together with `au`, or a `$` in Chile, leaves the currency empty.
  An explicit marker (`£`, `€`, `Rp`, `A$`, `USD`) wins over the countries.
- **A second period next to a different amount refuses the line.** DataAnnotation's
  header says `$104,000 - 208,000 per year` and the body says the work is paid hourly
  up to `$100+`. Returning the header would publish the wrong period with confidence.
  The same amount repeated in another period is not a contradiction.
- **A scaled or half-read amount refuses the line.** `$120k` and `1,200k` would
  read as 120 and 1,200, and `$50 - 60` or `27.510,00 € - 51090 €` would drop the
  ungrouped second bound, so each is refused rather than published as a confident
  wrong figure. The `k`/`m` suffix counts only on a money token, including one
  with no currency marker. `401k` in a benefits clause is not one.
- **The currency is the marker on the amount, not the first word on the line.**
  `A$120,000 per year (USD equivalent)` is AUD. `$120,000 USD` is USD, and that
  word wins over the posting's country. Two different markers on one amount
  (`A$120,000 USD`), or two amounts in different currencies, refuse the line.
  `US$` is still not `S$`, and `CA$` is still not `A$`: the letters immediately
  before the symbol belong to the marker.
