## MODIFIED Requirements

### Requirement: The feed's ordering vocabulary names relevance explicitly

The jobs feed's client-side sort vocabulary SHALL be `relevance`, `newest`,
`views` and `match`. `relevance` names the engine's own ranking — the order a
request with query text and no `sort` parameter already receives — so that the
control can report it rather than mislabel it. `views` orders by how many people
have opened each posting, labelled `Most viewed`.

The default SHALL be contextual: `relevance` when the filter state carries query
text, `newest` when it does not. Serialization SHALL omit the `sort` parameter
while the contextual default is selected, matching the endpoint's own defaults
(see `job-search`, "Default ordering is newest-added first"): no `sort` with
query text yields relevance, and no `sort` without query text yields `posted_at`
descending.

A non-default selection SHALL be serialized as the value the endpoint accepts:
`newest` as `sort=posted_at`, `views` as `sort=view_count`, `match` as
`sort=match`. Deserialization SHALL map `posted_at` back to `newest`,
`view_count` back to `views`, and `match` back to `match`; an absent or
unrecognised value SHALL resolve to the contextual default, so a hand-edited or
shared link never leaves the control in a state it cannot render.

The stored ordering SHALL distinguish "the caller chose this" from "no choice has
been made"; the latter SHALL resolve to the contextual default at read time rather
than being written into the state. Storing the resolved default instead would make
a browse feed's `newest` indistinguishable from a chosen `newest`, so typing into
the search box would carry `sort=posted_at` into a text search and date-order it —
defeating the contextual default at the commonest entry point, and changing the
serialization of the live filters so they no longer compare equal to the saved
search they came from.

`relevance` has nothing to rank against without query text. Resolving the
selection SHALL therefore collapse `relevance` to `newest` whenever the query is
empty. `views` SHALL NOT collapse — it ranks by a stored figure and is meaningful
with or without query text. Both resolutions SHALL be one pure function shared by
the control and the serializer so the two cannot disagree.

#### Scenario: Browsing with no query omits the sort parameter

- **WHEN** the filter state carries no query text and the selection is `newest`
- **THEN** the serialized parameters carry no `sort` key

#### Scenario: A text query defaults to relevance and omits the parameter

- **WHEN** the filter state carries query text and the selection is `relevance`
- **THEN** the serialized parameters carry no `sort` key

#### Scenario: Newest under a query is sent explicitly

- **WHEN** the filter state carries query text and the selection is `newest`
- **THEN** the serialized parameters carry `sort=posted_at`

#### Scenario: Most viewed is sent explicitly

- **WHEN** the selection is `views`
- **THEN** the serialized parameters carry `sort=view_count`

#### Scenario: A shared most-viewed link resolves

- **WHEN** parameters carrying `sort=view_count` are deserialized
- **THEN** the selection is `views`

#### Scenario: Most viewed survives clearing the query

- **WHEN** the selection is `views` and the query text is emptied
- **THEN** the resolved selection is still `views` and the serialized parameters
  carry `sort=view_count`

#### Scenario: A shared match link still resolves

- **WHEN** parameters carrying `sort=match` are deserialized
- **THEN** the selection is `match`

#### Scenario: An unrecognised sort value falls back to the contextual default

- **WHEN** parameters carrying `sort=bogus` and query text are deserialized
- **THEN** the selection is `relevance`

#### Scenario: Relevance collapses when the query is cleared

- **WHEN** the selection is `relevance` and the query text is emptied
- **THEN** the resolved selection is `newest` and the serialized parameters carry
  no `sort` key

#### Scenario: Typing a query does not pin the browse ordering

- **WHEN** query text is added to a filter state whose ordering was never chosen
- **THEN** the resolved selection is `relevance` and the serialized parameters
  carry no `sort` key

#### Scenario: A typed query still matches the saved search it came from

- **WHEN** the saved search `q=go` is compared against a filter state reached by
  typing `go` with no ordering chosen
- **THEN** the two serialize identically, so the saved search reads as active
  rather than dirty and saving again does not create a duplicate

### Requirement: The sort control is shown whenever it offers a choice

The jobs list SHALL render its sort control whenever the control holds more than
one option, and SHALL omit it otherwise. Option availability SHALL be:

- `newest` — always available.
- `views` — always available. It ranks by a stored figure, so it needs neither
  query text nor a signed-in profile.
- `relevance` — available only when the filter state carries query text.
- `match` — available only under the existing precondition (an authenticated
  caller whose loaded profile names at least one skill, and the runtime flag).

Because `newest` and `views` are both unconditional, the control now holds at
least two options for every caller and is therefore rendered on every listing.
The "omit it otherwise" clause is retained rather than dropped: it is the rule
that makes the control's presence a consequence of what it can offer, and a
future ordering could narrow the set again.

A signed-out visitor searching by text therefore reaches the freshest-first
ordering, which is the reachability gap this replaces: the control was previously
rendered only under the `match` precondition, so the only ordering a signed-out
visitor could ever receive under query text was relevance, unlabelled.

The URL parameter SHALL be honoured regardless of whether the control is
rendered, unchanged from today: the endpoint degrades an ordering it cannot serve
rather than refusing it. When the resolved ordering is one the control does not
offer, the control SHALL show the ordering the endpoint will actually serve that
caller rather than an unrepresented value — a select whose value matches no option
renders blank, which would put an empty control over a live ordering.

#### Scenario: A signed-out visitor with a text query can reorder

- **WHEN** a signed-out visitor searches for text
- **THEN** the sort control is rendered offering `Relevance`, `Newest` and
  `Most viewed`

#### Scenario: A signed-out visitor browsing is offered two orderings

- **WHEN** a signed-out visitor opens the list with no query text
- **THEN** the sort control is rendered offering `Newest` and `Most viewed`
- **AND** the feed is ordered freshest first

#### Scenario: An eligible caller is offered the match sort

- **WHEN** an authenticated caller whose profile names skills views the list with
  the match flag enabled
- **THEN** the sort control additionally offers `Best match`

#### Scenario: A shared match link renders as what it will actually serve

- **WHEN** a signed-out visitor opens a link carrying `q=go&sort=match`
- **THEN** the control shows `Relevance` — the ordering the endpoint degrades to —
  and the `sort=match` parameter is left in the URL untouched
