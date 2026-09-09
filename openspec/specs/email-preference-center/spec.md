# email-preference-center Specification

## Purpose

The way somebody holding one of our mails turns it off: a signed link that opens a
preference page with no account, no password and no support ticket. This capability
owns the token, the public endpoints, the RFC 8058 one-click target, the
`List-Unsubscribe` headers, and the four-group taxonomy that decides which switch
silences a given mail.

It exists because a subscriber wrote in to say there was no way out without signing
in, and that he did not think that was lawful. He was right on both counts: CAN-SPAM
does not permit an opt-out that costs a login, GDPR Art. 7(3) wants withdrawal to be
as easy as consent, and the Gmail and Yahoo bulk-sender rules have required
one-click unsubscribe since February 2024 — so the gap cost deliverability as well
as compliance.

## Requirements

### Requirement: Every non-essential mail carries a working unsubscribe link

Every mail the system sends that is not marked essential SHALL carry, in its footer
and in its `List-Unsubscribe` header, a URL that lets the recipient stop that kind of
mail without signing in, without a password, and without providing any information
beyond following the link. Mail marked essential — address verification and password
reset — SHALL carry neither the footer link nor the headers, because it cannot be
declined.

#### Scenario: A digest carries an unsubscribe URL

- **WHEN** the system renders a saved-search digest for delivery by email
- **THEN** the rendered footer contains an unsubscribe URL naming that recipient and the `alerts` group
- **AND** the outgoing message carries `List-Unsubscribe` and `List-Unsubscribe-Post` headers pointing at the same recipient and group

#### Scenario: A campaign carries an unsubscribe URL for its own group

- **WHEN** the system renders a one-off campaign for a recipient
- **THEN** the footer and headers carry an unsubscribe URL naming that recipient and the `news` group, not the `alerts` or `activity` group

#### Scenario: Essential mail carries no unsubscribe affordance

- **WHEN** the system renders an address-verification or password-reset mail
- **THEN** the rendered footer contains no unsubscribe link and no link to notification settings
- **AND** the outgoing message carries no `List-Unsubscribe` header

#### Scenario: A mail with no way out of it is refused rather than sent

- **WHEN** a message reaches the transport naming no group, or naming a silenceable group while carrying no unsubscribe URL
- **THEN** the send is refused and the mail does not go out

#### Scenario: An essential mail carrying an unsubscribe URL is refused too

- **WHEN** a message reaches the transport marked essential but carrying an unsubscribe URL
- **THEN** the send is refused, because that link would be an offer nobody can honour

### Requirement: Unsubscribe links are signed and name one user and one group

An unsubscribe link SHALL carry a token that names exactly one user and exactly one
mail group, authenticated by an HMAC over a server-held secret and verified in
constant time. The token SHALL NOT expire. A token whose signature does not verify,
whose group is not a known group, or whose user no longer exists SHALL be refused
without disclosing which of those was the case.

#### Scenario: A valid token identifies its user and group

- **WHEN** a token minted for a given user and the `news` group is presented
- **THEN** the system resolves it to that user and that group

#### Scenario: A tampered token is refused

- **WHEN** a token is presented whose user id, group, or signature has been altered
- **THEN** the system refuses it and reveals nothing about the account it names

#### Scenario: An old token still works

- **WHEN** a token minted a year earlier is presented
- **THEN** it resolves normally, because an unsubscribe link that has expired is a failed unsubscribe

#### Scenario: A token for a deleted account is refused

- **WHEN** a token names a user id that no longer exists
- **THEN** the system refuses it rather than erroring

### Requirement: A public preference page reachable without signing in

The system SHALL serve an unauthenticated page, excluded from search indexing, that
a valid unsubscribe token opens. The page SHALL show the account's email address,
one switch per mail group (`alerts`, `activity`, `news`), one switch per saved-search
subscription beneath the `alerts` group, and a single control that turns off all
non-essential mail at once. The page SHALL disclose no other account data — not CV
content, applications, tracking, billing, nor any other address — and SHALL NOT
establish a session.

