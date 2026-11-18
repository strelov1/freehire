# Computrabajo posting URL whitespace

## Why

Two Computrabajo detail reads failed with `net/url: invalid control character in URL`
in `freehire-ingest-run-computrabajo.service` during 2026-11-02 11:00:27–13:00:27 UTC.
Both links contain an embedded tab. Removing it returns HTTP 200 with a usable posting.

## Change

Strip ASCII tab, carriage return and line feed from posting hrefs as browsers do,
before validating the posting path. Preserve the posting identity and fragment removal.

## Non-goals

Do not change shared HTTP transport, retry policy or production configuration.
