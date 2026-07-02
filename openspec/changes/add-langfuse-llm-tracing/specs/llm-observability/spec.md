## ADDED Requirements

### Requirement: Tracing is gated on complete configuration

The system SHALL enable Langfuse tracing only when all of `LANGFUSE_BASE_URL`,
`LANGFUSE_PUBLIC_KEY`, and `LANGFUSE_SECRET_KEY` are set. When any is empty, the
tracer SHALL be a no-op and the LLM client SHALL behave exactly as if tracing did
not exist.

#### Scenario: All three variables set

- **WHEN** the LLM client is constructed with a base URL, public key, and secret key all present
- **THEN** an active tracer is attached and model calls are reported to Langfuse

#### Scenario: A variable is missing

- **WHEN** any of the three Langfuse variables is empty
- **THEN** no tracer is attached and `GenerateJSON` performs no tracing work

#### Scenario: Worker runs with no Langfuse configuration

- **WHEN** a worker starts with none of the Langfuse variables set
- **THEN** it constructs its LLM client, makes model calls, and completes its run exactly as before — no tracer goroutine, no network attempt, no error, and `Shutdown` on the absent tracer is a safe no-op

### Requirement: A successful model call is recorded as a generation

The system SHALL record each successful `GenerateJSON` call as a Langfuse
generation carrying the model id, the input (system and user prompts), the raw
output, the token usage (input, output, total), and the call latency.

#### Scenario: Successful enrichment call

- **WHEN** `GenerateJSON` completes and the model response includes usage tokens
- **THEN** a generation is queued with the model id, both prompts as input, the response as output, the token counts, and the measured latency

#### Scenario: Usage tokens absent from response

- **WHEN** the model response omits token usage
- **THEN** the generation is still queued with input, output, model, and latency, and usage is left unset rather than reported as zero

### Requirement: Tracing never blocks or fails the caller

The system SHALL treat tracing as best-effort. A tracer failure (network error,
non-2xx response, serialization error) SHALL be logged and swallowed, and SHALL
NOT alter the value or error returned by `GenerateJSON`.

#### Scenario: Langfuse endpoint unreachable

- **WHEN** reporting a generation to Langfuse fails
- **THEN** the failure is logged and `GenerateJSON` returns its normal result unchanged

#### Scenario: Reporting is off the caller's critical path

- **WHEN** `GenerateJSON` is called
- **THEN** the model response is returned without waiting on a synchronous round-trip to Langfuse

### Requirement: A failed model call is recorded at error level

The system SHALL record a generation at error level when a model call fails or
returns an unusable response, capturing the error detail so failures are
visible in Langfuse alongside successful calls.

#### Scenario: Model returns unparseable or empty response

- **WHEN** `GenerateJSON` fails because the model returned no choices or an unusable response
- **THEN** a generation is queued at error level with the error message recorded

### Requirement: Generations are attributed to their originating worker

The system SHALL tag each generation with metadata identifying which caller
produced it (for example enrichment versus telegram extraction), so traces can
be filtered by workload in Langfuse.

#### Scenario: Enrichment versus telegram extraction

- **WHEN** a generation originates from the enrichment worker rather than telegram extraction
- **THEN** its metadata distinguishes the two source workloads

### Requirement: Buffered generations are flushed before exit

The system SHALL flush any buffered generations to Langfuse before a run-once
worker exits, so no trace is lost when the process terminates normally.

#### Scenario: Worker finishes its run

- **WHEN** a run-once worker completes and shuts the tracer down
- **THEN** all buffered generations are sent before the process exits
