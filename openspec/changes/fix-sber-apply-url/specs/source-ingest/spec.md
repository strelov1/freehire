## MODIFIED Requirements

### Requirement: Russian bigtech single-company providers are registered

The system SHALL register adapters for the single-company Russian career APIs `yandex`,
`ozon`, `rwb`, `sber`, `alfabank`, `lamoda`, `kuper`, `aviasales`, `dodo`, `domclick`,
`mtslink`, `tbank`, `mts`, and `vk`, so each can be listed in the boards configuration.
Each adapter SHALL yield the normalized job shape (at least title, url, location, remote
flag, description, and the platform's native posting id) with the `description` as
sanitized HTML (or sanitized text for an API that publishes plain text) assembled from the
platform's authoritative field(s). An adapter whose list endpoint omits the description
SHALL fetch each posting's detail with bounded concurrency rather than yield an empty body,
and a single failed detail SHALL drop only that posting rather than abort the board. All
providers except `yandex` SHALL be `boardless`; `yandex` SHALL select host and language
(`ru`/`com`) from its `board`.

Where a provider's public vacancy page is addressed by a different identifier than its
dedup key, the adapter SHALL build the job `url` from the page identifier while keeping the
dedup `external_id` on the provider's stable posting id.

#### Scenario: Cursor-paginated board is fully crawled

- **WHEN** a `yandex` board is crawled and its list endpoint paginates by cursor
- **THEN** the adapter follows the cursor until exhausted, skips postings that redirect out
  or are hiring events, fetches each remaining posting's detail for the description, and
  yields each as the normalized job shape with `external_id` set to the posting's native id

#### Scenario: Page-paginated board is fully crawled

- **WHEN** an `ozon` board is crawled and its list endpoint paginates by page number
- **THEN** the adapter walks every page to the reported total, keeps only externally-listed
  vacancies, fetches each posting's detail for the description, and yields each as the
  normalized job shape

#### Scenario: Offset-paginated board with inline body needs no detail call

- **WHEN** a `sber` or `alfabank` board is crawled and the list endpoint carries the full
  body inline
- **THEN** the adapter walks the offset window to the reported total and yields each posting
  directly with a sanitized description, issuing no per-posting detail request

#### Scenario: Sber posting URL is built from the numeric internalId

- **WHEN** a `sber` posting is normalized and its feed record carries both a `requisitionId`
  GUID and a numeric `internalId`
- **THEN** the job `url` is `https://rabota.sber.ru/search/<internalId>` (the identifier the
  public vacancy page resolves — the `requisitionId` GUID route returns 404)
- **AND** the job `external_id` remains the `requisitionId`, so the dedup key is unchanged

#### Scenario: Header-gated board is crawled

- **WHEN** an `mts` board is crawled and its API requires a non-secret `x-api-key` header
- **THEN** the adapter obtains the public key, sends it on each request, walks the offset
  window to the reported total, fetches each posting's detail, and yields each as the
  normalized job shape

#### Scenario: A board with no open postings yields no jobs without error

- **WHEN** any of these providers' endpoints returns an empty posting list for a configured
  board
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped
