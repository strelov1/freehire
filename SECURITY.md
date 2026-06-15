# Security Policy

freehire is a server-side application: a public HTTP API, a set of standalone
crawl/enrichment workers, a PostgreSQL database, and a Meilisearch index. This
document describes the trust boundaries and how to report a vulnerability.

## Reporting a Vulnerability

Please report suspected vulnerabilities **privately**, by either:

- Opening a private report through **GitHub Security Advisories** for this
  repository (Security → Report a vulnerability), or
- Emailing **strelov1@gmail.com**.

Please include:

- A description of the issue and its impact.
- Steps to reproduce, a proof of concept, or relevant logs.
- The affected endpoint, worker, package, commit SHA, or configuration.
- Any known mitigations.

Do not open a public issue for security-sensitive reports. We will review and
coordinate disclosure as appropriate.

## In Scope

The hosted service at `freehire.dev` and the code in this repository, in
particular:

- **Authentication and session handling** — bypass of the JWT session cookie,
  API-key authentication, or the authorization-code OAuth flows
  (Google / GitHub / LinkedIn).
- **Account takeover** — any path that links or creates an account from an
  **unverified** external email, or that re-keys an existing account to another
  identity.
- **Authorization** — bypass of the `moderator` role gate on the job
  create/edit endpoints, or any per-user data leaking across users.
- **Server-side request forgery (SSRF)** in the crawl/link-following workers
  (`cmd/ingest`, `cmd/tg-ingest`, `internal/linksource`), where worker-fetched
  URLs could be steered at internal addresses or used to exfiltrate metadata.
- **Injection** reachable through the API or the ingest pipeline.
- **Secret exposure** — credentials owned by the project or granting access to
  `freehire.dev` infrastructure.

## Out of Scope

- Misconfiguration of a **self-hosted** deployment (weak `JWT_SECRET`,
  `COOKIE_SECURE=false` over public HTTP, an exposed database or Meilisearch
  master key, etc.). Securely operating your own instance is your
  responsibility.
- Denial of service that requires trusted local input or configuration, or that
  is simple volumetric flooding without an amplification/asymmetry in freehire.
- **Prompt injection of the enrichment LLM.** Worker input is untrusted job
  text; the LLM only classifies it and its output is sanitized against a fixed
  controlled vocabulary before storage (`Enrichment.Sanitize`/`Validate`).
  Getting the model to emit junk is expected and is filtered, not a
  vulnerability — unless you can show it crossing a real boundary (e.g.
  executing code, leaking secrets, or persisting an out-of-vocabulary value).
- Vulnerabilities in third-party dependencies that are not reachable through
  freehire. For dependency reports, include evidence the issue is reachable.
- Reports about the content of aggregated job postings (the data is sourced from
  third parties and is not authored by the project).

## Notes for Reporters

The most useful reports demonstrate a current, reproducible boundary bypass with
real impact, against the latest release or latest `main`. Include the exact
affected endpoint or worker, the commit SHA, the configuration, and a proof of
concept.
