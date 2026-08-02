## Context

The spike recorded Workable as unresolved rather than unreadable: `apply.workable.com`
answered Cloudflare 1015 to every probe from one machine while production ingest kept
crawling it. Re-probed from the production host there is no block, and the form
endpoint answers 200.

Everything downstream already exists. This is a fetcher, a mapper, and one registry
entry.

## Goals / Non-Goals

**Goals:** capture Workable forms through the machinery already built; get the three
shapes that differ from the other platforms right the first time.

**Non-Goals:** any change to the queue, the worker, the endpoint or the page.

## Decisions

### The vocabulary is measured, not assumed

Across 40 live technical postings, counting nested fields: `text` 310, `paragraph`
126, `date` 86, `boolean` 81, `multiple` 65, `file` 63, `group` 40, `email` 37,
`phone` 34, `dropdown` 26, `number` 15.

`multiple` did not appear at all in the first ten postings sampled. Had the mapper
been written from that sample it would have silently dropped every multi-choice
question — which is the shape of the most substantial questions employers ask.

`email` and `phone` map to plain text: the normalized vocabulary describes the KIND of
control, and both are text boxes. The validation hint survives in `RawType`, exactly as
Ashby's `Email` already does.

### The option pair is inverted, and that is the trap

```json
{"name": "6166574", "value": "I actively attend AI industry events…"}
```

`name` is the identifier, `value` is the text a candidate reads. Greenhouse, Recruitee
and Ashby all mean the opposite by those words. Written by analogy, the mapper would
label every choice with a number and store the sentence as the submit token — wrong in
both directions at once, and invisible until someone read a captured form.

### A group is one control, not five

Education and Experience arrive as repeatable groups with nested fields. The nested
fields are the parts of one entry — school, degree, dates — and listing them
individually would say "this application asks for your start date" where the true
statement is "this application asks for your education history".

So the group is captured as one control and its children are not walked. This is also
why the type dictionary has no entry for `group`: it is not a kind of answer.

### `QA_` is the question discriminator

Every employer-authored question observed carries an identifier prefixed `QA_`;
everything else is Workable's standard profile. This is the platform's own convention,
the same kind of marker Ashby's `_systemfield_` prefix provides, and it is preferable
to an id list because Workable's standard profile is long and would have to be tracked
as it grows.

*Alternative rejected:* enumerate the standard ids. It works today and rots quietly —
a profile field Workable adds later would start appearing among the employer's
questions, and nothing would fail.

## Risks / Trade-offs

- **`QA_` is a convention, not a contract** → If Workable changed it, standard profile
  fields would surface as questions on the job page. Visible and reversible rather than
  silent; and the alternative rots in the same way for the opposite reason.
- **The first deploy queues ~16k captures** → Absorbed by the existing hourly drain at
  8000 per run; nothing new to schedule.
- **The rate limiting that hid this platform is real** → It came from one machine, not
  from production, but the capture worker is a second caller against the same host as
  the crawl. Its concurrency bound (2) is what keeps that from becoming a problem, and
  the queue's retry covers a transient 429 without losing the posting.

## Migration Plan

None. Purely additive to a registry; the first ingest after deploy starts queueing.
