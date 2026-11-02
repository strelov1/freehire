# Design

Normalize only the Computrabajo card href before its existing path check. The fixed
origin and posting-path restriction still select the destination. A regression test
parses real HTML structure with embedded control whitespace and verifies the exact
posting URL, identity, title and HTTP request construction.

OpenSpec CLI and the workflow's dependency fallback document are unavailable on this
host; the change artifacts are maintained manually.
