## MODIFIED Requirements

### Requirement: The page tool is confined to the browsing preset

`read_current_page` SHALL NOT be registered for the `chat` or `tailor` presets. A
conversation held outside a browser extension has no page to read, and a tool
that can only fail spends the model's context without ever returning an answer.

Preset alone is not sufficient: `read_current_page` SHALL additionally be withheld
from a `browse`-preset turn unless the request that carries it authenticated with a
Bearer session JWT — the extension's own carrier — rather than the website's session
cookie. A `browse` session reached some other way (a stale link, a future client
that lists it anyway) SHALL still run as a conversation; it SHALL simply run without
the page tool, the same degrade-rather-than-fail pattern a tailoring session without
a CV binding already follows.

#### Scenario: A chat session is not offered the page tool

- **WHEN** a session running the `chat` preset builds its tool registry
- **THEN** `read_current_page` is absent from it

#### Scenario: A browsing session reached over the website's cookie has no page tool

- **WHEN** a turn for a `browse`-preset session is submitted authenticated by the website's session cookie rather than a Bearer session JWT
- **THEN** the registry built for that turn does not contain `read_current_page`, and the turn otherwise runs as an ordinary chat

#### Scenario: A browsing session reached from the extension keeps the page tool

- **WHEN** a turn for a `browse`-preset session is submitted authenticated by a Bearer session JWT
- **THEN** the registry built for that turn contains `read_current_page`
