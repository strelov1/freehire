## ADDED Requirements

### Requirement: The inbox hides mail that is not about an application

The inbox listing SHALL, by default, omit messages the classifier labelled as not being
about an application at all, and SHALL report how many it omitted. A caller SHALL be able
to ask for them.

The judgement is the classifier's, not a curated list of senders. It already labels every
message on a call the system already makes, it judges by content rather than by who sent
it, and a sender nobody has seen before is judged on arrival rather than after somebody
notices it. A domain list would need maintaining indefinitely against senders who register
domains for a living.

The count is not decoration. A filter that hides silently makes a misclassification
impossible to find, and the classifier reads attacker-controlled text.

#### Scenario: Mail that is not about an application is omitted by default

- **WHEN** a caller lists their inbox and some of their mail is labelled as not about an
  application
- **THEN** that mail is absent from the listing

#### Scenario: The listing says how much it hid

- **WHEN** a listing omits such mail
- **THEN** it reports the number omitted

#### Scenario: The caller can see what was hidden

- **WHEN** a caller asks for the hidden mail
- **THEN** it is listed

#### Scenario: An unclassified message is never hidden

- **WHEN** a message has no classification yet
- **THEN** it is listed, because nothing has judged it
