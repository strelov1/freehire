## Why

Three call sites answer "what part of the candidate's CV may a model see?", and only one of them
is typed. `resumeextract.Professional` exists precisely to be that answer — a **whitelist**, so a
field later added to `Structured` is withheld until somebody adds it to `Professional` too. Its
doc comment says so, and says why a blacklist is the wrong way round.

The ATS review does exactly what that comment argues against. `internal/handler/ats_report.go`
marshals the **full** `Structured` — contacts included — into a string, and `internal/atscheck`
then deletes four known keys from the JSON map. It happens to leak nothing today: the complement
of `Professional` is exactly `full_name`, `email`, `phone`, `links`, so the two mechanisms agree
byte for byte. The defect fires on the first field added to `Structured` — an address, a github,
a date of birth — which ships to the gateway through the ATS review while the fit chain, reading
the same structure through the projection, correctly withholds it.

The fit chain gets the answer right but states it in a type that cannot hold it:
`matchanalysis.Input.StructuredResume` is a `string`, so the guarantee lives in the producer's
discipline. Its consumer defends by unmarshalling the string back into `Structured` and
re-projecting — a full JSON round trip whose second projection is provably a no-op, because the
producer already marshalled a `Professional`.

## What Changes

- `resumeextract.Professional` becomes the seam's type, not a convention about a string.
  `handler.structuredResumeJSON` is replaced by a reader returning
  `(resumeextract.Professional, bool)`; `PostATSReport` skips the model call when `!ok` instead
  of the analyzer inferring "no résumé" from an empty string.
- `atscheck.Analyze` takes `resumeextract.Professional` and marshals + truncates in
  `reviewUserPrompt`. **`stripContacts` is deleted** — the blacklist goes with it.
- `matchanalysis.Input.StructuredResume` becomes `resumeextract.Professional`, and
  `candidateContext` collapses to marshal + `TruncateRunes`: the unmarshal-and-re-project round
  trip is gone because the type already carries the guarantee.
- Found while implementing: `buildHardConstraintInputs` is a **fourth** model-bound CV path the
  finding does not name — the blockers it produces carry their reasons into the fit chain's
  stage-1 prompt. It took the full `Structured`. Safe by content (`hardconstraint.CVEvidence` is
  itself a six-field whitelist, all of them in `Professional`) but not by type, so it takes the
  projection too. One call site, two lines.
- No behaviour change today. The bytes reaching every model are identical before and after;
  what changes is that a new `Structured` field can no longer reach any of them by default.

## Capabilities

### New Capabilities

None. This types an existing seam.

### Modified Capabilities

- `resume-structured-profile`: gains the rule that the contact-free projection is the one seam
  every model-facing consumer takes, as a typed value rather than a JSON string it re-filters,
  and that the field set is a whitelist.
- `cv-ats-score`: the qualitative review reads the projection; it no longer removes contact
  fields itself.
- `job-fit-analysis`: the fit input is the projection, stated by name rather than by enumerating
  the four contact keys — the enumeration is the blacklist restated in prose.

## Impact

- `internal/atscheck/analyzer.go` — `Analyze` signature; `stripContacts` and its tests deleted.
- `internal/matchanalysis/analyzer.go` — `Input.StructuredResume` type; `candidateContext`.
- `internal/handler/ats_report.go`, `match_analysis.go`, `match_analysis_stream.go` — the three
  producers; `hardconstraint_inputs.go` — the fourth.
- No migration, no wire-format change, no new dependency. `internal/atscheck` gains an import of
  `internal/resumeextract`, which `internal/matchanalysis` already has.
