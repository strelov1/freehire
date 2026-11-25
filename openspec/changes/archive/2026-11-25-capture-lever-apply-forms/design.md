## Context

`collect-ats-apply-forms` deferred Lever on a wrong number. It recorded the apply page
as 727 KB and reasoned that capturing it would be "88% of the traffic for 16% of the
coverage" — a cost that demanded a third acquisition mode.

That 727 KB came from a `curl` that did not ask for compression. Lever serves the page
gzipped and Go's HTTP client requests and decodes it transparently, so the figure the
worker would actually pay is **97 KB**. Re-measured from production: 42,393 open
postings (≈4 GB once), ~1,520 new a day (≈150 MB a day). Greenhouse costs 22 KB per
posting, so Lever is 4.4× that rather than 33×.

Captures happen once per posting, not once per crawl, so nothing here justifies a
second mechanism.

## Goals / Non-Goals

**Goals:** capture Lever through the existing queue; parse its markup correctly the
first time, against the shapes the real pages use.

**Non-Goals:** a lazy or on-demand acquisition mode; any change to the queue, the
worker, the endpoint or the page.

## Decisions

### HTML joins the transport role rather than a new port

`applyform.Transport` is the narrow HTTP role the fetchers depend on, declared in this
package because `internal/sources` imports it and the dependency cannot run both ways.
It gains an HTML method beside the JSON ones — the real `sources.Client` already
implements one, so nothing new is built, and a fetcher still declares exactly what it
needs.

*Alternative rejected:* fetch the page as raw bytes and parse here. The client's HTML
method already returns a parsed tree with the size caps and timeouts the crawl relies
on; duplicating that to avoid one interface method is the wrong trade.

### The parser reads the question block, not the document

Lever renders each question as `li.application-question` containing a label and one or
more controls. The parser walks those blocks and ignores everything else on the page —
which is most of a 731 KB document, and all of it noise.

Within a block, controls are grouped by submit name. That grouping is the whole reason
the parser cannot work control-by-control: a radio group is several inputs sharing one
name, and read individually it would become one question per alternative.

### An option's two halves come from two places

For a radio group the submit value is the input's `value` attribute and the text a
candidate reads is the sibling `span.application-answer-alternative`. For a `select`
they are the option's `value` attribute and its text. Both are the same distinction the
JSON platforms draw — and the reason the capture stores both halves at all.

### Required is read from the attribute, and the glyph is stripped

Lever states the requirement three ways at once: a `required` attribute on the input, a
`required-field` class on the field wrapper, and a `✱` appended to the label. The
attribute is authoritative because it is what the platform's own validation uses; the
glyph is removed from the label because it is decoration, not part of the question.

### The submit name is the identifier, and its shape is the discriminator

`cards[<uuid>][field0]` is an employer's question; `name`, `email`, `phone`, `org`,
`urls[…]`, `resume`, `location` and the consent boxes are the standard application.
The `cards[` prefix is the marker, in the same spirit as Ashby's `_systemfield_` and
Workable's `QA_` — the platform's own convention rather than an inference from wording.

## Risks / Trade-offs

- **Markup is a less stable contract than JSON** → A Lever redesign would break the
  parser, and it would break loudly: captures fail, the queue records the reason, and
  the failure count is already the worker's alert. A form that parsed to nothing would
  be worse, so a block yielding no controls is not stored as an empty form.
- **42k pages is a lot of requests at one platform** → The worker's concurrency bound
  (2) is what keeps that civil, and the queue's retry absorbs a transient refusal. The
  backlog is a one-time ~4 GB; the ongoing cost is 150 MB a day.
- **The parse costs CPU that the JSON platforms did not** → 97 KB compressed is ~730 KB
  to walk. Bounded by the same concurrency, and the worker runs on a cadence rather
  than continuously.

## Migration Plan

None. Additive to a registry; the first ingest after deploy starts queueing.
