## ADDED Requirements

### Requirement: The category facet distinguishes AI engineering from classic ML

The `category` vocabulary SHALL include `ai_engineering` alongside `ml_ai`, and
the title dictionary SHALL route role titles between them: titles denoting
LLM/GenAI application work (RAG, agents, prompt engineering, applied AI, "AI
Engineer") resolve to `ai_engineering`, while titles denoting classic machine
learning / deep learning model work resolve to `ml_ai`. A title that explicitly
carries the ML token in a combined form (`ML/AI`, `AI/ML`, `ML Engineer`) SHALL
resolve to `ml_ai` — the ML signal wins over the bare AI signal. The dictionary
never guesses: a title naming neither resolves to an empty category, unchanged.

#### Scenario: An AI-application title resolves to ai_engineering

- **WHEN** a job titled "Applied AI Engineer" (or "LLM Engineer",
  "Generative AI Engineer", "Prompt Engineer", "RAG Engineer", "AI Engineer") is
  classified by the title dictionary
- **THEN** the derived `category` is `ai_engineering`

#### Scenario: A classic-ML title resolves to ml_ai

- **WHEN** a job titled "Machine Learning Engineer" (or "Deep Learning Engineer",
  "ML Engineer") is classified by the title dictionary
- **THEN** the derived `category` is `ml_ai`

#### Scenario: A combined ML-carrying title stays ml_ai

- **WHEN** a job titled "ML/AI Engineer" or "AI/ML Engineer" is classified by the
  title dictionary
- **THEN** the derived `category` is `ml_ai` (the explicit ML token wins over the
  bare AI token)

#### Scenario: ai_engineering is a recognized category value

- **WHEN** the LLM enrichment payload or any consumer validates a `category` of
  `ai_engineering` against the controlled vocabulary
- **THEN** the value is accepted (it is a member of the category vocabulary), not
  sanitized away
