# Local PII filtering before CV extraction

CV extraction sends bounded original text to the configured LLM gateway and extracts
contacts together with the professional sections. It no longer uses a local detection model.

## Why this is out of scope

The removal was requested after a production audit on 2026-11-13 (local time):

- Centralized logs for the preceding seven days contained approximately 7,300 successful
  detector entries and ten API-side detector errors, all context deadline expirations.
  These are log counts, not a measured request failure percentage.
- Four short synthetic requests took 5–14 seconds each on a host under high load. This
  measured an additional dependency's latency, not the complete CV extraction time.
- Recognition defects had required fixes for character/byte offsets and partial names
  on 2026-10-19, and employment dates recognized as phones on 2026-11-02. The audit did
  not measure a production recognition error rate.
- The detector silently considered only the first 4,096 tokens while the extraction
  prompt could include up to 12,000 characters, leaving a suffix without model detection.
- Its separate runtime, model weights and availability gate made extraction more complex
  while its automatic tests did not measure recognition accuracy on a labeled CV corpus.

The user accepted removal of preprocessing. This deliberately changes the gateway boundary:
the extraction model receives names and contacts. It does not change the contact fields
excluded by professional/public projections, sanitation, date validation, caller attribution
or upload-stamp persistence guards. Professional strings repeating extracted contacts are still rejected.

The evidence does not support saying that every request failed or quantifying how often the
model missed PII. The decision is based on latency, observed timeouts, known recognition
defects and the simpler direct extraction path.

## Revisit if …

A concrete requirement demands masking before the gateway, with representative labeled
CV evaluation, complete input coverage and a measured latency/availability budget.

## Deployment retirement

Host configuration belongs to the private `freehire-ops` repository. Deploy the application
without a detector dependency before disabling `freehire-pii-filter.service`; old application
processes fail closed without it. Then remove `PII_FILTER_URL`, the service and its dedicated
monitoring configuration from ops and the hosts. Do not delete model weights as part of the
application change or claim that deleting this repository's sources stops the live service.

## Prior requests

- 2026-11-13: remove pii-filter because of slow processing and recognition problems,
  simplify CV extraction and verify the resulting behavior.
