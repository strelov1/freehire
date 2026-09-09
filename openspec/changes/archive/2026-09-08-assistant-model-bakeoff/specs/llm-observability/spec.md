## MODIFIED Requirements

### Requirement: A successful model call is recorded as a generation

The system SHALL record each successful `GenerateJSON` call as a Langfuse
generation carrying the model id, the input (system and user prompts), the raw
output, the token usage (input, output, cached input, total), and the call latency.

Cached input tokens are the subset of the input the provider served from its prompt
cache. The count SHALL be read from the same per-choice generation info the other counts
come from, and SHALL be recorded whenever any token count was reported at all.

The system SHALL NOT claim to distinguish "the provider reported no cached count" from
"the provider served nothing from cache". The LLM library writes that key unconditionally
from a zero-valued struct, so both arrive as `0` and the transport cannot tell them
apart. A reader that needs the distinction draws it across a run, not from one call.

#### Scenario: Successful enrichment call

- **WHEN** `GenerateJSON` completes and the model response includes usage tokens
- **THEN** a generation is queued with the model id, both prompts as input, the response as output, the token counts, and the measured latency

#### Scenario: Usage tokens absent from response

- **WHEN** the model response omits token usage
- **THEN** the generation is still queued with input, output, model, and latency, and usage is left unset rather than reported as zero

#### Scenario: The response carries a cached input count

- **WHEN** the model response carries a non-zero cached prompt-token count
- **THEN** the generation records it alongside the input and output counts

#### Scenario: The response carries no cached input count

- **WHEN** the model response carries input and output counts and no cached count
- **THEN** the generation records input and output and a cached count of zero, which is
  indistinguishable from a genuine cache miss and is not reported as if it were absent
