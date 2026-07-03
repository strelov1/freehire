## ADDED Requirements

### Requirement: Summary keyword-density line item

The ATS score SHALL include a deterministic line item that rewards a keyword-dense
professional summary: it SHALL isolate the CV's summary section (by heading) and
award the item when the summary carries at least a configured number of the CV's
concrete skill tags, otherwise mark it recoverable with a fix to add role keywords
to the summary. The item SHALL slot into an existing category without changing the
five categories' total maximum of 100 (item weights re-balanced within the
category). It SHALL require no LLM.

#### Scenario: Keyword-dense summary is rewarded
- **WHEN** the CV's summary section mentions several of the CV's skills (e.g. "Go, Kafka, Kubernetes")
- **THEN** the summary keyword-density item passes and awards its points

#### Scenario: Empty or generic summary is flagged
- **WHEN** the CV's summary carries none of the CV's concrete skills
- **THEN** the item is recoverable with a fix to add role keywords to the summary

#### Scenario: Category maxima still sum to 100
- **WHEN** the report is scored
- **THEN** the five categories' maxima still sum to 100
