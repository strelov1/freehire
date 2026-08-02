# Show the questions a job's application form asks

## Why

`apply_forms` already holds the application form for every posting on Recruitee,
Greenhouse and Ashby — the questions, their types, whether an answer is required.
Nothing reads it. A candidate still cannot tell a one-click apply from an evening
of essay writing until they have committed to the form.

This shows the questions on the job page. It is the smallest possible reader over
a store that is already full.

## What it does

A block on the job detail page listing the questions the employer will ask:

```
What this application asks                        Greenhouse

  Name, email, phone, CV
  LinkedIn profile                                optional
  Which country do you currently reside in?       choose one
  Will you require visa sponsorship?              choose one
  What made you apply for this role?              written answer
```

Nothing is computed. The questions are shown as the platform published them, with
the control kind rendered as a word — that word is what separates a question worth
a minute from one worth twenty.

The interface is English (`<html lang="en">`), so the copy is English; only the
question text itself carries whatever language the employer wrote it in.

The control kinds map to display words as follows. A kind the capture could not
normalize (`Field.Type` empty, its `RawType` kept) gets no word rather than a
guessed one — the question is still shown, just without a hint about its cost.

| `applyform.FieldType` | word |
|---|---|
| `text`, `number`, `date` | — (nothing; a one-line answer is the default expectation) |
| `textarea` | written answer |
| `select` | choose one |
| `multiselect` | choose any |
| `boolean` | yes / no |
| `file` | upload |
| `hidden`, `info` | not shown at all — neither is a question |

## Decisions

**Verbatim question text is shown.** An earlier decision on the capture change was
to store the text but display only a derived summary, because the questions are the
employer's own writing and showing them is republication. That was reversed here
deliberately: the summary is a worse answer to "what will they ask me" than the
questions themselves.

**Standard fields collapse into one line.** Name, email, phone and CV appear on
every form; one bullet each would pad the list with what everyone already expects.
They are listed together, once, so their absence is still visible.

**The EEO survey is not shown.** It is not the employer's questions — it is the
mandated diversity survey the platform serves in a separate block, always optional
and near-identical everywhere. Listing it would bury the real questions in noise.
It stays in the store; the capture already marks it (`Field.Demographic`).

**No derived facts of any kind.** No question count, no time estimate, no detection
of whether salary or visa is asked. Counting is exact; reading meaning out of
employer prose is guessing, and a wrong guess sitting beside the apply button would
discredit the accurate parts of the block along with itself.

**No provider fallback.** A posting whose platform we cannot read shows nothing.
Saying something general about the platform was considered and dropped as
unnecessary for a first reader.

## Shape

- `GET /jobs/:slug/apply-form` — returns the stored form for one posting. 404 when
  there is none, which is the common case today and always will be for the
  providers whose forms cannot be read.
- The SSR loader fetches it as a fourth parallel request beside `similar` and
  `copies`, degrading to nothing on any failure — the same pattern those two
  already use, for the same reason: a discovery aid must not break the page.
- A Svelte component renders the list.

`internal/jobview` is deliberately untouched. That projection is also what goes
into the Meilisearch documents, so a field added there would inflate every indexed
job for the sake of one page — and keeping it out leaves a future filter as its own
clean piece of work.

## Coverage, and why it is fine

The three readable providers are 8.8% of the open catalogue and 15.8% of technical
postings. Workday (553k), Oracle (209k), SmartRecruiters (194k) and UKG (157k) are
walls or unexamined. So the block is absent from most job pages, and that is the
honest state of the world rather than a defect to design around: where it appears,
it is exact.

Capture is still draining at the time of writing (Greenhouse 11.5k of 134k), so in
development most Greenhouse postings will have no form. That is the ordinary
absent case, not a broken one.

## Testing

- The handler: a posting with a form, a posting without one (404), and a form whose
  only fields are the standard ones.
- The projection: standard fields collapsed, demographic fields excluded, control
  kinds mapped to their display words.
- The component: renders a list, renders nothing when the fetch returned nothing.

## Out of scope

A filter on how much applying costs. It needs a filterable attribute in the search
index and a full reindex to register it, which doubles the work and cannot be
justified before anyone has shown they read this block at all.
