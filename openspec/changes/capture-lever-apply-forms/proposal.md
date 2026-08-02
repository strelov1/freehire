## Why

Lever was deferred by `collect-ats-apply-forms` on a measurement that was wrong.
That change recorded its apply page as **727 KB per posting** — "88% of the traffic
this whole effort would cost, for 16% of the coverage" — and concluded Lever needed
a third acquisition mode, fetching lazily when a candidate opens the job.

The 727 KB came from a `curl` that did not ask for compression. On the wire the page
is **97 KB**: Lever serves it gzipped, and Go's HTTP client requests and decodes that
transparently. Re-measured from production, the real cost is:

- backlog: 42,393 open postings × 97 KB ≈ **4 GB, once**
- ongoing: ~1,520 new postings a day × 97 KB ≈ **150 MB a day**

Against Greenhouse's 22 KB that is 4.4× per posting, not 33×. The queue captures a
posting once, not once per crawl, so there is nothing here that warrants a second
mechanism. Lever goes through the machinery the other four already use.

## What Changes

- `lever` becomes a capture provider: ingest queues its postings and
  `cmd/capture-apply-form` drains them.
- A fetcher over the apply page, and a parser for its markup. This is the first
  captured platform whose form is HTML rather than JSON, so the transport role the
  fetchers depend on gains an HTML method alongside the JSON ones.
- The EU host is honoured. Unlike Greenhouse — one host serving its EU boards —
  Lever splits, and the crawl adapter already picks between `jobs.lever.co` and
  `jobs.eu.lever.co` by the board entry's region.

What the markup requires, none of which the JSON platforms did:

- **A radio group is one question.** Several `<input type="radio">` share one `name`;
  each carries its own submit `value` and a sibling span with the text a candidate
  reads. Read control-by-control it would become one question per option.
- **Required is stated three ways** — a `required` attribute, a `required-field`
  class, and a `✱` glyph appended to the label. The glyph has to come off the label
  text whichever way the flag is read.
- **The submit name is the identifier**, exactly as it is for Greenhouse:
  `cards[<uuid>][field0]` for an employer's question, `name`/`email`/`urls[LinkedIn]`
  for the standard ones.

## Capabilities

### Modified Capabilities
- `apply-form-capture`: `lever` joins the providers whose form is fetched per
  posting, and a form may now be read from HTML rather than a JSON API.
- `apply-form-display`: the display projection learns which Lever controls are the
  standard application rather than the employer's questions.

## Impact

- `internal/applyform` — a fetcher, an HTML parser, one registry entry, and one
  method added to the transport role.
- No migration, no endpoint, no web change. The first ingest after deploy queues
  ~42k captures; at the worker's current budget that is roughly five hours of drain
  and 4 GB of transfer, once.
