## Why

The capture spike left Workable unresolved rather than ruled out: every request to
`apply.workable.com` from the probing machine came back Cloudflare 1015, while
production ingest kept crawling the platform happily. That was absence of evidence,
and the change that shipped said so.

Re-probed from the production host, there is no block at all:
`GET apply.workable.com/api/v1/jobs/{shortcode}/form` answers 200 with a sectioned
form carrying each control's id, label, type and required flag — the same substance
the three captured platforms provide.

It is also the cheapest coverage left. The queue, the worker, the enqueue gate, the
display projection and the job-page block all exist; Workable needs a fetcher and a
mapper and nothing else. 16k postings.

## What Changes

- `workable` becomes a capture provider: the ingest write path queues its postings
  and `cmd/capture-apply-form` drains them, exactly as it does Greenhouse and Ashby.
- A fetcher over `apply.workable.com/api/v1/jobs/{shortcode}/form`. The shortcode is
  the second half of the stored `external_id`, so nothing new has to be looked up.
- A mapper for Workable's payload, whose vocabulary was measured across 40 live
  technical postings rather than assumed: `text`, `paragraph`, `date`, `boolean`,
  `multiple`, `file`, `group`, `email`, `phone`, `dropdown`, `number`.

Three shapes in that payload need naming, because each would be got wrong by
analogy with the platforms already mapped:

- **The option pair is inverted.** Workable sends `{"name": "6166574", "value": "I
  actively attend AI industry events…"}` — `name` is the identifier and `value` is
  the human text. Every other platform means the opposite by those words, so a
  mapping written from habit would label every choice with a number.
- **`group` is not a control.** It is a repeatable compound — Education, Experience —
  carrying its own nested fields. Flattening it would list "School, Field of study,
  Degree, Start date, End date…" where the honest statement is "this application
  asks for your education history".
- **An employer's question is identified by a `QA_` prefix**, the platform's own
  convention across every posting sampled. The same kind of discriminator Ashby's
  `_systemfield_` prefix already provides, and more robust than an id list that
  would have to grow with Workable's standard profile fields.

## Capabilities

### Modified Capabilities
- `apply-form-capture`: `workable` joins the providers whose form is fetched per
  posting. The capability's existing requirements — the queue, the gate, the
  worker's isolation and retry — are unchanged and already cover it.
- `apply-form-display`: the display projection learns which Workable controls are
  the candidate's standard profile rather than the employer's questions.

## Impact

- `internal/applyform` — one fetcher, one mapper, one entry in the provider
  registry that the enqueue gate and the worker both read.
- No migration, no new endpoint, no web change, no worker change. The first ingest
  after deploy queues ~16k captures, which the existing hourly drain absorbs.
