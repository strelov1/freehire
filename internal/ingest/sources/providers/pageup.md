# PageUp

Board remains the numeric institution ID. Canonical tenants use the XHR JSON
search/detail envelopes on `careers.pageuppeople.com/{board}/cw/en/`.
`X-Requested-With: XMLHttpRequest` is required; ordinary HTML is not the API.

Board 1023 is a verified exception: North Memorial uses
`jobs.northmemorial.com/nmh/en-us/search/?page-items=1000` and
`/nmh/en-us/job/{numericID}/{slug}`. Only this host is routed through Scrapling
HTTP + proxy. It is a legacy PageUp theme, not a Clinch sitemap. Preserve catalog
company and numeric job IDs; do not infer an employer rename in the parser.

Responsive markup repeats job links. Deduplicate native IDs before comparing the
unique listing count with the envelope count. A mismatch fails before detail
hydration, including when a catalogue exceeds the requested 1000-item page.
1023 also requires count to be present. Other canonical tenants keep their
existing envelope compatibility when count is absent. No full/empty-board sweep
capability is claimed. Details use `#job-details`; 1023 and 889 always fetch it
because their listing text is absent or a teaser. Detail failures retain the
listed job with an empty body, as before.

See [completion evidence](../../../../docs/investigations/2026-11-15-ingest-completion.md)
and [migration ledger](../../../../docs/investigations/2026-11-15-ingest-pageup.md).
