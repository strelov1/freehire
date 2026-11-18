# source-descriptions Specification

## Purpose
How adapters produce descriptions: sanitized HTML, a description derived from source HTML, and a company-level description kept apart from posting bodies. Split out of source-ingest.

## Requirements

### Requirement: Adapter descriptions are sanitized HTML

Each adapter SHALL yield the job `description` as sanitized HTML assembled from the platform's authoritative HTML field(s), so the stored value is safe to render directly in a browser without further escaping. An adapter SHALL NOT yield raw or entity-encoded source markup, and SHALL NOT rely on a platform plain-text field that the platform may leave empty or partial. Sanitization SHALL run server-side before the description is persisted, stripping scripts, event handlers, and other active content while preserving structural formatting (headings, paragraphs, lists, emphasis).

A persisted description SHALL carry no links. Sanitization SHALL unwrap every anchor to its visible text, so a posting can neither route a reader off the catalogue nor pass link authority, while the sentence the link sat in stays whole. It SHALL discard an anchor whose visible text merely restates its own destination, together with any block element that leaves empty — stripped of its href such text is a dead address rather than prose.

#### Scenario: Greenhouse entity-encoded HTML is decoded and sanitized

- **WHEN** a greenhouse posting returns `content` as entity-encoded HTML (e.g. `&lt;h2&gt;Role&lt;/h2&gt;`)
- **THEN** the adapter yields a description whose entities are decoded to real markup and then sanitized, so the stored value contains `<h2>Role</h2>` rather than the encoded entities

#### Scenario: Lever multi-field body is assembled, not truncated

- **WHEN** a lever posting splits its body across `description`, one or more `lists` (each with a heading `text` and HTML `content`), and `additional`
- **THEN** the adapter yields a description that concatenates the opening `description`, each list as a heading followed by its content, and the closing `additional` — even when `descriptionPlain` is empty

#### Scenario: Ashby uses the HTML field

- **WHEN** an ashby posting exposes both `descriptionHtml` and `descriptionPlain`
- **THEN** the adapter yields the sanitized `descriptionHtml`, preserving its formatting

#### Scenario: Active content is stripped

- **WHEN** a source posting's HTML contains a `<script>` tag or an inline event handler (e.g. `onclick`)
- **THEN** the persisted description contains neither the script nor the event handler, while its safe structural markup is retained

#### Scenario: A worded link keeps its words and loses its destination

- **WHEN** a source posting's HTML links a word inside a sentence (e.g. `We use <a href="https://k8s.io">Kubernetes</a> in prod.`)
- **THEN** the persisted description reads `We use Kubernetes in prod.` with no anchor, href, or `rel` attribute

#### Scenario: A link labelled with its own address is discarded

- **WHEN** a source posting's HTML contains an anchor whose visible text restates its destination (e.g. `<p><a href="https://x.co/1">https://x.co/1</a></p>`)
- **THEN** the persisted description contains neither the address nor the paragraph it stood alone in

### Requirement: An adapter may derive the description from source HTML

An adapter SHALL be permitted to obtain a posting body by extracting it from the source's
server-rendered HTML when the platform exposes no machine-readable description in its API,
and SHALL yield it as sanitized HTML, consistent with every other adapter's description
contract. Such extraction SHALL be isolated to that adapter, and an extraction failure for
one posting SHALL drop only that posting rather than abort the board.

#### Scenario: VK description is extracted from the vacancy page

- **WHEN** a `vk` board is crawled and VK's API carries no description field
- **THEN** the adapter fetches the vacancy page, extracts the description sections from its
  HTML, and yields a sanitized HTML description in the normalized job shape

#### Scenario: A failed extraction drops only that posting

- **WHEN** one VK posting's page cannot be fetched or its body cannot be extracted
- **THEN** that posting is skipped and every other posting is still yielded, without
  aborting the board

### Requirement: An adapter may supply a company-level description separate from posting bodies

A `Source` adapter MAY, when its platform exposes a company/board-level
description through a distinct endpoint or field from the one it uses for
postings, fetch and return that text once per board crawl. An adapter that
does not implement this SHALL simply return nothing for it, and the pipeline
SHALL treat that as "no company description available" rather than an error.

Fetching a company-level description SHALL NOT require a per-posting
request: it is fetched at most once per board per crawl, independent of how
many postings that board yields.

#### Scenario: An adapter returns a company description alongside its postings

- **WHEN** an adapter's platform exposes a company-level description and the
  adapter implements this capability
- **THEN** a single crawl of that board yields the postings as before, plus
  one company-level description for the board

#### Scenario: An adapter without this capability yields nothing extra

- **WHEN** an adapter does not implement a company-level description lookup
- **THEN** the pipeline receives no company description for that board and
  proceeds exactly as it did before this capability existed

#### Scenario: A blank company-level field is not treated as a description

- **WHEN** an adapter's platform exposes the field but the employer left it
  empty
- **THEN** the adapter returns nothing for it, not an empty string
