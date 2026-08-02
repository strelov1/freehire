## MODIFIED Requirements

### Requirement: Obvious non-vacancy posts are filtered before extraction

The crawl SHALL apply a heuristic prefilter at insert: a post with no vacancy
markers SHALL be stored as already-processed with zero vacancies, so it is
never sent to the LLM, while remaining recorded against re-crawls. The filter
SHALL favor recall: any post with plausible vacancy markers proceeds to
extraction. The marker set SHALL cover every language the configured channels
publish in, and a channel SHALL NOT be added to `sources/telegram.yml` in a
language the markers do not yet cover — an uncovered language makes the filter
reject that channel's vacancies wholesale, which is indistinguishable from the
channel having none.

#### Scenario: A non-vacancy post is recorded but not queued

- **WHEN** a crawled post contains no vacancy markers
- **THEN** it is stored as processed with zero vacancies and is not claimable
  by the extraction worker

#### Scenario: A Ukrainian-language vacancy reaches extraction

- **WHEN** a crawled post advertises a role in Ukrainian (e.g. «Шукаємо Golang
  розробника», «Вакансія: QA Engineer») and contains no Russian or English
  marker
- **THEN** it is stored as pending and is claimable by the extraction worker

#### Scenario: A Ukrainian-language non-vacancy post is still filtered

- **WHEN** a crawled post is Ukrainian editorial content carrying no hiring
  markers (e.g. «Дайджест новин тижня»)
- **THEN** it is stored as processed with zero vacancies and is not claimable
  by the extraction worker