#### Scenario: Opening the link shows the current preferences

- **WHEN** a recipient opens their unsubscribe link
- **THEN** the page shows their email address, the current state of all three group switches, and their saved-search subscriptions with each one's current state

#### Scenario: Changing a switch is recorded without authentication

- **WHEN** the recipient turns the `news` group off on that page
- **THEN** the preference is recorded for that account
- **AND** no further campaign or onboarding mail is sent to them

#### Scenario: Turning one saved search off leaves the others alone

- **WHEN** the recipient turns off one of several saved-search subscriptions
- **THEN** only that subscription stops producing digests, and the others continue

#### Scenario: Unsubscribe from everything

- **WHEN** the recipient uses the "unsubscribe from everything" control
- **THEN** all three group switches are turned off
- **AND** address-verification and password-reset mail continues to be delivered

#### Scenario: The page leaks nothing else about the account

- **WHEN** the page is served for a valid token
- **THEN** the response carries only the email address, the group switches, and the saved-search names and states
- **AND** no session cookie is issued

#### Scenario: An invalid token yields no account information

- **WHEN** the page is opened with a missing, malformed, or unverifiable token
- **THEN** the system shows a generic failure and discloses no address, name, or account state

### Requirement: One-click unsubscribe turns off only the group that mailed

The system SHALL expose an RFC 8058 one-click target that a mail client may POST to
without user confirmation. The target SHALL turn off exactly the group named by the
token and no other, SHALL succeed on a request carrying the
`List-Unsubscribe=One-Click` form body, and SHALL respond successfully so the client
reports the unsubscribe as done. Its response SHALL name what was turned off and
offer the full preference page.

#### Scenario: One-click on a digest stops digests only

- **WHEN** a mail client POSTs to the one-click target of a token naming the `alerts` group
- **THEN** the `alerts` group is turned off for that account
- **AND** the `activity` and `news` groups are left as they were

#### Scenario: One-click on a campaign stops campaigns only

- **WHEN** a mail client POSTs to the one-click target of a token naming the `news` group
- **THEN** the `news` group is turned off and saved-search digests continue to be delivered

#### Scenario: The client's one-click body is accepted

- **WHEN** the POST carries the `List-Unsubscribe=One-Click` form body required by RFC 8058
- **THEN** the request succeeds rather than being rejected as malformed

#### Scenario: The confirmation offers the rest

- **WHEN** a one-click request succeeds
- **THEN** the response names the group that was turned off and links to the full preference page

#### Scenario: A repeated one-click is not an error

- **WHEN** the same one-click target is POSTed a second time
- **THEN** the request succeeds and the group remains off

### Requirement: Mail belongs to exactly one group

Every mail the system sends SHALL belong to exactly one of four groups, and the
group SHALL decide which switch silences it. `alerts` covers saved-search digests.
`activity` covers saved-job reminders, lifecycle nudges, reports about the
recipient's own activity, and referral pings — somebody asking this person for
something because they offered to be asked is about them, not about us. `news`
covers one-off campaigns and the onboarding sequence: the mail the product sends on
its own initiative. `essential` covers address verification and password reset, and
SHALL NOT be silenceable.

#### Scenario: Turning off news does not stop activity mail

- **WHEN** an account turns off the `news` group
- **THEN** campaigns, onboarding mail, and referral pings stop
- **AND** saved-job reminders and lifecycle nudges continue to be delivered

#### Scenario: Turning off everything still delivers a password reset

- **WHEN** an account with all three groups turned off requests a password reset
- **THEN** the reset mail is delivered

### Requirement: The public routes are rate limited

The preference-reading, preference-writing, and one-click endpoints SHALL be
rate-limited, since they are reachable without authentication.

#### Scenario: Excessive requests are throttled

- **WHEN** a client issues requests to the preference endpoints faster than the configured limit
- **THEN** the excess requests are refused with the same throttling response the other public routes use
