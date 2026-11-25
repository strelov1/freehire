## ADDED Requirements

### Requirement: Bank edits offer a CV refresh with consent

After a successful experience-bank mutation in the web UI (create, update, or delete of an
employment or evidence atom, including merges), the product SHALL prompt the candidate to
refresh CV content from the current seed. The prompt SHALL make clear that agreeing
rebuilds CV body content from the experience bank and current résumé seed (same meaning as
Reset from résumé). Declining or dismissing the prompt MUST leave every CV document
unchanged. The system MUST NOT rewrite a CV solely because the bank changed.

#### Scenario: Tailor workspace — agree refreshes the open tailored CV

- **WHEN** the candidate successfully adds or updates banked experience while the tailor
  workspace is open on a tailored CV, and they confirm the refresh prompt
- **THEN** the open tailored CV is reset from the current seed (existing reset-from-résumé
  behaviour) and the workspace shows the updated document

#### Scenario: Tailor workspace — decline leaves the CV alone

- **WHEN** the candidate successfully edits the bank on the tailor workspace and declines
  the refresh prompt
- **THEN** the open tailored CV is unchanged

#### Scenario: Profile experience — agree reseeds the base CV

- **WHEN** the candidate successfully edits the bank on the profile Experience tab, they
  have a base CV, and they confirm the refresh prompt
- **THEN** the base CV body is rebuilt from the current seed and presentation (title,
  template, margins, style) is preserved

#### Scenario: No silent multi-job rewrite

- **WHEN** the candidate confirms a refresh from the profile Experience tab
- **THEN** tailored copies for other vacancies are not rewritten as part of that single
  confirmation (only the base CV is refreshed from profile context)
