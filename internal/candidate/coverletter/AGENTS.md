# Cover letter drafting

## Scope

`internal/candidate/coverletter` writes the cover letter a vacancy's application form asks for,
from the achievement atoms the candidate has actually asserted. It owns the three-stage chain,
the wire shape and its sanitize pass, the evidence gathering, and the single stored draft per
(candidate, vacancy).

Of the 402,117 open postings whose apply form we have captured, 209,297 ask for a letter and
172,783 of those accept it as text. The remaining 36,514 (recruitee, ashby) take a file upload
only — the letter is still useful there, but by hand.

## Always true

- **A fixed chain, NOT an autonomous agent** — the same first line
  [`matchanalysis`](../matchanalysis/AGENTS.md) opens with, and for the same reason. The agent
  has no context the chain lacks: `fitanalysis.TailoringContext` is one projection serving the
  HTTP reader and the agent tool alike, so an agent would spend turns deciding to fetch what a
  chain is handed, at roughly 7 model calls to this chain's 3.
- **The provenance gate runs on the chain's INPUT, inside `Draft`.** `Publishable` delegates to
  `experience.Provenance.Publishable` rather than restating the three admissible labels — a
  second copy of that list is a second answer, and the drift between them would look exactly
  like a working gate. Checking the finished prose instead would mean matching sentences back
  to atoms, the fuzzy problem the gate exists to avoid; a model that never sees an inferred
  atom cannot cite one. A caller cannot skip it, which is why it is not a parameter.
- **The letter is written in the language of the VACANCY.** This inverts `matchanalysis`, which
  writes in the candidate's profile language, and the inversion is the point: an analysis is
  the candidate reading themselves, a letter is read by the employer. `LanguageOf` falls back
  to English, never to the profile — about 5% of open postings carry no detected language, and
  a Russian letter to a German employer reads as a mistake where English reads as a guess.
- **Stage 3 is a skeptic and it is what enforces brevity.** A "be concise" instruction inside
  the drafting prompt is not the mechanism. It is handed BOTH the letter and the achievements:
  sending the letter alone would ask it to verify support against a list it was never given,
  and it would still shorten the letter while passing every invented claim — which is exactly
  the bug the first review caught.
- **The audit may improve the letter, never destroy it.** An unparseable answer and one cut
  below `Bounds.Floor` are the same failure in different clothes, and both keep the un-audited
  draft. A stage whose only instruction is to cut can cut to nothing.
- **Citations follow the AUDITED letter.** An atom whose sentence the audit removed is no
  longer evidence for anything the letter says; displaying it would claim support the letter
  lost — the mirror of the invented-id case `Sanitize` guards. An audit that names none has not
  said the letter cites nothing, so stage 1's selection stands. When stage 1 named nothing
  either and the chain fell back to everything offered, the list is **empty**: listing five of
  thirty atoms nobody chose would assert support nobody gave.
- **`Sanitize` filters citations against the set actually offered.** The model is asked which
  atoms it used; an id it invents would render beside the letter as evidence the letter does
  not have, and that display is the whole claim this feature makes.
- **Only `resumeextract.Professional` reaches the model.** Raw CV text never does. As in the
  fit chain, de-identification is the argument's TYPE, not something this package performs.
- **`Bounds` defaults field by field (`OrDefault`), never by a zero-value check.** A caller
  that sets only `MaxCited` leaves the ceilings at 0, and a ceiling of 0 clips every body to
  `""` — an empty letter, persisted, with nothing failing. `MaxCited: 0` is worse: `Sanitize`
  stops at `len(kept) == MaxCited`, never true after the first append, so the cap does not
  default — it disappears.

## How it works

```
bank ──► Gather (missing-have only) ──► Publishable ──► offered
                                                          │
   stage 1 select ──► stage 2 draft ──► stage 3 audit ──► Sanitize ──► Store
                                          ▲                 ▲
                            letter + achievements      offered set
```

`Gather` retrieves per requirement because that is the grain the bank scores at, keeps an
atom's best score across the requirements it answered, and reads the whole bank when nothing
retrieves — evidence that does not line up requirement-by-requirement is a reason to write
from the bank, not to refuse. It filters to `missing-have`: a `missing-gap` is filtered out
rather than deprioritised, because adjacent evidence in front of a drafting stage is an
invitation to stretch it into a claim.

Stage 2 decodes into a type naming only `body`. Unmarshalling into `Letter` would let junk
under `cited_atom_ids` — a key that prompt never mentions — fail the whole draft, and the
caller overwrites that field regardless.

## Boundaries

| Data | Owner | Why |
|---|---|---|
| The vacancy, the verdict, the requirement split | [`fitanalysis`](../fitanalysis/AGENTS.md) | one projection, three readers |
| Achievement atoms and their provenance | [`experience`](../experience/AGENTS.md) | the bank accumulates; this only reads |
| The de-identified candidate | `resumeextract` | a type, not a convention |
| The plan allowance a draft consumes | `internal/ai/plan` | metered on the write path only |

The vacancy arrives as a `db.Job` parameter from a caller that has already loaded and
authorized it: `candidate` is layer 4 and may never import `job` at layer 5.

**A cached fit analysis is a precondition, not something drafting produces.** `Required` reads
the cache and returns `ErrNoAnalysis` when it is empty; producing is `Ensure`'s, and `Ensure`
takes the coalescing `Request` the autopilot assembles. A letter has no business assembling that
and no business paying for an analysis the candidate did not ask for, so an absent analysis
renders as "run the fit analysis first" — a state they can fix one tab over. This shipped wrong
once: the endpoint flattened every chain failure into a 502, so the commonest state on
production arrived as a Bad Gateway.

## Limitations

- **The length bands are a decision, not a measurement.** No captured `apply_forms` field
  carries `maxlength` — the stored keys are `id`, `type`, `raw_type`, `label`, `required`,
  `section`. Widening that capture belongs to `applyform`.
- **No file export.** The 17% of postings that take a file only are served by copying the text.
- **No revisions.** A letter is a paragraph regenerated in seconds; `cvedit`'s machinery earns
  its place on a document edited over months.
- **`Gather` duplicates the per-requirement retrieval loop in
  `internal/api/handler/assistant_cv_tools.go`.** `handler` sits above `candidate` and could
  call this instead; folding them together is a separate change.
