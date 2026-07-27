## REMOVED Requirements

### Requirement: The tailoring agent acts as the user via a scoped, short-lived credential

**Reason**: The tailoring agent is no longer an external process. It runs inside
the freehire backend as the authenticated caller, so there is nothing to
authenticate and no credential to confine — the requirement's whole purpose was
to bound the blast radius of a key handed to somebody else's machine.

**Migration**: The tailoring bootstrap stops minting a key and stops returning
`cli_token`; its callers stop reading that field. Owner confinement of CV reads
and edits is now covered by the `assistant-agent-runtime` requirement "Tools act
as the authenticated caller", which holds the same guarantee (a CV tool cannot
touch another user's CV) without a credential. User-created `cv`-scoped API keys
under `/me` are unaffected and keep working for the public CLI.
